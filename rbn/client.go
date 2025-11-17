// Package rbn implements a client for the Reverse Beacon Network (RBN) telnet service.
//
// RBN provides automated CW and RTTY spot reports from skimmer stations worldwide.
// This client connects to RBN's telnet server, logs in, and parses spot messages
// into the canonical Spot format.
//
// RBN Spot Format:
//   DX de N2WQ-1-#:   14024.0  LZ5VV   CW    22 dB  28 WPM  CQ      1928Z
//   Components: spotter, frequency, callsign, mode, signal, speed, comment, time
//
// Features:
//   - Automatic login with configured callsign
//   - Real-time spot parsing with regex-based whitespace normalization
//   - Callsign normalization (removes numeric SSID: N2WQ-1-# → N2WQ-#)
//   - 5-minute read timeout for connection monitoring
//   - Buffered spot channel (100 spots)
//   - Graceful shutdown support
//
// The client feeds spots into the deduplicator's input channel in the unified architecture.
package rbn

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dxcluster/spot"
)

// Client represents a Reverse Beacon Network (RBN) telnet client.
//
// The client maintains a persistent telnet connection to the RBN server,
// logs in with the configured callsign, and continuously reads and parses
// spot messages.
//
// Fields:
//   - host, port: RBN server address (typically telnet.reversebeacon.net:7000)
//   - callsign: Amateur radio callsign for authentication
//   - conn, reader, writer: Telnet connection and I/O buffers
//   - connected: Connection state flag
//   - shutdown: Channel for coordinating graceful shutdown
//   - spotChan: Buffered channel for outputting parsed spots (capacity 100)
//
// Thread Safety:
//   - readLoop runs in its own goroutine
//   - spotChan is buffered and uses non-blocking sends
//   - shutdown channel coordinates clean termination
type Client struct {
	host      string          // RBN server hostname
	port      int             // RBN server port (typically 7000)
	callsign  string          // Callsign for RBN authentication
	conn      net.Conn        // Telnet TCP connection
	reader    *bufio.Reader   // Buffered reader for telnet protocol
	writer    *bufio.Writer   // Buffered writer for sending commands
	connected bool            // Connection state flag
	shutdown  chan struct{}   // Shutdown coordination channel
	spotChan  chan *spot.Spot // Output channel for parsed spots (buffered 100)
}

// NewClient creates a new RBN client
func NewClient(host string, port int, callsign string) *Client {
	return &Client{
		host:     host,
		port:     port,
		callsign: callsign,
		shutdown: make(chan struct{}),
		spotChan: make(chan *spot.Spot, 100),
	}
}

// Connect establishes connection to RBN
func (c *Client) Connect() error {
	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	log.Printf("Connecting to RBN at %s...", addr)

	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to RBN: %w", err)
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.writer = bufio.NewWriter(conn)
	c.connected = true

	log.Println("Connected to RBN")

	// Start login sequence
	go c.handleLogin()

	// Start reading spots
	go c.readLoop()

	return nil
}

// handleLogin performs the RBN login sequence
func (c *Client) handleLogin() {
	// Wait for login prompt and respond with callsign
	time.Sleep(2 * time.Second)

	log.Printf("Logging in to RBN as %s", c.callsign)
	c.writer.WriteString(c.callsign + "\n")
	c.writer.Flush()
}

// readLoop reads lines from RBN
func (c *Client) readLoop() {
	defer func() {
		c.connected = false
		if c.conn != nil {
			c.conn.Close()
		}
	}()

	for {
		select {
		case <-c.shutdown:
			log.Println("RBN client shutting down")
			return
		default:
			// Set read timeout
			c.conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

			line, err := c.reader.ReadString('\n')
			if err != nil {
				log.Printf("RBN read error: %v", err)
				return
			}

			line = strings.TrimSpace(line)

			// Skip empty lines
			if line == "" {
				continue
			}

			// Log and parse DX spots
			if strings.HasPrefix(line, "DX de") {
				c.parseSpot(line)
			}
		}
	}
}

// normalizeRBNCallsign removes numeric SSID from RBN callsigns
// Example: N2WQ-1-# -> N2WQ-#, N2WQ-73-# -> N2WQ-#, K3LR-2-# -> K3LR-#
func normalizeRBNCallsign(callsign string) string {
	// Pattern: CALL-NUMBER-# where NUMBER can be 1-3 digits
	// We want to keep CALL-# but remove the numeric SSID

	// Check if it ends with -#
	if !strings.HasSuffix(callsign, "-#") {
		return callsign // Not an RBN skimmer callsign
	}

	// Remove the trailing -#
	withoutHash := strings.TrimSuffix(callsign, "-#")

	// Split by hyphen
	parts := strings.Split(withoutHash, "-")

	if len(parts) < 2 {
		return callsign // No SSID to remove
	}

	// Check if the last part is numeric (the SSID we want to remove)
	lastPart := parts[len(parts)-1]
	if _, err := strconv.Atoi(lastPart); err == nil {
		// Last part is numeric, remove it
		// Rejoin everything except the last part, then add back -#
		baseParts := parts[:len(parts)-1]
		return strings.Join(baseParts, "-") + "-#"
	}

	// Last part is not numeric, keep as-is
	return callsign
}

// parseSpot parses an RBN spot line into a Spot object
func (c *Client) parseSpot(line string) {
	// Normalize whitespace - replace multiple spaces with single space
	normalized := regexp.MustCompile(`\s+`).ReplaceAllString(line, " ")

	// Split by spaces
	parts := strings.Fields(normalized)

	// Minimum: DX de CALL: FREQ DXCALL MODE DB dB WPM WPM COMMENT TIME
	// Example parts: [DX de G4ZFE-#: 10111.0 LZ2PC CW 11 dB 22 WPM CQ 1928Z]
	if len(parts) < 10 {
		log.Printf("RBN spot too short: %s", line)
		return
	}

	// Extract fields
	deCall := strings.TrimSuffix(parts[2], ":") // Remove trailing colon
	deCall = normalizeRBNCallsign(deCall)       // Normalize RBN callsign

	freqStr := parts[3]
	dxCall := parts[4]
	mode := parts[5]
	dbStr := parts[6]
	// parts[7] is "dB"
	wpmStr := parts[8]
	// parts[9] is "WPM"

	// Everything after WPM until the time is the comment
	// Find the time (4 digits followed by Z)
	var comment string
	for i := 10; i < len(parts); i++ {
		if len(parts[i]) == 5 && strings.HasSuffix(parts[i], "Z") {
			// Everything between WPM and time is comment
			if i > 10 {
				comment = strings.Join(parts[10:i], " ")
			}
			break
		}
	}

	// Parse frequency
	freq, err := strconv.ParseFloat(freqStr, 64)
	if err != nil {
		log.Printf("Failed to parse frequency '%s': %v", freqStr, err)
		return
	}

	// Create spot
	s := spot.NewSpot(dxCall, deCall, freq, mode)
	s.Comment = fmt.Sprintf("%s dB %s WPM %s", dbStr, wpmStr, comment)
	s.SourceType = spot.SourceRBN

	// Send to spot channel
	select {
	case c.spotChan <- s:
		log.Printf("Parsed RBN spot: %s spotted by %s on %.1f kHz", dxCall, deCall, freq)
	default:
		log.Println("RBN spot channel full, dropping spot")
	}
}

// GetSpotChannel returns the channel for receiving spots
func (c *Client) GetSpotChannel() <-chan *spot.Spot {
	return c.spotChan
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	return c.connected
}

// Stop closes the RBN connection
func (c *Client) Stop() {
	log.Println("Stopping RBN client...")
	close(c.shutdown)
	if c.conn != nil {
		c.conn.Close()
	}
}
