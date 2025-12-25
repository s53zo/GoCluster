package peer

import "testing"

func TestHandleFrameQueuesLegacy(t *testing.T) {
	m := &Manager{
		topology: &topologyStore{},
		legacyCh: make(chan legacyWork, 1),
	}
	frame, err := ParseFrame("PC19^1^N0CALL^0^1.0^H99^")
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	m.HandleFrame(frame, nil)
	select {
	case got := <-m.legacyCh:
		if got.frame == nil || got.frame.Type != "PC19" {
			t.Fatalf("unexpected legacy frame: %+v", got.frame)
		}
	default:
		t.Fatal("expected legacy frame to be queued")
	}
}
