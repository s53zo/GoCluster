package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dxcluster/buffer"
	"dxcluster/commands"
	"dxcluster/config"
	"dxcluster/dedup"
	"dxcluster/pskreporter"
	"dxcluster/rbn"
	"dxcluster/telnet"
)

// Version will be set at build time
var Version = "dev"

func main() {
	// Print startup message
	fmt.Printf("DX Cluster Server v%s starting...\n", Version)

	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Print the configuration
	cfg.Print()

	// Create spot buffer (ring buffer for storing recent spots)
	spotBuffer := buffer.NewRingBuffer(1000)
	log.Println("Ring buffer created (capacity: 1000)")

	// Create deduplicator if enabled
	// THIS IS THE UNIFIED DEDUP ENGINE - ALL SOURCES FEED INTO IT
	var deduplicator *dedup.Deduplicator
	if cfg.Dedup.Enabled {
		window := time.Duration(cfg.Dedup.ClusterWindowSeconds) * time.Second
		deduplicator = dedup.NewDeduplicator(window)
		deduplicator.Start() // Start the processing loop
		log.Printf("Deduplication enabled with %v window", window)

		// Wire up dedup output to ring buffer and telnet broadcast
		// Deduplicated spots → Ring Buffer → Broadcast to clients
		go processOutputSpots(deduplicator, spotBuffer, nil) // We'll pass telnet server later
	}

	// Create command processor
	processor := commands.NewProcessor(spotBuffer)

	// Create and start telnet server
	telnetServer := telnet.NewServer(
		cfg.Telnet.Port,
		cfg.Telnet.WelcomeMessage,
		cfg.Telnet.MaxConnections,
		processor,
	)

	err = telnetServer.Start()
	if err != nil {
		log.Fatalf("Failed to start telnet server: %v", err)
	}

	// Now wire up the telnet server to the output processor
	if cfg.Dedup.Enabled {
		// Restart the output processor with telnet server
		go processOutputSpots(deduplicator, spotBuffer, telnetServer)
	}

	// Connect to RBN if enabled
	// RBN spots go INTO the deduplicator input channel
	var rbnClient *rbn.Client
	if cfg.RBN.Enabled {
		rbnClient = rbn.NewClient(cfg.RBN.Host, cfg.RBN.Port, cfg.RBN.Callsign)
		err = rbnClient.Connect()
		if err != nil {
			log.Printf("Warning: Failed to connect to RBN: %v", err)
		} else {
			if cfg.Dedup.Enabled {
				// RBN → Deduplicator Input Channel
				go processRBNSpots(rbnClient, deduplicator)
				log.Println("RBN client feeding spots into unified dedup engine")
			} else {
				// No dedup - RBN goes directly to buffer (legacy path)
				go processRBNSpotsNoDedupe(rbnClient, spotBuffer, telnetServer)
			}
		}
	}

	// Connect to PSKReporter if enabled
	// PSKReporter spots go INTO the deduplicator input channel
	var pskrClient *pskreporter.Client
	if cfg.PSKReporter.Enabled {
		pskrClient = pskreporter.NewClient(cfg.PSKReporter.Broker, cfg.PSKReporter.Port, cfg.PSKReporter.Topic)
		err = pskrClient.Connect()
		if err != nil {
			log.Printf("Warning: Failed to connect to PSKReporter: %v", err)
		} else {
			if cfg.Dedup.Enabled {
				// PSKReporter → Deduplicator Input Channel
				go processPSKRSpots(pskrClient, deduplicator)
				log.Println("PSKReporter client feeding spots into unified dedup engine")
			} else {
				// No dedup - PSKReporter goes directly to buffer (legacy path)
				go processPSKRSpotsNoDedupe(pskrClient, spotBuffer, telnetServer)
			}
		}
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	fmt.Println("\nCluster is running. Press Ctrl+C to stop.")
	fmt.Printf("Connect via: telnet localhost %d\n", cfg.Telnet.Port)
	if cfg.RBN.Enabled {
		fmt.Println("Receiving real-time CW/RTTY spots from RBN...")
	}
	if cfg.PSKReporter.Enabled {
		fmt.Printf("Receiving digital mode spots from PSKReporter (topic: %s)...\n", cfg.PSKReporter.Topic)
	}
	if cfg.Dedup.Enabled {
		fmt.Printf("Unified deduplication active: %d second window\n", cfg.Dedup.ClusterWindowSeconds)
		fmt.Println("Architecture: ALL sources → Dedup Engine → Ring Buffer → Clients")
	}

	// Wait for shutdown signal
	sig := <-sigChan
	fmt.Printf("\nReceived signal: %v\n", sig)
	fmt.Println("Shutting down gracefully...")

	// Stop deduplicator
	if deduplicator != nil {
		deduplicator.Stop()
	}

	// Stop RBN client
	if rbnClient != nil {
		rbnClient.Stop()
	}

	// Stop PSKReporter client
	if pskrClient != nil {
		pskrClient.Stop()
	}

	// Stop the telnet server
	telnetServer.Stop()

	log.Println("Cluster stopped")
}

// processRBNSpots receives spots from RBN and sends to deduplicator
// This is the UNIFIED ARCHITECTURE path
// RBN → Deduplicator Input Channel
func processRBNSpots(client *rbn.Client, deduplicator *dedup.Deduplicator) {
	spotChan := client.GetSpotChannel()
	dedupInput := deduplicator.GetInputChannel()

	for spot := range spotChan {
		// Send spot to deduplicator input channel
		// All sources send here!
		dedupInput <- spot
	}
}

// processPSKRSpots receives spots from PSKReporter and sends to deduplicator
// PSKReporter → Deduplicator Input Channel
func processPSKRSpots(client *pskreporter.Client, deduplicator *dedup.Deduplicator) {
	spotChan := client.GetSpotChannel()
	dedupInput := deduplicator.GetInputChannel()

	for spot := range spotChan {
		// Send spot to deduplicator input channel
		dedupInput <- spot
	}
}

// processOutputSpots receives deduplicated spots and distributes them
// Deduplicator Output → Ring Buffer → Broadcast to Clients
func processOutputSpots(deduplicator *dedup.Deduplicator, buf *buffer.RingBuffer, telnet *telnet.Server) {
	outputChan := deduplicator.GetOutputChannel()

	for spot := range outputChan {
		// Add to ring buffer
		buf.Add(spot)

		// Broadcast to all connected telnet clients
		if telnet != nil {
			telnet.BroadcastSpot(spot)
		}
	}
}

// processRBNSpotsNoDedupe is the legacy path when deduplication is disabled
// RBN → Ring Buffer → Clients (no deduplication)
func processRBNSpotsNoDedupe(client *rbn.Client, buf *buffer.RingBuffer, telnet *telnet.Server) {
	spotChan := client.GetSpotChannel()

	for spot := range spotChan {
		// Add directly to buffer (no dedup)
		buf.Add(spot)

		// Broadcast to all connected telnet clients
		telnet.BroadcastSpot(spot)
	}
}

// processPSKRSpotsNoDedupe is the legacy path when deduplication is disabled
func processPSKRSpotsNoDedupe(client *pskreporter.Client, buf *buffer.RingBuffer, telnet *telnet.Server) {
	spotChan := client.GetSpotChannel()

	for spot := range spotChan {
		buf.Add(spot)
		telnet.BroadcastSpot(spot)
	}
}
