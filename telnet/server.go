package telnet

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Server represents a telnet server
type Server struct {
	port           int
	welcomeMessage string
	maxConnections int
	listener       net.Listener
	clients        map[string]*Client
	clientsMutex   sync.RWMutex
	shutdown       chan struct{}
}

// Client represents a connected telnet client
type Client struct {
	conn      net.Conn
	reader    *bufio.Reader
	writer    *bufio.Writer
	callsign  string
	connected time.Time
	address   string
}

// Telnet protocol constants
const (
	IAC  = 255 // Interpret As Command
	DONT = 254
	DO   = 253
	WONT = 252
	WILL = 251
)

// NewServer creates a new telnet server
func NewServer(port int, welcomeMessage string, maxConnections int) *Server {
	return &Server{
		port:           port,
		welcomeMessage: welcomeMessage,
		maxConnections: maxConnections,
		clients:        make(map[string]*Client),
		shutdown:       make(chan struct{}),
	}
}

// Start begins listening for telnet connections
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start telnet server: %w", err)
	}

	s.listener = listener
	log.Printf("Telnet server listening on port %d", s.port)

	// Accept connections in a goroutine
	go s.acceptConnections()

	return nil
}

// acceptConnections handles incoming connections
func (s *Server) acceptConnections() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				// Server is shutting down
				return
			default:
				log.Printf("Error accepting connection: %v", err)
				continue
			}
		}

		// Handle this client in a new goroutine
		go s.handleClient(conn)
	}
}

// handleClient manages a single client connection
func (s *Server) handleClient(conn net.Conn) {
	defer conn.Close()

	address := conn.RemoteAddr().String()
	log.Printf("New connection from %s", address)

	// Create client object
	client := &Client{
		conn:      conn,
		reader:    bufio.NewReader(conn),
		writer:    bufio.NewWriter(conn),
		connected: time.Now(),
		address:   address,
	}

	// Send telnet options to suppress advanced features
	client.SendRaw([]byte{IAC, DONT, 1}) // Don't echo
	client.SendRaw([]byte{IAC, DONT, 3}) // Don't suppress go-ahead
	client.SendRaw([]byte{IAC, WONT, 1}) // Won't echo
	client.SendRaw([]byte{IAC, WILL, 3}) // Will suppress go-ahead

	// Send welcome message
	client.Send(s.welcomeMessage)
	client.Send("Enter your callsign: ")

	// Read callsign
	callsign, err := client.ReadLine()
	if err != nil {
		log.Printf("Error reading callsign from %s: %v", address, err)
		return
	}

	client.callsign = strings.ToUpper(strings.TrimSpace(callsign))
	log.Printf("Client %s logged in as %s", address, client.callsign)

	// Register client
	s.registerClient(client)
	defer s.unregisterClient(client)

	// Send login confirmation
	client.Send(fmt.Sprintf("Hello %s, you are now connected.\n", client.callsign))
	client.Send("Type 'bye' to disconnect.\n")

	// Read commands from client
	for {
		line, err := client.ReadLine()
		if err != nil {
			log.Printf("Client %s disconnected: %v", client.callsign, err)
			return
		}

		// Normalize command (trim and lowercase)
		cmd := strings.ToLower(strings.TrimSpace(line))

		// Skip empty lines
		if cmd == "" {
			continue
		}

		// Handle simple commands
		if cmd == "bye" || cmd == "quit" || cmd == "exit" {
			client.Send("73!\n")
			log.Printf("Client %s logged out", client.callsign)
			return
		}

		// Unknown command
		client.Send(fmt.Sprintf("Unknown command: %s\n", line))
	}
}

// registerClient adds a client to the active clients list
func (s *Server) registerClient(client *Client) {
	s.clientsMutex.Lock()
	defer s.clientsMutex.Unlock()
	s.clients[client.callsign] = client
	log.Printf("Registered client: %s (total: %d)", client.callsign, len(s.clients))
}

// unregisterClient removes a client from the active clients list
func (s *Server) unregisterClient(client *Client) {
	s.clientsMutex.Lock()
	defer s.clientsMutex.Unlock()
	delete(s.clients, client.callsign)
	log.Printf("Unregistered client: %s (total: %d)", client.callsign, len(s.clients))
}

// GetClientCount returns the number of connected clients
func (s *Server) GetClientCount() int {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	return len(s.clients)
}

// Stop shuts down the telnet server
func (s *Server) Stop() {
	log.Println("Stopping telnet server...")
	close(s.shutdown)
	if s.listener != nil {
		s.listener.Close()
	}

	// Disconnect all clients
	s.clientsMutex.Lock()
	for _, client := range s.clients {
		client.conn.Close()
	}
	s.clientsMutex.Unlock()
}

// SendRaw sends raw bytes to the client
func (c *Client) SendRaw(data []byte) error {
	_, err := c.writer.Write(data)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

// Send writes a message to the client with proper line endings
func (c *Client) Send(message string) error {
	// Replace \n with \r\n for proper telnet line endings
	message = strings.ReplaceAll(message, "\n", "\r\n")
	_, err := c.writer.WriteString(message)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

// ReadLine reads a line from the client, filtering out telnet control codes
func (c *Client) ReadLine() (string, error) {
	var line []byte

	for {
		b, err := c.reader.ReadByte()
		if err != nil {
			return "", err
		}

		// Handle telnet IAC (Interpret As Command) sequences
		if b == IAC {
			// Read next byte
			cmd, err := c.reader.ReadByte()
			if err != nil {
				return "", err
			}

			// If it's DO, DONT, WILL, WONT, read and discard the option byte
			if cmd == DO || cmd == DONT || cmd == WILL || cmd == WONT {
				_, err := c.reader.ReadByte()
				if err != nil {
					return "", err
				}
			}
			// Skip this sequence and continue reading
			continue
		}

		// End of line
		if b == '\n' {
			break
		}

		// Skip carriage return
		if b == '\r' {
			continue
		}

		// Add to line buffer
		line = append(line, b)
	}

	return string(line), nil
}
