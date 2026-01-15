package pskreporter

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func intPtr(v int) *int {
	return &v
}

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
	client := NewClient("localhost", 1883, nil, nil, "", 1, nil, false, 16, 0)

	msg := &PSKRMessage{
		SequenceNumber:  1,
		Frequency:       14074000,
		Mode:            "FT8",
		Report:          intPtr(10),
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
	client := NewClient("localhost", 1883, nil, nil, "", 1, nil, false, 16, 4)
	client.processing = make(chan []byte, 1)

	client.messageHandler(nil, testMessage{payload: make([]byte, 10)})

	select {
	case <-client.processing:
		t.Fatalf("expected oversized payload to be dropped")
	default:
	}
}

func TestHandlePayloadFiltersModes(t *testing.T) {
	// Allow only FT8.
	client := NewClient("localhost", 1883, nil, []string{"FT8"}, "", 1, nil, false, 2, 0)
	allowed := PSKRMessage{
		Frequency:    14074000,
		Mode:         "FT8",
		Report:       intPtr(5),
		Timestamp:    time.Now().Unix(),
		SenderCall:   "K1ABC",
		ReceiverCall: "N0CALL",
	}
	blocked := allowed
	blocked.Mode = "FT4"

	allowedPayload, _ := json.Marshal(allowed)
	blockedPayload, _ := json.Marshal(blocked)

	// Blocked mode should not reach spotChan.
	client.handlePayload(blockedPayload)
	select {
	case <-client.spotChan:
		t.Fatalf("expected blocked mode to be dropped")
	default:
	}

	// Allowed mode should enqueue a spot.
	client.handlePayload(allowedPayload)
	select {
	case <-client.spotChan:
		// ok
	default:
		t.Fatalf("expected allowed mode to enqueue a spot")
	}
}

func TestHandlePayloadDropsZeroSNR(t *testing.T) {
	client := NewClient("localhost", 1883, nil, nil, "", 1, nil, false, 2, 0)
	msg := PSKRMessage{
		Frequency:    14074000,
		Mode:         "FT8",
		Report:       intPtr(0),
		Timestamp:    time.Now().Unix(),
		SenderCall:   "K1ABC",
		ReceiverCall: "N0CALL",
	}
	payload, _ := json.Marshal(msg)

	client.handlePayload(payload)
	select {
	case spot := <-client.spotChan:
		t.Fatalf("expected zero-SNR spot to be dropped, got %+v", spot)
	default:
	}
}

func TestHandlePayloadAllowsMissingReport(t *testing.T) {
	client := NewClient("localhost", 1883, nil, nil, "", 1, nil, false, 2, 0)
	msg := PSKRMessage{
		Frequency:    14074000,
		Mode:         "FT8",
		Timestamp:    time.Now().Unix(),
		SenderCall:   "K1ABC",
		ReceiverCall: "N0CALL",
	}
	payload, _ := json.Marshal(msg)

	client.handlePayload(payload)
	select {
	case spot := <-client.spotChan:
		if spot.HasReport {
			t.Fatalf("expected HasReport=false when report missing, got Report=%d", spot.Report)
		}
	default:
		t.Fatalf("expected missing-report payload to enqueue a spot")
	}
}

func TestHandlePayloadAllowsPSKVariantsWithCanonicalAllowlist(t *testing.T) {
	client := NewClient("localhost", 1883, nil, []string{"PSK"}, "", 1, nil, false, 2, 0)
	psk31 := PSKRMessage{
		Frequency:    14074000,
		Mode:         "PSK31",
		Report:       intPtr(5),
		Timestamp:    time.Now().Unix(),
		SenderCall:   "K1ABC",
		ReceiverCall: "N0CALL",
	}
	payload, _ := json.Marshal(psk31)

	client.handlePayload(payload)
	select {
	case spot := <-client.spotChan:
		if spot.Mode != "PSK31" || spot.ModeNorm != "PSK" {
			t.Fatalf("expected PSK31 variant with canonical PSK ModeNorm, got Mode=%q ModeNorm=%q", spot.Mode, spot.ModeNorm)
		}
	default:
		t.Fatalf("expected PSK31 to be accepted when PSK is allowed")
	}
}
