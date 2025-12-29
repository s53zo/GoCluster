package pskreporter

import (
	"strings"
	"testing"
	"time"
)

func TestDecorateSpotterCall(t *testing.T) {
	with := &Client{appendSSID: true}

	if got := with.decorateSpotterCall("K1ABC"); got != "K1ABC-#" {
		t.Fatalf("expected -# suffix for bare call, got %s", got)
	}
	if got := with.decorateSpotterCall("K1ABC-1"); got != "K1ABC-1" {
		t.Fatalf("expected existing SSID to remain untouched, got %s", got)
	}
	// 10-character calls can be expanded when still within the validation limit.
	longCall := "AB2CDEFGHI" // length 10
	if got := with.decorateSpotterCall(longCall); got != longCall+"-#" {
		t.Fatalf("expected long call to include SSID suffix, got %s", got)
	}

	without := &Client{appendSSID: false}
	if got := without.decorateSpotterCall("K1ABC"); got != "K1ABC" {
		t.Fatalf("expected disabled flag to leave call unchanged, got %s", got)
	}
}

func TestConvertToSpotOmitsCommentAndCarriesGrids(t *testing.T) {
	client := NewClient("localhost", 1883, nil, "", 1, nil, false, 16, 0)

	msg := &PSKRMessage{
		SequenceNumber:  1,
		Frequency:       14074000,
		Mode:            "FT8",
		Report:          10,
		Timestamp:       time.Now().Add(-time.Minute).Unix(),
		SenderCall:      "K1ABC",
		SenderLocator:   "fn42",
		ReceiverCall:    "N0CALL",
		ReceiverLocator: "em10",
	}

	spot := client.convertToSpot(msg)
	if spot == nil {
		t.Fatalf("expected spot, got nil")
	}
	if trimmed := strings.TrimSpace(spot.Comment); trimmed != "" {
		t.Fatalf("expected empty comment, got %q", trimmed)
	}
	if spot.DXMetadata.Grid != "FN42" {
		t.Fatalf("expected DX grid FN42, got %q", spot.DXMetadata.Grid)
	}
	if spot.DEMetadata.Grid != "EM10" {
		t.Fatalf("expected DE grid EM10, got %q", spot.DEMetadata.Grid)
	}
}

type testMessage struct {
	payload []byte
}

func (m testMessage) Duplicate() bool { return false }
func (m testMessage) Qos() byte       { return 0 }
func (m testMessage) Retained() bool  { return false }
func (m testMessage) Topic() string   { return "" }
func (m testMessage) MessageID() uint16 {
	return 0
}
func (m testMessage) Payload() []byte { return m.payload }
func (m testMessage) Ack()            {}

func TestMessageHandlerDropsOversizePayload(t *testing.T) {
	client := NewClient("localhost", 1883, nil, "", 1, nil, false, 16, 4)
	client.processing = make(chan []byte, 1)

	client.messageHandler(nil, testMessage{payload: make([]byte, 10)})

	select {
	case <-client.processing:
		t.Fatalf("expected oversized payload to be dropped")
	default:
	}
}
