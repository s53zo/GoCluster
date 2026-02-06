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

func TestUpdateEventsOverviewBoxesMatchesOverviewSummary(t *testing.T) {
	lines := []string{
		"Cluster: test  Version: 1  Uptime: 00:01",
		"MEMORY / GC",
		"Heap: 1 MiB",
		"INGEST RATES (per min)",
		"RBN: 1",
		"PSK: 2",
		"P92: 3",
		"Path: 4",
		"Primary: ok",
		"Corrections: ok",
		"CACHES & DATA FRESHNESS",
		"Grid: ok",
		"PATH PREDICTIONS",
		"160m: ok",
		"NETWORK",
		"Telnet: ok",
	}
	d := &DashboardV2{
		overviewHdr:      newBoxedTextView("Overview"),
		overviewMem:      newBoxedTextView("Memory / GC"),
		overviewIngest:   newBoxedTextView("Ingest Rates (per min)"),
		overviewPipeline: newBoxedTextView("Pipeline Quality"),
		eventsHdr:        newBoxedTextView("Overview"),
		eventsMem:        newBoxedTextView("Memory / GC"),
		eventsIngest:     newBoxedTextView("Ingest Rates (per min)"),
		eventsPipeline:   newBoxedTextView("Pipeline Quality"),
	}

	d.updateOverviewBoxes(lines)
	d.updateEventsOverviewBoxes(lines)

	if got, want := d.eventsHdr.GetText(true), d.overviewHdr.GetText(true); got != want {
		t.Fatalf("events header mismatch: got %q want %q", got, want)
	}
	if got, want := d.eventsMem.GetText(true), d.overviewMem.GetText(true); got != want {
		t.Fatalf("events memory mismatch: got %q want %q", got, want)
	}
	if got, want := d.eventsIngest.GetText(true), d.overviewIngest.GetText(true); got != want {
		t.Fatalf("events ingest mismatch: got %q want %q", got, want)
	}
	if got, want := d.eventsPipeline.GetText(true), d.overviewPipeline.GetText(true); got != want {
		t.Fatalf("events pipeline mismatch: got %q want %q", got, want)
	}
}

func TestEventsPanelOnlyReceivesSystemStream(t *testing.T) {
	eventsView := newBoxedTextView("Events")
	validationView := newBoxedTextView("Validation")
	correctedView := newBoxedTextView("Corrected")
	unlicensedView := newBoxedTextView("Unlicensed")
	harmonicsView := newBoxedTextView("Harmonics")

	d := &DashboardV2{
		eventsPanel:     newStreamPanel(eventsView, "Events", 10),
		validationPanel: newStreamPanel(validationView, "Validation", 10),
		correctedPanel:  newStreamPanel(correctedView, "Corrected", 10),
		unlicensedPanel: newStreamPanel(unlicensedView, "Unlicensed", 10),
		harmonicsPanel:  newStreamPanel(harmonicsView, "Harmonics", 10),
		scheduler:       newFrameScheduler(tview.NewApplication(), 60, 50, nil),
	}

	d.AppendDropped("CTY drop: A")
	d.AppendCall("corr B")
	d.AppendUnlicensed("unl C")
	d.AppendHarmonic("harm D")
	d.AppendReputation("rep E")
	d.AppendSystem("sys F")

	d.eventsPanel.Render(nil)
	got := d.eventsPanel.View().GetText(true)
	if !strings.Contains(got, "sys F") {
		t.Fatalf("expected system line in events panel, got %q", got)
	}
	for _, forbidden := range []string{"CTY drop: A", "corr B", "unl C", "harm D", "rep E"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("unexpected non-system line in events panel: %q (panel=%q)", forbidden, got)
		}
	}
}

func TestEventsStreamDynamicColorsDisabled(t *testing.T) {
	eventsView := newBoxedTextView("Events")
	eventsView.SetDynamicColors(false)
	panel := newStreamPanel(eventsView, "Events", 10)

	panel.Append("[yellow]tag[-] literal")
	panel.Render(nil)

	got := eventsView.GetText(true)
	if !strings.Contains(got, "[yellow]tag[-] literal") {
		t.Fatalf("expected literal bracket text in events stream, got %q", got)
	}
}
