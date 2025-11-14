package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"dxcluster/config"
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

	// Create and start telnet server
	telnetServer := telnet.NewServer(
		cfg.Telnet.Port,
		cfg.Telnet.WelcomeMessage,
		cfg.Telnet.MaxConnections,
	)

	err = telnetServer.Start()
	if err != nil {
		log.Fatalf("Failed to start telnet server: %v", err)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	fmt.Println("\nCluster is running. Press Ctrl+C to stop.")
	fmt.Printf("Connect via: telnet localhost %d\n", cfg.Telnet.Port)

	// Wait for shutdown signal
	sig := <-sigChan
	fmt.Printf("\nReceived signal: %v\n", sig)
	fmt.Println("Shutting down gracefully...")

	// Stop the telnet server
	telnetServer.Stop()

	log.Println("Cluster stopped")
}
