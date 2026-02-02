package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rivo/tview"
)

func TestPaneWriterBounds(t *testing.T) {
	writer := &paneWriter{dash: &DashboardV2{}}
	input := bytes.Repeat([]byte("a"), paneWriterMaxBytes*2)
	n, err := writer.Write(input)
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
	if n != len(input) {
		t.Fatalf("expected write %d bytes, got %d", len(input), n)
	}
	if len(writer.buf) != paneWriterMaxBytes {
		t.Fatalf("expected buffer size %d, got %d", paneWriterMaxBytes, len(writer.buf))
	}
	if writer.droppedBytes == 0 {
		t.Fatalf("expected dropped bytes to be tracked")
	}
}

func TestPipelineCorrectedOverflow(t *testing.T) {
	dash := &DashboardV2{
		pipelineCorrected: tview.NewTextView(),
		correctedMax:      2,
		scheduler:         &frameScheduler{pending: make(map[string]func())},
	}
	dash.appendCorrectedStream("alpha")
	dash.appendCorrectedStream("bravo")
	dash.appendCorrectedStream("charlie")

	dash.scheduler.mu.Lock()
	fn := dash.scheduler.pending["corrected"]
	dash.scheduler.mu.Unlock()
	if fn == nil {
		t.Fatalf("expected scheduled corrected update")
	}
	fn()

	text := dash.pipelineCorrected.GetText(true)
	if !strings.Contains(text, "... +1 more") {
		t.Fatalf("expected overflow marker, got %q", text)
	}
}
