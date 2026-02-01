package ui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"dxcluster/config"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	maxSearchResults = 1000
)

// DashboardV2 implements the page-based tview UI.
type DashboardV2 struct {
	app       *tview.Application
	pages     *tview.Pages
	scheduler *frameScheduler

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	ready chan struct{}

	snapshotMu sync.RWMutex
	snapshot   Snapshot
	statsMu    sync.Mutex
	statsLines []string

	eventsBuf *BoundedEventBuffer
	debugBuf  *BoundedEventBuffer

	overviewView *tview.TextView
	ingestView   *tview.TextView
	networkView  *tview.TextView

	eventsPage *eventPage
	debugPage  *eventPage
	pipeline   *pipelinePage

	pageOrder []string
	pageIndex int
	helpShown bool
	metrics   *Metrics
}

// NewDashboardV2 constructs the v2 dashboard if enabled.
func NewDashboardV2(cfg config.UIConfig, enable bool) *DashboardV2 {
	if !enable {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	app := tview.NewApplication().EnableMouse(cfg.V2.EnableMouse)
	pages := tview.NewPages()
	ready := make(chan struct{})
	var once sync.Once
	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		once.Do(func() { close(ready) })
		return false
	})

	metrics := NewMetrics()
	d := &DashboardV2{
		app:       app,
		pages:     pages,
		ctx:       ctx,
		cancel:    cancel,
		ready:     ready,
		pageOrder: cfg.V2.Pages,
		metrics:   metrics,
	}

	eventPolicy := DropPolicy{
		MaxMessageBytes:  cfg.V2.EventBuffer.MaxMessageBytes,
		EvictOnByteLimit: cfg.V2.EventBuffer.EvictOnByteLimit,
		LogDrops:         cfg.V2.EventBuffer.LogDrops,
	}
	debugPolicy := DropPolicy{
		MaxMessageBytes:  cfg.V2.DebugBuffer.MaxMessageBytes,
		EvictOnByteLimit: cfg.V2.DebugBuffer.EvictOnByteLimit,
		LogDrops:         cfg.V2.DebugBuffer.LogDrops,
	}
	eventMaxBytes := int64(cfg.V2.EventBuffer.MaxBytesMB) * 1024 * 1024
	debugMaxBytes := int64(cfg.V2.DebugBuffer.MaxBytesMB) * 1024 * 1024
	d.eventsBuf = NewBoundedEventBuffer("events", cfg.V2.EventBuffer.MaxEvents, eventMaxBytes, eventPolicy, log.Printf)
	d.debugBuf = NewBoundedEventBuffer("debug", cfg.V2.DebugBuffer.MaxEvents, debugMaxBytes, debugPolicy, log.Printf)

	d.overviewView = newPageTextView("Overview")
	d.ingestView = newPageTextView("Ingest")
	d.networkView = newPageTextView("Network")

	d.eventsPage = newEventPage(ctx, "Events", d.eventsBuf, true, metrics)
	d.debugPage = newEventPage(ctx, "Debug", d.debugBuf, false, metrics)
	d.pipeline = newPipelinePage(ctx, d.eventsBuf)

	d.pages.AddPage("overview", d.overviewView, true, false)
	d.pages.AddPage("ingest", d.ingestView, true, false)
	d.pages.AddPage("pipeline", d.pipeline.root, true, false)
	d.pages.AddPage("network", d.networkView, true, false)
	d.pages.AddPage("events", d.eventsPage.root, true, false)
	d.pages.AddPage("debug", d.debugPage.root, true, false)

	help := buildHelpOverlay()
	d.pages.AddPage("help", help, true, false)

	d.scheduler = newFrameScheduler(app, cfg.V2.TargetFPS, 100*time.Millisecond, metrics.ObserveRender)
	d.scheduler.Start()

	d.installKeybindings(cfg)
	d.installRoot(cfg)

	go func() {
		if err := app.Run(); err != nil {
			log.Printf("UI: tview-v2 error: %v", err)
		}
	}()

	return d
}

func (d *DashboardV2) installRoot(cfg config.UIConfig) {
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(d.pages, 0, 1, true).
		AddItem(buildFooter(), 1, 0, false)
	d.app.SetRoot(root, true)
	d.showPage("overview")
}

func (d *DashboardV2) installKeybindings(cfg config.UIConfig) {
	d.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if d.helpShown {
			if event.Key() == tcell.KeyEsc || event.Key() == tcell.KeyF1 || event.Rune() == 'h' || event.Rune() == '?' {
				d.toggleHelp(false)
				return nil
			}
		}

		if pageName, _ := d.pages.GetFrontPage(); pageName == "events" {
			if d.eventsPage.handleInput(event, d.app) {
				return nil
			}
		} else if pageName == "pipeline" {
			if d.pipeline.handleInput(event) {
				return nil
			}
		} else if pageName == "debug" {
			if d.debugPage.handleInput(event, d.app) {
				return nil
			}
		}

		switch event.Key() {
		case tcell.KeyF1:
			d.toggleHelp(!d.helpShown)
			return nil
		case tcell.KeyF2:
			d.showPage("overview")
			return nil
		case tcell.KeyF3:
			d.showPage("ingest")
			return nil
		case tcell.KeyF4:
			d.showPage("pipeline")
			return nil
		case tcell.KeyF5:
			d.showPage("network")
			return nil
		case tcell.KeyF6:
			d.showPage("events")
			return nil
		case tcell.KeyF7:
			d.showPage("debug")
			return nil
		case tcell.KeyTab:
			d.nextPage()
			return nil
		case tcell.KeyBacktab:
			d.prevPage()
			return nil
		case tcell.KeyCtrlC:
			d.Stop()
			return nil
		}

		switch event.Rune() {
		case 'q', 'Q':
			d.Stop()
			return nil
		case 'h', '?':
			d.toggleHelp(!d.helpShown)
			return nil
		}

		if cfg.V2.Keybindings.UseAlternatives {
			switch event.Rune() {
			case 'o':
				d.showPage("overview")
				return nil
			case 'i':
				d.showPage("ingest")
				return nil
			case 'p':
				d.showPage("pipeline")
				return nil
			case 'n':
				d.showPage("network")
				return nil
			case 'e':
				d.showPage("events")
				return nil
			case 'd':
				d.showPage("debug")
				return nil
			}
		}

		return event
	})
}

func (d *DashboardV2) toggleHelp(show bool) {
	d.helpShown = show
	d.pages.ShowPage("help")
	d.pages.SendToFront("help")
	if !show {
		d.pages.HidePage("help")
	}
}

func (d *DashboardV2) showPage(name string) {
	if !d.pageEnabled(name) {
		if len(d.pageOrder) == 0 {
			return
		}
		name = d.pageOrder[0]
	}
	for i, page := range d.pageOrder {
		if page == name {
			d.pageIndex = i
			break
		}
	}
	d.pages.SwitchToPage(name)
	if d.metrics != nil {
		d.metrics.PageSwitch()
	}
	switch name {
	case "events":
		d.app.SetFocus(d.eventsPage.list)
	case "debug":
		d.app.SetFocus(d.debugPage.list)
	case "overview":
		d.app.SetFocus(d.overviewView)
	case "ingest":
		d.app.SetFocus(d.ingestView)
	case "pipeline":
		d.app.SetFocus(d.pipeline.corrected)
	case "network":
		d.app.SetFocus(d.networkView)
	}
}

func (d *DashboardV2) nextPage() {
	if len(d.pageOrder) == 0 {
		return
	}
	d.pageIndex = (d.pageIndex + 1) % len(d.pageOrder)
	d.showPage(d.pageOrder[d.pageIndex])
}

func (d *DashboardV2) prevPage() {
	if len(d.pageOrder) == 0 {
		return
	}
	d.pageIndex--
	if d.pageIndex < 0 {
		d.pageIndex = len(d.pageOrder) - 1
	}
	d.showPage(d.pageOrder[d.pageIndex])
}

func (d *DashboardV2) pageEnabled(name string) bool {
	for _, page := range d.pageOrder {
		if page == name {
			return true
		}
	}
	return false
}

func (d *DashboardV2) WaitReady() {
	if d == nil || d.ready == nil {
		return
	}
	<-d.ready
}

func (d *DashboardV2) Stop() {
	if d == nil {
		return
	}
	d.cancel()
	if d.scheduler != nil {
		d.scheduler.Stop()
	}
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		log.Printf("UI: dashboard stop timeout, some goroutines may leak")
	}
	if d.app != nil {
		d.app.Stop()
	}
}

func (d *DashboardV2) SetStats(lines []string) {
	if d == nil {
		return
	}
	d.statsMu.Lock()
	d.statsLines = append(d.statsLines[:0], lines...)
	d.statsMu.Unlock()
	d.scheduler.Schedule("stats", func() {
		d.renderSnapshot()
	})
}

func (d *DashboardV2) SetSnapshot(snapshot Snapshot) {
	if d == nil {
		return
	}
	d.snapshotMu.Lock()
	d.snapshot = snapshot
	d.snapshotMu.Unlock()
	d.scheduler.Schedule("snapshot", func() {
		d.renderSnapshot()
	})
}

func (d *DashboardV2) renderSnapshot() {
	snap := d.snapshotCopy()
	if len(snap.OverviewLines) == 0 {
		d.statsMu.Lock()
		snap.OverviewLines = append([]string{}, d.statsLines...)
		d.statsMu.Unlock()
	}
	d.overviewView.SetText(strings.Join(snap.OverviewLines, "\n"))
	d.ingestView.SetText(strings.Join(snap.IngestLines, "\n"))
	d.networkView.SetText(strings.Join(snap.NetworkLines, "\n"))

	d.eventsPage.refresh()
	d.debugPage.refresh()
	d.pipeline.refresh()
}

func (d *DashboardV2) snapshotCopy() Snapshot {
	d.snapshotMu.RLock()
	defer d.snapshotMu.RUnlock()
	copyLines := func(lines []string) []string {
		if len(lines) == 0 {
			return nil
		}
		out := make([]string, len(lines))
		copy(out, lines)
		return out
	}
	return Snapshot{
		GeneratedAt:   d.snapshot.GeneratedAt,
		OverviewLines: copyLines(d.snapshot.OverviewLines),
		IngestLines:   copyLines(d.snapshot.IngestLines),
		PipelineLines: copyLines(d.snapshot.PipelineLines),
		NetworkLines:  copyLines(d.snapshot.NetworkLines),
	}
}

func (d *DashboardV2) AppendDropped(line string) {
	d.appendEvent(EventDrop, line, d.eventsBuf)
}

func (d *DashboardV2) AppendCall(line string) {
	d.appendEvent(EventCorrection, line, d.eventsBuf)
}

func (d *DashboardV2) AppendUnlicensed(line string) {
	d.appendEvent(EventUnlicensed, line, d.eventsBuf)
}

func (d *DashboardV2) AppendHarmonic(line string) {
	d.appendEvent(EventHarmonic, line, d.eventsBuf)
}

func (d *DashboardV2) AppendReputation(line string) {
	d.appendEvent(EventReputation, line, d.eventsBuf)
}

func (d *DashboardV2) AppendSystem(line string) {
	d.appendEvent(EventSystem, line, d.eventsBuf)
	d.appendEvent(EventSystem, line, d.debugBuf)
}

func (d *DashboardV2) appendEvent(kind EventKind, line string, buf *BoundedEventBuffer) {
	if d == nil || buf == nil {
		return
	}
	event := StyledEvent{
		Timestamp: time.Now().UTC(),
		Kind:      kind,
		Message:   stripTags(line),
	}
	if buf.Append(event) {
		d.scheduler.Schedule("events", func() {
			d.eventsPage.refresh()
			d.debugPage.refresh()
			d.pipeline.refresh()
		})
	}
}

func (d *DashboardV2) SystemWriter() io.Writer {
	if d == nil {
		return nil
	}
	return &paneWriter{dash: d}
}

type paneWriter struct {
	dash *DashboardV2
	buf  []byte
	mu   sync.Mutex
}

func (w *paneWriter) Write(p []byte) (int, error) {
	if w == nil || w.dash == nil {
		return len(p), nil
	}
	w.mu.Lock()
	w.buf = append(w.buf, p...)
	data := w.buf
	w.mu.Unlock()

	for {
		idx := bytes.IndexByte(data, '\n')
		if idx == -1 {
			break
		}
		line := string(bytes.TrimRight(data[:idx], "\r"))
		w.dash.AppendSystem(line)
		data = data[idx+1:]
	}
	w.mu.Lock()
	w.buf = data
	w.mu.Unlock()
	return len(p), nil
}

func newPageTextView(title string) *tview.TextView {
	tv := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	if title != "" {
		tv.SetTitle(title).SetTitleAlign(tview.AlignLeft)
	}
	return tv
}

func buildFooter() *tview.TextView {
	return tview.NewTextView().SetText("[F1]Help  [F2]Overview  [F3]Ingest  [F4]Pipeline  [F5]Network  [F6]Events  [F7]Debug  [Q]Quit")
}

func buildHelpOverlay() tview.Primitive {
	help := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	help.SetText(strings.TrimSpace(`
KEYBOARD HELP

NAVIGATION
  F1  Help   F2 Overview   F3 Ingest   F4 Pipeline   F5 Network   F6 Events   F7 Debug
  Tab Next page   Shift+Tab Previous page   q / Ctrl+C Quit

EVENTS/DEBUG
  ↑/↓ or k/j Scroll   PageUp/Down Fast scroll   Home/End Top/Bottom
  1-6 Filter tabs (Events)   / Search   Esc Clear search / close
`))
	help.SetBorder(true).SetTitle("Help")
	container := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(help, 15, 1, true).
			AddItem(nil, 0, 1, false),
			60, 1, true).
		AddItem(nil, 0, 1, false)
	return container
}

type eventPage struct {
	root   *tview.Flex
	header *tview.TextView
	footer *tview.TextView
	search *tview.InputField
	list   *VirtualList

	buffer        *BoundedEventBuffer
	filterEnabled bool
	filterIndex   int
	searchFilter  *SearchFilter
	title         string
	metrics       *Metrics

	scratch []StyledEvent
}

func newEventPage(ctx context.Context, title string, buffer *BoundedEventBuffer, filterEnabled bool, metrics *Metrics) *eventPage {
	header := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	footer := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	search := tview.NewInputField().SetLabel("Search: ").SetFieldWidth(30)
	list := NewVirtualList()
	root := tview.NewFlex().SetDirection(tview.FlexRow)

	headerRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(header, 0, 3, false).
		AddItem(search, 0, 1, false)
	root.AddItem(headerRow, 1, 0, false)
	root.AddItem(list, 0, 1, true)
	root.AddItem(footer, 1, 0, false)

	page := &eventPage{
		root:          root,
		header:        header,
		footer:        footer,
		search:        search,
		list:          list,
		buffer:        buffer,
		filterEnabled: filterEnabled,
		filterIndex:   0,
		searchFilter:  NewSearchFilter(ctx),
		title:         title,
		metrics:       metrics,
	}

	search.SetChangedFunc(func(text string) {
		page.searchFilter.SetQuery(text, func() {
			page.refresh()
		})
	})

	page.updateHeader()
	return page
}

func (p *eventPage) handleInput(event *tcell.EventKey, app *tview.Application) bool {
	if p == nil || event == nil {
		return false
	}
	switch event.Key() {
	case tcell.KeyUp:
		p.list.ScrollUp(1)
		return true
	case tcell.KeyDown:
		p.list.ScrollDown(1)
		return true
	case tcell.KeyPgUp:
		p.list.ScrollUp(10)
		return true
	case tcell.KeyPgDn:
		p.list.ScrollDown(10)
		return true
	case tcell.KeyHome:
		p.list.ScrollToStart()
		return true
	case tcell.KeyEnd:
		p.list.ScrollToEnd()
		return true
	case tcell.KeyEsc:
		p.search.SetText("")
		if app != nil {
			app.SetFocus(p.list)
		}
		return true
	}

	switch event.Rune() {
	case '/':
		if app != nil {
			app.SetFocus(p.search)
		}
		return true
	case 'k':
		p.list.ScrollUp(1)
		return true
	case 'j':
		p.list.ScrollDown(1)
		return true
	}

	if p.filterEnabled {
		switch event.Rune() {
		case '1', '2', '3', '4', '5', '6':
			p.filterIndex = int(event.Rune() - '1')
			p.refresh()
			return true
		}
	}
	return false
}

func (p *eventPage) refresh() {
	if p == nil || p.buffer == nil {
		return
	}
	snapshot := p.buffer.SnapshotInto(p.scratch)
	p.scratch = snapshot.Events

	indices := p.filterSnapshot(snapshot.Events)
	p.list.SetSnapshot(snapshot.Events, indices)
	p.updateFooter(snapshot.Events, indices)
}

func (p *eventPage) filterSnapshot(events []StyledEvent) []int {
	if len(events) == 0 {
		return nil
	}
	query := p.searchFilter.ActiveQuery()
	start := time.Time{}
	if query != "" && p.metrics != nil {
		start = time.Now()
	}
	indices := make([]int, 0, len(events))
	for i, event := range events {
		if p.filterEnabled && !matchFilter(p.filterIndex, event.Kind) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(event.Message), query) {
			continue
		}
		indices = append(indices, i)
		if query != "" && len(indices) >= maxSearchResults {
			break
		}
	}
	if !start.IsZero() {
		p.metrics.ObserveSearch(time.Since(start))
	}
	if len(indices) == len(events) && query == "" && !p.filterEnabled {
		return nil
	}
	return indices
}

func (p *eventPage) updateHeader() {
	if !p.filterEnabled {
		p.header.SetText(fmt.Sprintf("%s", p.title))
		return
	}
	labels := []string{"All", "Drops", "Corrections", "Unlicensed", "Harmonics", "System"}
	var b strings.Builder
	for i, label := range labels {
		if i == p.filterIndex {
			fmt.Fprintf(&b, "[yellow]%s[-] ", label)
		} else {
			fmt.Fprintf(&b, "%s ", label)
		}
	}
	p.header.SetText(strings.TrimSpace(b.String()))
}

func (p *eventPage) updateFooter(events []StyledEvent, indices []int) {
	p.updateHeader()
	count, maxCount, bytes, maxBytes := p.buffer.BufferUsage()
	drops := p.buffer.DropSnapshot()
	filtered := len(events)
	if indices != nil {
		filtered = len(indices)
	}
	p.footer.SetText(fmt.Sprintf("Showing: %d  Buffer: %d/%d  Bytes: %d/%d  Drops: O:%d E:%d B:%d",
		filtered, count, maxCount, bytes, maxBytes, drops.Oversized, drops.Evicted, drops.ByteLimit))
}

func matchFilter(filterIndex int, kind EventKind) bool {
	switch filterIndex {
	case 0:
		return true
	case 1:
		return kind == EventDrop || kind == EventReputation
	case 2:
		return kind == EventCorrection
	case 3:
		return kind == EventUnlicensed
	case 4:
		return kind == EventHarmonic
	case 5:
		return kind == EventSystem
	default:
		return true
	}
}

type pipelinePage struct {
	root       *tview.Flex
	corrected  *VirtualList
	unlicensed *VirtualList
	harmonics  *VirtualList
	reputation *VirtualList
	buffer     *BoundedEventBuffer
	scratch    []StyledEvent
}

func newPipelinePage(ctx context.Context, buffer *BoundedEventBuffer) *pipelinePage {
	header := tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetText("PIPELINE")
	root := tview.NewFlex().SetDirection(tview.FlexRow)
	root.AddItem(header, 1, 0, false)

	makeColumn := func(title string) (*tview.Flex, *VirtualList) {
		label := tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetText(title)
		list := NewVirtualList()
		list.SetTimeFormat("15:04")
		col := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(label, 1, 0, false).
			AddItem(list, 0, 1, true)
		return col, list
	}

	cols := tview.NewFlex().SetDirection(tview.FlexColumn)
	colCorrected, correctedList := makeColumn("Corrected")
	colUnlicensed, unlicensedList := makeColumn("Unlicensed")
	colHarmonics, harmonicsList := makeColumn("Harmonics")
	colReputation, reputationList := makeColumn("Reputation")

	cols.AddItem(colCorrected, 0, 1, true)
	cols.AddItem(colUnlicensed, 0, 1, false)
	cols.AddItem(colHarmonics, 0, 1, false)
	cols.AddItem(colReputation, 0, 1, false)

	root.AddItem(cols, 0, 1, true)

	return &pipelinePage{
		root:       root,
		corrected:  correctedList,
		unlicensed: unlicensedList,
		harmonics:  harmonicsList,
		reputation: reputationList,
		buffer:     buffer,
	}
}

func (p *pipelinePage) refresh() {
	if p == nil || p.buffer == nil {
		return
	}
	snap := p.buffer.SnapshotInto(p.scratch)
	p.scratch = snap.Events
	p.corrected.SetSnapshot(snap.Events, filterIndicesByKind(snap.Events, EventCorrection))
	p.unlicensed.SetSnapshot(snap.Events, filterIndicesByKind(snap.Events, EventUnlicensed))
	p.harmonics.SetSnapshot(snap.Events, filterIndicesByKind(snap.Events, EventHarmonic))
	p.reputation.SetSnapshot(snap.Events, filterIndicesByKind(snap.Events, EventReputation))
}

func (p *pipelinePage) handleInput(event *tcell.EventKey) bool {
	if p == nil || event == nil {
		return false
	}
	scrollAll := func(fn func(*VirtualList)) {
		fn(p.corrected)
		fn(p.unlicensed)
		fn(p.harmonics)
		fn(p.reputation)
	}
	switch event.Key() {
	case tcell.KeyUp:
		scrollAll(func(v *VirtualList) { v.ScrollUp(1) })
		return true
	case tcell.KeyDown:
		scrollAll(func(v *VirtualList) { v.ScrollDown(1) })
		return true
	case tcell.KeyPgUp:
		scrollAll(func(v *VirtualList) { v.ScrollUp(10) })
		return true
	case tcell.KeyPgDn:
		scrollAll(func(v *VirtualList) { v.ScrollDown(10) })
		return true
	case tcell.KeyHome:
		scrollAll(func(v *VirtualList) { v.ScrollToStart() })
		return true
	case tcell.KeyEnd:
		scrollAll(func(v *VirtualList) { v.ScrollToEnd() })
		return true
	}
	switch event.Rune() {
	case 'k':
		scrollAll(func(v *VirtualList) { v.ScrollUp(1) })
		return true
	case 'j':
		scrollAll(func(v *VirtualList) { v.ScrollDown(1) })
		return true
	}
	return false
}

func filterIndicesByKind(events []StyledEvent, kind EventKind) []int {
	if len(events) == 0 {
		return nil
	}
	indices := make([]int, 0, len(events))
	for i, event := range events {
		if event.Kind == kind {
			indices = append(indices, i)
		}
	}
	if len(indices) == len(events) {
		return nil
	}
	return indices
}

func stripTags(s string) string {
	if s == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"[red]", "",
		"[green]", "",
		"[yellow]", "",
		"[blue]", "",
		"[magenta]", "",
		"[cyan]", "",
		"[white]", "",
		"[-]", "",
	)
	return replacer.Replace(s)
}
