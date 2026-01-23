package telnet

import (
	"strings"
	"testing"
	"time"

	"dxcluster/spot"
)

func TestHandleDiagCommandToggle(t *testing.T) {
	server := NewServer(ServerOptions{}, nil)
	client := &Client{}

	resp, handled := server.handleDiagCommand(client, "SET DIAG ON")
	if !handled {
		t.Fatalf("expected SET DIAG ON to be handled")
	}
	if !strings.Contains(resp, "ON") {
		t.Fatalf("expected ON response, got %q", resp)
	}
	if !client.diagEnabled.Load() {
		t.Fatalf("expected diagEnabled to be true")
	}

	resp, handled = server.handleDiagCommand(client, "SET DIAG OFF")
	if !handled {
		t.Fatalf("expected SET DIAG OFF to be handled")
	}
	if !strings.Contains(resp, "OFF") {
		t.Fatalf("expected OFF response, got %q", resp)
	}
	if client.diagEnabled.Load() {
		t.Fatalf("expected diagEnabled to be false")
	}
}

func TestFormatSpotForClientDiagComment(t *testing.T) {
	server := NewServer(ServerOptions{}, nil)
	client := &Client{}
	client.setDedupePolicy(dedupePolicySlow)
	client.diagEnabled.Store(true)

	sp := spot.NewSpot("LZ2BE", "M9PSY-#", 3524.6, "CW")
	sp.Report = 26
	sp.HasReport = true
	sp.Time = time.Date(2025, time.January, 7, 4, 9, 0, 0, time.UTC)
	sp.SourceType = spot.SourceRBN
	sp.DEMetadata.ADIF = 291
	sp.DEMetadata.CQZone = 5
	sp.DEMetadata.Grid = "KN33"
	sp.DXMetadata.Grid = "KN33"
	sp.Confidence = "S"
	sp.Comment = "ORIG"

	line := server.formatSpotForClient(client, sp)
	if strings.Contains(line, "ORIG") {
		t.Fatalf("expected diagnostic comment to replace original, got %q", line)
	}
	if !strings.Contains(line, "R2910580S") {
		t.Fatalf("expected diagnostic tag in output, got %q", line)
	}
	if !strings.HasSuffix(strings.TrimRight(line, "\r\n "), "KN33 S 0409Z") {
		t.Fatalf("expected tail preserved, got %q", line)
	}
}
