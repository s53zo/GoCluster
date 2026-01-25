package telnet

import (
	"strings"
	"testing"
)

func TestHandlePathSettingsNoiseIndustrial(t *testing.T) {
	server := &Server{
		noiseOffsets: map[string]float64{
			"QUIET":      0,
			"INDUSTRIAL": 21,
		},
	}
	client := &Client{}
	resp, handled := server.handlePathSettingsCommand(client, "SET NOISE INDUSTRIAL")
	if !handled {
		t.Fatalf("expected SET NOISE to be handled")
	}
	if !strings.Contains(resp, "Noise class set to INDUSTRIAL") {
		t.Fatalf("unexpected response: %q", resp)
	}
	if client.noiseClass != "INDUSTRIAL" {
		t.Fatalf("expected noise class INDUSTRIAL, got %q", client.noiseClass)
	}
	if client.noisePenalty != 21 {
		t.Fatalf("expected industrial penalty 21, got %.2f", client.noisePenalty)
	}
}
