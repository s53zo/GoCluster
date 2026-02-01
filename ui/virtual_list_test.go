package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestVirtualListDraw(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}

	list := NewVirtualList()
	list.SetRect(0, 0, 40, 2)
	events := []StyledEvent{
		{Timestamp: time.Unix(0, 0), Kind: EventDrop, Message: "alpha"},
		{Timestamp: time.Unix(1, 0), Kind: EventSystem, Message: "beta"},
	}
	list.SetSnapshot(events, nil)
	list.Draw(screen)

	ch, _, _, _ := screen.GetContent(0, 0)
	if ch == ' ' {
		t.Fatalf("expected rendered content, got blank")
	}
}
