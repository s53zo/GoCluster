package peer

import (
	"context"
	"strings"
	"testing"
)

func TestHandlePingUsesPriorityQueue(t *testing.T) {
	s := &session{
		localCall:      "N2WQ-77",
		priorityLineCh: make(chan string, 1),
		ctx:            context.Background(),
	}
	frame, err := ParseFrame("PC51^N2WQ-77^N2WQ-73^1^")
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	s.handlePing(frame)
	select {
	case line := <-s.priorityLineCh:
		if !strings.HasPrefix(line, "PC51^N2WQ-73^N2WQ-77^0^") {
			t.Fatalf("unexpected ACK line: %q", line)
		}
	default:
		t.Fatal("expected PC51 ACK to be enqueued on priorityLineCh")
	}
}
