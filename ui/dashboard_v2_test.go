package ui

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
)

func TestStreamPanelOverflow(t *testing.T) {
	tv := tview.NewTextView()
	panel := newStreamPanel(tv, "Corrected", 2)

	panel.Append("alpha")
	panel.Append("bravo")
	panel.Append("charlie")
	panel.Render(nil)

	text := tv.GetText(true)
	if !strings.Contains(text, "... +1 more") {
		t.Fatalf("expected overflow marker, got %q", text)
	}
}
