package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestFormatLinePadsAndStripsControls(t *testing.T) {
	got := formatLine("a\r\nb\tc", 5, false)
	want := "abc  "
	if got != want {
		t.Fatalf("formatLine control stripping mismatch: got %q want %q", got, want)
	}
}

func TestFormatLineMarkup(t *testing.T) {
	got := formatLine("[red]X[-]", 2, true)
	want := "\x1b[31mX\x1b[0m "
	if got != want {
		t.Fatalf("formatLine markup mismatch: got %q want %q", got, want)
	}

	plain := formatLine("[red]X[-]", 1, false)
	if plain != "X" {
		t.Fatalf("formatLine strip mismatch: got %q want %q", plain, "X")
	}
}

func TestFillFrameLayout(t *testing.T) {
	c := &ansiConsole{
		blankLine:   formatLine("", ansiWidth, false),
		headerDrop:  formatLine("Dropped", ansiWidth, false),
		headerCall:  formatLine("Corrected", ansiWidth, false),
		headerUnlic: formatLine("Unlicensed", ansiWidth, false),
		headerHarm:  formatLine("Harmonics", ansiWidth, false),
		headerSys:   formatLine("System Log", ansiWidth, false),
	}
	frame := make([]string, ansiTotalRows)
	stats := make([]string, ansiStatsLines)
	for i := 0; i < ansiStatsLines; i++ {
		stats[i] = formatLine(fmt.Sprintf("S%02d", i+1), ansiWidth, false)
	}
	dropped := []string{formatLine("D1", ansiWidth, false)}
	calls := []string{formatLine("C1", ansiWidth, false)}
	unlic := []string{}
	harm := []string{}
	system := []string{formatLine("SYS1", ansiWidth, false)}

	c.fillFrame(frame, stats, dropped, calls, unlic, harm, system)

	if len(frame) != ansiTotalRows {
		t.Fatalf("frame length mismatch: got %d want %d", len(frame), ansiTotalRows)
	}
	if frame[0] != stats[0] || frame[ansiStatsLines-1] != stats[ansiStatsLines-1] {
		t.Fatalf("stats lines not in expected positions")
	}
	if frame[ansiStatsLines] != c.blankLine {
		t.Fatalf("expected blank line after stats")
	}
	if frame[ansiStatsLines+1] != c.headerDrop {
		t.Fatalf("expected dropped header after stats gap")
	}
	if frame[ansiStatsLines+2] != dropped[0] {
		t.Fatalf("expected first dropped line after dropped header")
	}
	callsBlankIdx := ansiStatsLines + 2 + ansiPaneLines
	if idx := callsBlankIdx; frame[idx] != c.blankLine {
		t.Fatalf("expected blank line before calls header (idx=%d)", idx)
	}
	if frame[callsBlankIdx+1] != c.headerCall {
		t.Fatalf("expected calls header after blank")
	}
	unlicBlankIdx := callsBlankIdx + 2 + ansiPaneLines
	if frame[unlicBlankIdx] != c.blankLine {
		t.Fatalf("expected blank line before unlicensed header (idx=%d)", unlicBlankIdx)
	}
	if frame[unlicBlankIdx+1] != c.headerUnlic {
		t.Fatalf("expected unlicensed header after blank")
	}
	harmBlankIdx := unlicBlankIdx + 2 + ansiPaneLines
	if frame[harmBlankIdx] != c.blankLine {
		t.Fatalf("expected blank line before harmonics header (idx=%d)", harmBlankIdx)
	}
	if frame[harmBlankIdx+1] != c.headerHarm {
		t.Fatalf("expected harmonics header after blank")
	}
	sysBlankIdx := harmBlankIdx + 2 + ansiPaneLines
	if frame[sysBlankIdx] != c.blankLine {
		t.Fatalf("expected blank line before system header (idx=%d)", sysBlankIdx)
	}
	if frame[sysBlankIdx+1] != c.headerSys {
		t.Fatalf("expected system header after blank")
	}
	if !strings.Contains(frame[sysBlankIdx+2], "SYS1") {
		t.Fatalf("expected system line after system header")
	}
}
