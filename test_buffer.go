package main

import (
	"dxcluster/buffer"
	"dxcluster/spot"
	"fmt"
)

func main() {
	fmt.Println("=== Ring Buffer Demo ===\n")

	// Create a buffer that holds 5 spots
	buf := buffer.NewRingBuffer(5)

	// Add some spots
	fmt.Println("Adding 3 spots...")
	buf.Add(spot.NewSpot("LZ5VV", "W1ABC", 14074.5, "FT8"))
	buf.Add(spot.NewSpot("DL1ABC", "K3XYZ", 7100.0, "CW"))
	buf.Add(spot.NewSpot("JA1XYZ", "VE3QQ", 21200.0, "SSB"))

	fmt.Printf("Buffer size: %d\n\n", buf.Size())

	// Get all spots
	spots := buf.GetAll()
	fmt.Println("All spots (most recent first):")
	for _, s := range spots {
		fmt.Println(s.FormatDXCluster())
	}

	// Add more spots (will wrap around)
	fmt.Println("\nAdding 4 more spots (buffer will wrap)...")
	buf.Add(spot.NewSpot("G4ABC", "W2XYZ", 3550.0, "SSB"))
	buf.Add(spot.NewSpot("VE3ABC", "K1DEF", 14250.0, "SSB"))
	buf.Add(spot.NewSpot("ZL1ABC", "W3GHI", 7050.0, "CW"))
	buf.Add(spot.NewSpot("VK2XYZ", "K4JKL", 14025.0, "CW"))

	fmt.Printf("Buffer size: %d (max capacity)\n\n", buf.Size())

	// Get recent spots
	fmt.Println("Last 3 spots:")
	recent := buf.GetRecent(3)
	for _, s := range recent {
		fmt.Println(s.FormatDXCluster())
	}

	fmt.Println("\nAll spots in buffer:")
	all := buf.GetAll()
	for _, s := range all {
		fmt.Println(s.FormatDXCluster())
	}
}
