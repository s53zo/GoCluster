package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// focusable abstracts a focusable, optionally scrollable, text box.
type focusable interface {
	View() *tview.TextView
	SetFocused(focused bool)
	Scrollable() bool
}

// focusBox wraps a boxed TextView with focus styling metadata.
type focusBox struct {
	tv         *tview.TextView
	baseTitle  string
	scrollable bool
}

func newFocusBox(tv *tview.TextView, baseTitle string, scrollable bool) *focusBox {
	return &focusBox{
		tv:         tv,
		baseTitle:  baseTitle,
		scrollable: scrollable,
	}
}

func (b *focusBox) View() *tview.TextView {
	if b == nil {
		return nil
	}
	return b.tv
}

func (b *focusBox) SetFocused(focused bool) {
	if b == nil {
		return
	}
	applyFocusStyle(b.tv, b.baseTitle, focused)
}

func (b *focusBox) Scrollable() bool {
	if b == nil {
		return false
	}
	return b.scrollable
}

// focusGroup manages focus cycling and scroll handling for a set of boxes.
type focusGroup struct {
	items []focusable
	index int
}

func newFocusGroup(items ...focusable) focusGroup {
	filtered := make([]focusable, 0, len(items))
	for _, item := range items {
		if item == nil || item.View() == nil {
			continue
		}
		filtered = append(filtered, item)
	}
	return focusGroup{items: filtered}
}

func (g *focusGroup) set(app *tview.Application, idx int) {
	if g == nil || len(g.items) == 0 {
		return
	}
	if idx < 0 || idx >= len(g.items) {
		idx = 0
	}
	g.index = idx
	for i, item := range g.items {
		item.SetFocused(i == idx)
	}
	if app != nil {
		app.SetFocus(g.items[idx].View())
	}
}

func (g *focusGroup) cycle(app *tview.Application, delta int) {
	if g == nil || len(g.items) == 0 {
		return
	}
	next := g.index + delta
	if next < 0 {
		next = len(g.items) - 1
	} else if next >= len(g.items) {
		next = 0
	}
	g.set(app, next)
}

func (g *focusGroup) handleScroll(app *tview.Application, event *tcell.EventKey) bool {
	if g == nil || app == nil || event == nil {
		return false
	}
	focused := app.GetFocus()
	for _, item := range g.items {
		if item.Scrollable() && item.View() == focused {
			return scrollTextView(item.View(), event)
		}
	}
	return false
}

// streamPanel holds a bounded line history and renders it into a TextView.
// Concurrency: Append may be called from multiple goroutines; Render is called
// from the UI goroutine. Memory is bounded to max lines.
type streamPanel struct {
	*focusBox
	mu    sync.Mutex
	lines []string
	snap  []string
	head  int
	count int
	total uint64
	max   int
	dirty bool
}

func newStreamPanel(tv *tview.TextView, baseTitle string, max int) *streamPanel {
	if max <= 0 {
		max = 1
	}
	if tv != nil {
		tv.SetScrollable(true)
	}
	return &streamPanel{
		focusBox: newFocusBox(tv, baseTitle, true),
		lines:    make([]string, max),
		max:      max,
	}
}

func (p *streamPanel) Append(line string) {
	if p == nil || p.max <= 0 {
		return
	}
	p.mu.Lock()
	if p.count < p.max {
		pos := (p.head + p.count) % p.max
		p.lines[pos] = line
		p.count++
	} else {
		p.lines[p.head] = line
		p.head = (p.head + 1) % p.max
	}
	p.total++
	p.dirty = true
	p.mu.Unlock()
}

func (p *streamPanel) Render(app *tview.Application) {
	if p == nil || p.View() == nil {
		return
	}
	p.mu.Lock()
	if !p.dirty {
		p.mu.Unlock()
		return
	}
	count := p.count
	total := p.total
	head := p.head
	lines := p.lines
	if cap(p.snap) < count {
		p.snap = make([]string, count)
	} else {
		p.snap = p.snap[:count]
	}
	for i := 0; i < count; i++ {
		idx := (head + i) % p.max
		p.snap[i] = lines[idx]
	}
	snapshot := p.snap
	p.dirty = false
	p.mu.Unlock()

	var b strings.Builder
	for i := 0; i < len(snapshot); i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(snapshot[i])
	}
	if overflow := int(total) - len(snapshot); overflow > 0 {
		if len(snapshot) > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "... +%d more", overflow)
	}
	text := padLines(b.String())

	p.View().SetText(text)
	if app == nil || app.GetFocus() != p.View() {
		p.View().ScrollToEnd()
	}
}
