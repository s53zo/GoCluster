package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestVirtualLogViewMaintainsBoundedHistory(t *testing.T) {
	v := newVirtualLogView("Events", 3, false)
	v.Append("one")
	v.Append("two")
	v.Append("three")
	v.Append("four")

	got := v.SnapshotText()
	if strings.Contains(got, "one") {
		t.Fatalf("expected oldest line to be evicted, got %q", got)
	}
	for _, line := range []string{"two", "three", "four", "... +1 more"} {
		if !strings.Contains(got, line) {
			t.Fatalf("expected %q in snapshot, got %q", line, got)
		}
	}
}

func TestVirtualLogViewScrollDeterministic(t *testing.T) {
	v := newVirtualLogView("Events", 8, false)
	v.SetRect(0, 0, 40, 5)
	for i := 0; i < 8; i++ {
		v.Append("line")
	}

	if !v.HandleScroll(tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone)) {
		t.Fatalf("expected home key to be handled")
	}
	v.mu.Lock()
	homeOffset := v.offset
	v.mu.Unlock()
	if homeOffset != 0 {
		t.Fatalf("expected home offset 0, got %d", homeOffset)
	}

	if !v.HandleScroll(tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone)) {
		t.Fatalf("expected end key to be handled")
	}
	v.mu.Lock()
	endOffset := v.offset
	v.mu.Unlock()
	if endOffset == 0 {
		t.Fatalf("expected non-zero end offset with overflow")
	}

	if !v.HandleScroll(tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone)) {
		t.Fatalf("expected k-scroll to be handled")
	}
	v.mu.Lock()
	upOffset := v.offset
	v.mu.Unlock()
	if upOffset >= endOffset {
		t.Fatalf("expected k-scroll to move up, end=%d current=%d", endOffset, upOffset)
	}
}
