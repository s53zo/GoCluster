package ui

import (
	"testing"
	"time"
)

func TestBoundedEventBufferEvictsOldest(t *testing.T) {
	buf := NewBoundedEventBuffer("events", 2, 0, DropPolicy{MaxMessageBytes: 0, EvictOnByteLimit: true}, nil)
	buf.Append(StyledEvent{Timestamp: time.Unix(1, 0), Kind: EventDrop, Message: "a"})
	buf.Append(StyledEvent{Timestamp: time.Unix(2, 0), Kind: EventDrop, Message: "b"})
	buf.Append(StyledEvent{Timestamp: time.Unix(3, 0), Kind: EventDrop, Message: "c"})

	snap := buf.SnapshotInto(nil)
	if len(snap.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(snap.Events))
	}
	if snap.Events[0].Message != "b" || snap.Events[1].Message != "c" {
		t.Fatalf("unexpected order: %+v", snap.Events)
	}
}

func TestBoundedEventBufferMaxMessageDrop(t *testing.T) {
	buf := NewBoundedEventBuffer("events", 2, 0, DropPolicy{MaxMessageBytes: 3, EvictOnByteLimit: true, LogDrops: false}, nil)
	if ok := buf.Append(StyledEvent{Timestamp: time.Now(), Kind: EventDrop, Message: "abcd"}); ok {
		t.Fatalf("expected oversized message drop")
	}
	drops := buf.DropSnapshot()
	if drops.Oversized != 1 {
		t.Fatalf("expected oversized drop count 1, got %d", drops.Oversized)
	}
}

func TestBoundedEventBufferByteLimitReject(t *testing.T) {
	buf := NewBoundedEventBuffer("events", 10, 4, DropPolicy{MaxMessageBytes: 0, EvictOnByteLimit: false, LogDrops: false}, nil)
	if ok := buf.Append(StyledEvent{Timestamp: time.Now(), Kind: EventDrop, Message: "abcd"}); !ok {
		t.Fatalf("expected first append to succeed")
	}
	if ok := buf.Append(StyledEvent{Timestamp: time.Now(), Kind: EventDrop, Message: "ef"}); ok {
		t.Fatalf("expected byte limit drop")
	}
	drops := buf.DropSnapshot()
	if drops.ByteLimit != 1 {
		t.Fatalf("expected byte limit drop count 1, got %d", drops.ByteLimit)
	}
}
