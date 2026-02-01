package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// VirtualList renders only visible rows from a snapshot.
type VirtualList struct {
	*tview.Box
	snapshot     []StyledEvent
	indices      []int
	visibleRows  int
	scrollOffset int
	rowCache     map[int]string
	timeFormat   string
}

func NewVirtualList() *VirtualList {
	return &VirtualList{
		Box:        tview.NewBox(),
		rowCache:   make(map[int]string),
		timeFormat: "15:04:05",
	}
}

func (v *VirtualList) SetTimeFormat(format string) {
	if format == "" {
		format = "15:04:05"
	}
	v.timeFormat = format
}

func (v *VirtualList) SetSnapshot(snapshot []StyledEvent, indices []int) {
	v.snapshot = snapshot
	v.indices = indices
	v.rowCache = make(map[int]string, v.visibleRows)
	maxOffset := maxInt(0, v.listLen()-v.visibleRows)
	if v.scrollOffset > maxOffset {
		v.scrollOffset = maxOffset
	}
	if v.scrollOffset < 0 {
		v.scrollOffset = 0
	}
}

func (v *VirtualList) listLen() int {
	if v.indices != nil {
		return len(v.indices)
	}
	return len(v.snapshot)
}

func (v *VirtualList) Draw(screen tcell.Screen) {
	v.Box.DrawForSubclass(screen, v)
	x, y, width, height := v.GetInnerRect()
	v.visibleRows = height
	if width <= 0 || height <= 0 {
		return
	}

	maxOffset := maxInt(0, v.listLen()-v.visibleRows)
	if v.scrollOffset > maxOffset {
		v.scrollOffset = maxOffset
	}
	if v.scrollOffset < 0 {
		v.scrollOffset = 0
	}

	for i := 0; i < v.visibleRows; i++ {
		row := v.scrollOffset + i
		if row >= v.listLen() {
			break
		}
		event := v.eventAt(row)
		line := v.formatRow(row, event, width)
		drawText(screen, x, y+i, width, line, styleForKind(event.Kind))
	}
}

func (v *VirtualList) eventAt(row int) StyledEvent {
	if v.indices == nil {
		return v.snapshot[row]
	}
	idx := v.indices[row]
	if idx < 0 || idx >= len(v.snapshot) {
		return StyledEvent{}
	}
	return v.snapshot[idx]
}

func (v *VirtualList) formatRow(row int, event StyledEvent, width int) string {
	if cached, ok := v.rowCache[row]; ok {
		return cached
	}
	ts := event.Timestamp.Format("15:04:05")
	if v.timeFormat != "" {
		ts = event.Timestamp.Format(v.timeFormat)
	}
	label := event.Kind.Label()
	msgWidth := width - len(ts) - len(label) - 4
	if msgWidth < 0 {
		msgWidth = 0
	}
	msg := truncateRunes(event.Message, msgWidth)
	line := strings.TrimSpace(ts + " [" + label + "] " + msg)
	v.rowCache[row] = line
	return line
}

func (v *VirtualList) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	v.scrollOffset -= n
	if v.scrollOffset < 0 {
		v.scrollOffset = 0
	}
}

func (v *VirtualList) ScrollDown(n int) {
	if n <= 0 {
		return
	}
	maxOffset := maxInt(0, v.listLen()-v.visibleRows)
	v.scrollOffset += n
	if v.scrollOffset > maxOffset {
		v.scrollOffset = maxOffset
	}
}

func (v *VirtualList) ScrollToStart() {
	v.scrollOffset = 0
}

func (v *VirtualList) ScrollToEnd() {
	v.scrollOffset = maxInt(0, v.listLen()-v.visibleRows)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	count := 0
	for _, r := range s {
		if count >= max {
			break
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}

func drawText(screen tcell.Screen, x, y, width int, text string, style tcell.Style) {
	if width <= 0 {
		return
	}
	runes := []rune(text)
	for i := 0; i < width; i++ {
		ch := ' '
		if i < len(runes) {
			ch = runes[i]
		}
		screen.SetContent(x+i, y, ch, nil, style)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func styleForKind(kind EventKind) tcell.Style {
	switch kind {
	case EventDrop:
		return tcell.StyleDefault.Foreground(tcell.ColorRed)
	case EventCorrection:
		return tcell.StyleDefault.Foreground(tcell.ColorGreen)
	case EventUnlicensed:
		return tcell.StyleDefault.Foreground(tcell.ColorYellow)
	case EventHarmonic:
		return tcell.StyleDefault.Foreground(tcell.ColorDarkOrange)
	case EventReputation:
		return tcell.StyleDefault.Foreground(tcell.ColorMediumPurple)
	case EventSystem:
		return tcell.StyleDefault.Foreground(tcell.ColorWhite)
	default:
		return tcell.StyleDefault
	}
}
