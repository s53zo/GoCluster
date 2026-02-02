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
	maxSearchResults   = 1000
	paneWriterMaxBytes = 64 * 1024
)

const (
	accentTag   = "[#ff69b4]"
	accentReset = "[-]"
)

var (
	uiBorderColor = tcell.ColorGray
	uiTitleColor  = tcell.ColorHotPink
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

	overviewRoot      *tview.Flex
	overviewHdr       *tview.TextView
	overviewMem       *tview.TextView
	overviewIngest    *tview.TextView
	overviewPipeline  *tview.TextView
	overviewCaches    *tview.TextView
	overviewPath      *tview.TextView
	overviewNetwork   *tview.TextView
	ingestRoot        *tview.Flex
	ingestHdr         *tview.TextView
	ingestIngest      *tview.TextView
	ingestValidation  *tview.TextView
	ingestUnlicensed  *tview.TextView
	pipelineRoot      *tview.Flex
	pipelineHdr       *tview.TextView
	pipelineQuality   *tview.TextView
	pipelineCorrected *tview.TextView
	pipelineHarmonics *tview.TextView
	networkView       *tview.TextView

	eventsPage *eventPage
	debugPage  *eventPage
	pipeline   *pipelinePage

	pageOrder []string
	pageIndex int
	helpShown bool
	metrics   *Metrics

	validationMu    sync.Mutex
	validationLines []string
	validationTotal uint64
	validationMax   int

	unlicensedMu    sync.Mutex
	unlicensedLines []string
	unlicensedTotal uint64
	unlicensedMax   int

	ingestFocus         int
	validationTitleBase string
	unlicensedTitleBase string
	overviewNetworkBase string
	overviewFocus       int
	pipelineFocus       int
	correctedTitleBase  string
	harmonicsTitleBase  string

	correctedMu    sync.Mutex
	correctedLines []string
	correctedTotal uint64
	correctedMax   int

	harmonicsMu    sync.Mutex
	harmonicsLines []string
	harmonicsTotal uint64
	harmonicsMax   int

	pagePresent map[string]bool
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
		app:         app,
		pages:       pages,
		ctx:         ctx,
		cancel:      cancel,
		ready:       ready,
		pageOrder:   cfg.V2.Pages,
		metrics:     metrics,
		pagePresent: make(map[string]bool),
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

	d.overviewHdr = newBoxedTextView("Overview")
	d.overviewMem = newBoxedTextView("Memory / GC")
	d.overviewIngest = newBoxedTextView("Ingest Rates (per min)")
	d.overviewPipeline = newBoxedTextView("Pipeline Quality")
	d.overviewCaches = newBoxedTextView("Caches & Data Freshness")
	d.overviewPath = newBoxedTextView("Path Predictions")
	d.overviewNetworkBase = "Network"
	d.overviewNetwork = newBoxedTextView(d.overviewNetworkBase)
	d.overviewNetwork.SetScrollable(true)
	d.seedOverviewPlaceholders()
	d.overviewRoot = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(d.overviewHdr, 3, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.overviewMem, 3, 0, false).
		AddItem(newSpacer(), 1, 0, false)
	addOverviewTopSections(d.overviewRoot, d.overviewIngest)
	d.overviewRoot.
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.overviewPipeline, 4, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.overviewCaches, 10, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.overviewPath, 3, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.overviewNetwork, 0, 1, false)
	d.ingestHdr = newBoxedTextView("Overview")
	d.ingestIngest = newBoxedTextView("Ingest Rates (per min)")
	d.validationTitleBase = "Validation"
	d.unlicensedTitleBase = "Unlicensed"
	d.ingestValidation = newBoxedTextView(d.validationTitleBase)
	d.ingestValidation.SetScrollable(true)
	d.ingestValidation.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if d.handleIngestScroll(event) {
			return nil
		}
		return event
	})
	d.validationMax = 200
	d.ingestUnlicensed = newBoxedTextView(d.unlicensedTitleBase)
	d.ingestUnlicensed.SetScrollable(true)
	d.ingestUnlicensed.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if d.handleIngestScroll(event) {
			return nil
		}
		return event
	})
	d.unlicensedMax = 200
	d.seedIngestPlaceholders()
	d.ingestRoot = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(d.ingestHdr, 3, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.ingestIngest, 6, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.ingestValidation, 28, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.ingestUnlicensed, 28, 0, false)

	d.pipelineHdr = newBoxedTextView("Overview")
	d.pipelineQuality = newBoxedTextView("Pipeline Quality")
	d.correctedTitleBase = "Corrected"
	d.harmonicsTitleBase = "Harmonics"
	d.pipelineCorrected = newBoxedTextView(d.correctedTitleBase)
	d.pipelineCorrected.SetScrollable(true)
	d.pipelineCorrected.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if d.handlePipelineScroll(event) {
			return nil
		}
		return event
	})
	d.correctedMax = 200
	d.pipelineHarmonics = newBoxedTextView(d.harmonicsTitleBase)
	d.pipelineHarmonics.SetScrollable(true)
	d.pipelineHarmonics.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if d.handlePipelineScroll(event) {
			return nil
		}
		return event
	})
	d.harmonicsMax = 200
	d.seedPipelinePlaceholders()
	d.pipelineRoot = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(d.pipelineHdr, 3, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.pipelineQuality, 4, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.pipelineCorrected, 28, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.pipelineHarmonics, 28, 0, false)

	d.addPage("overview", d.overviewRoot, true, false)
	d.addPage("ingest", d.ingestRoot, true, false)
	d.addPage("pipeline", d.pipelineRoot, true, false)

	help := buildHelpOverlay()
	d.addPage("help", help, true, false)

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
	d.showFirstAvailablePage()
}

func (d *DashboardV2) installKeybindings(cfg config.UIConfig) {
	d.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if d.helpShown {
			if event.Key() == tcell.KeyEsc || event.Key() == tcell.KeyF1 || event.Rune() == 'h' || event.Rune() == '?' {
				d.toggleHelp(false)
				return nil
			}
		}

		if pageName, _ := d.pages.GetFrontPage(); pageName == "ingest" {
			if d.handleIngestScroll(event) {
				return nil
			}
		} else if pageName == "overview" {
			if d.handleOverviewScroll(event) {
				return nil
			}
		} else if pageName == "pipeline" {
			if d.handlePipelineScroll(event) {
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
		case tcell.KeyTab:
			if pageName, _ := d.pages.GetFrontPage(); pageName == "ingest" {
				d.cycleIngestFocus(1)
			} else if pageName == "overview" {
				d.cycleOverviewFocus(1)
			} else if pageName == "pipeline" {
				d.cyclePipelineFocus(1)
			} else {
				d.nextPage()
			}
			return nil
		case tcell.KeyBacktab:
			if pageName, _ := d.pages.GetFrontPage(); pageName == "ingest" {
				d.cycleIngestFocus(-1)
			} else if pageName == "overview" {
				d.cycleOverviewFocus(-1)
			} else if pageName == "pipeline" {
				d.cyclePipelineFocus(-1)
			} else {
				d.prevPage()
			}
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
	if !d.pageEnabled(name) || !d.pageAvailable(name) {
		return
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
		if d.eventsPage != nil && d.eventsPage.list != nil {
			d.app.SetFocus(d.eventsPage.list)
		}
	case "debug":
		if d.debugPage != nil && d.debugPage.list != nil {
			d.app.SetFocus(d.debugPage.list)
		}
	case "overview":
		d.app.SetFocus(d.overviewRoot)
	case "ingest":
		d.setIngestFocus(0)
	case "pipeline":
		d.setPipelineFocus(0)
	case "network":
		if d.networkView != nil {
			d.app.SetFocus(d.networkView)
		}
	}
}

func (d *DashboardV2) showFirstAvailablePage() {
	if d == nil {
		return
	}
	if name, ok := d.firstAvailablePage(); ok {
		d.showPage(name)
	}
}

func (d *DashboardV2) firstAvailablePage() (string, bool) {
	if d == nil {
		return "", false
	}
	for _, name := range d.pageOrder {
		if d.pageAvailable(name) {
			return name, true
		}
	}
	return "", false
}

func (d *DashboardV2) nextPage() {
	if len(d.pageOrder) == 0 {
		return
	}
	d.cyclePage(1)
}

func (d *DashboardV2) prevPage() {
	if len(d.pageOrder) == 0 {
		return
	}
	d.cyclePage(-1)
}

func (d *DashboardV2) pageEnabled(name string) bool {
	for _, page := range d.pageOrder {
		if page == name {
			return true
		}
	}
	return false
}

func (d *DashboardV2) pageAvailable(name string) bool {
	if d == nil {
		return false
	}
	return d.pagePresent[name]
}

func (d *DashboardV2) addPage(name string, page tview.Primitive, resize, visible bool) {
	if d == nil || d.pages == nil || page == nil || name == "" {
		return
	}
	d.pages.AddPage(name, page, resize, visible)
	d.pagePresent[name] = true
}

func (d *DashboardV2) cyclePage(delta int) {
	if d == nil || len(d.pageOrder) == 0 {
		return
	}
	for i := 0; i < len(d.pageOrder); i++ {
		d.pageIndex += delta
		if d.pageIndex < 0 {
			d.pageIndex = len(d.pageOrder) - 1
		} else if d.pageIndex >= len(d.pageOrder) {
			d.pageIndex = 0
		}
		name := d.pageOrder[d.pageIndex]
		if d.pageAvailable(name) {
			d.showPage(name)
			return
		}
	}
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

func (d *DashboardV2) UpdateNetworkStatus(summaryLine string, clientLines []string) {
	if d == nil {
		return
	}
	lines := make([]string, 0, 1+len(clientLines))
	if summaryLine != "" {
		lines = append(lines, summaryLine)
	}
	lines = append(lines, clientLines...)
	text := padLines(strings.Join(lines, "\n"))
	d.scheduler.Schedule("network", func() {
		if d.overviewNetwork != nil {
			d.overviewNetwork.SetText(text)
			if d.overviewRoot != nil {
				const (
					networkMaxRows = 10
					baseLines      = 4
					overflowLine   = 1
				)
				maxHeight := baseLines + networkMaxRows + overflowLine + 2
				height := len(lines) + 2
				if height > maxHeight {
					height = maxHeight
				}
				if height < 3 {
					height = 3
				}
				d.overviewRoot.ResizeItem(d.overviewNetwork, height, 0)
			}
		}
	})
}

func (d *DashboardV2) renderSnapshot() {
	snap := d.snapshotCopy()
	if len(snap.OverviewLines) == 0 {
		d.statsMu.Lock()
		snap.OverviewLines = append([]string{}, d.statsLines...)
		d.statsMu.Unlock()
	}
	d.updateOverviewBoxes(snap.OverviewLines)
	d.updateIngestBoxes(snap.OverviewLines)
	d.updatePipelineBoxes(snap.OverviewLines)

	// Only overview + ingest + pipeline pages are active.
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
	if strings.HasPrefix(line, "CTY drop:") {
		d.appendValidation(line)
	}
	d.appendEvent(EventDrop, line, d.eventsBuf)
}

func (d *DashboardV2) AppendCall(line string) {
	d.appendCorrectedStream(line)
	d.appendEvent(EventCorrection, line, d.eventsBuf)
}

func (d *DashboardV2) AppendUnlicensed(line string) {
	d.appendUnlicensedStream(line)
	d.appendEvent(EventUnlicensed, line, d.eventsBuf)
}

func (d *DashboardV2) AppendHarmonic(line string) {
	d.appendHarmonicsStream(line)
	d.appendEvent(EventHarmonic, line, d.eventsBuf)
}

func (d *DashboardV2) appendValidation(line string) {
	if d == nil || d.ingestValidation == nil {
		return
	}
	d.validationMu.Lock()
	d.validationLines = append(d.validationLines, line)
	d.validationTotal++
	overflow := 0
	if d.validationMax > 0 && len(d.validationLines) > d.validationMax {
		d.validationLines = d.validationLines[len(d.validationLines)-d.validationMax:]
	}
	lines := append([]string{}, d.validationLines...)
	if d.validationTotal > uint64(len(lines)) {
		overflow = int(d.validationTotal - uint64(len(lines)))
	}
	d.validationMu.Unlock()
	if overflow > 0 {
		lines = append(lines, fmt.Sprintf("... +%d more", overflow))
	}
	text := padLines(strings.Join(lines, "\n"))
	d.scheduler.Schedule("validation", func() {
		if d.ingestValidation != nil {
			d.ingestValidation.SetText(text)
			if d.app == nil || d.app.GetFocus() != d.ingestValidation {
				d.ingestValidation.ScrollToEnd()
			}
		}
	})
}

func (d *DashboardV2) appendUnlicensedStream(line string) {
	if d == nil || d.ingestUnlicensed == nil {
		return
	}
	d.unlicensedMu.Lock()
	d.unlicensedLines = append(d.unlicensedLines, line)
	d.unlicensedTotal++
	overflow := 0
	if d.unlicensedMax > 0 && len(d.unlicensedLines) > d.unlicensedMax {
		d.unlicensedLines = d.unlicensedLines[len(d.unlicensedLines)-d.unlicensedMax:]
	}
	lines := append([]string{}, d.unlicensedLines...)
	if d.unlicensedTotal > uint64(len(lines)) {
		overflow = int(d.unlicensedTotal - uint64(len(lines)))
	}
	d.unlicensedMu.Unlock()
	if overflow > 0 {
		lines = append(lines, fmt.Sprintf("... +%d more", overflow))
	}
	text := padLines(strings.Join(lines, "\n"))
	d.scheduler.Schedule("unlicensed", func() {
		if d.ingestUnlicensed != nil {
			d.ingestUnlicensed.SetText(text)
			if d.app == nil || d.app.GetFocus() != d.ingestUnlicensed {
				d.ingestUnlicensed.ScrollToEnd()
			}
		}
	})
}

func (d *DashboardV2) appendCorrectedStream(line string) {
	if d == nil || d.pipelineCorrected == nil {
		return
	}
	d.correctedMu.Lock()
	d.correctedLines = append(d.correctedLines, line)
	d.correctedTotal++
	overflow := 0
	if d.correctedMax > 0 && len(d.correctedLines) > d.correctedMax {
		d.correctedLines = d.correctedLines[len(d.correctedLines)-d.correctedMax:]
	}
	lines := append([]string{}, d.correctedLines...)
	if d.correctedTotal > uint64(len(lines)) {
		overflow = int(d.correctedTotal - uint64(len(lines)))
	}
	d.correctedMu.Unlock()
	if overflow > 0 {
		lines = append(lines, fmt.Sprintf("... +%d more", overflow))
	}
	text := padLines(strings.Join(lines, "\n"))
	d.scheduler.Schedule("corrected", func() {
		if d.pipelineCorrected != nil {
			d.pipelineCorrected.SetText(text)
			if d.app == nil || d.app.GetFocus() != d.pipelineCorrected {
				d.pipelineCorrected.ScrollToEnd()
			}
		}
	})
}

func (d *DashboardV2) appendHarmonicsStream(line string) {
	if d == nil || d.pipelineHarmonics == nil {
		return
	}
	d.harmonicsMu.Lock()
	d.harmonicsLines = append(d.harmonicsLines, line)
	d.harmonicsTotal++
	overflow := 0
	if d.harmonicsMax > 0 && len(d.harmonicsLines) > d.harmonicsMax {
		d.harmonicsLines = d.harmonicsLines[len(d.harmonicsLines)-d.harmonicsMax:]
	}
	lines := append([]string{}, d.harmonicsLines...)
	if d.harmonicsTotal > uint64(len(lines)) {
		overflow = int(d.harmonicsTotal - uint64(len(lines)))
	}
	d.harmonicsMu.Unlock()
	if overflow > 0 {
		lines = append(lines, fmt.Sprintf("... +%d more", overflow))
	}
	text := padLines(strings.Join(lines, "\n"))
	d.scheduler.Schedule("harmonics", func() {
		if d.pipelineHarmonics != nil {
			d.pipelineHarmonics.SetText(text)
			if d.app == nil || d.app.GetFocus() != d.pipelineHarmonics {
				d.pipelineHarmonics.ScrollToEnd()
			}
		}
	})
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
			if d.eventsPage != nil {
				d.eventsPage.refresh()
			}
			if d.debugPage != nil {
				d.debugPage.refresh()
			}
			// pipeline page uses stream panes, no event list refresh.
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
	// buf holds any partial line; it is bounded to avoid unbounded growth when no newline arrives.
	buf          []byte
	mu           sync.Mutex
	droppedBytes uint64
	lastDropLog  time.Time
}

func (w *paneWriter) Write(p []byte) (int, error) {
	if w == nil || w.dash == nil {
		return len(p), nil
	}
	var logDrop bool
	var dropBytes uint64
	var totalDropped uint64
	now := time.Now().UTC()
	w.mu.Lock()
	w.buf = append(w.buf, p...)
	if excess := len(w.buf) - paneWriterMaxBytes; excess > 0 {
		w.buf = w.buf[excess:]
		w.droppedBytes += uint64(excess)
		dropBytes = uint64(excess)
		totalDropped = w.droppedBytes
		if w.lastDropLog.IsZero() || now.Sub(w.lastDropLog) >= 30*time.Second {
			w.lastDropLog = now
			logDrop = true
		}
	}
	data := w.buf
	w.mu.Unlock()
	if logDrop {
		log.Printf("UI: paneWriter dropped %d bytes (total %d) due to missing newline", dropBytes, totalDropped)
	}

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

func newBoxedTextView(title string) *tview.TextView {
	tv := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	tv.SetBorder(true)
	if title != "" {
		tv.SetTitle(accentText(title)).SetTitleAlign(tview.AlignLeft)
	}
	tv.SetBorderColor(uiBorderColor)
	tv.SetTitleColor(uiTitleColor)
	return tv
}

func newSpacer() *tview.Box {
	return tview.NewBox()
}

func buildFooter() *tview.TextView {
	return tview.NewTextView().SetDynamicColors(true).SetText(
		accentText("F1") + "Help  " + accentText("F2") + "Overview  " + accentText("F3") + "Ingest  " + accentText("F4") + "Pipeline  [Q]Quit",
	)
}

func (d *DashboardV2) updateOverviewBoxes(lines []string) {
	if len(lines) == 0 {
		d.seedOverviewPlaceholders()
		return
	}
	// Expected format from buildOverviewLines:
	// 0 header
	// 1 "MEMORY / GC"
	// 2 memory line
	// 3 "INGEST RATES (per min)"
	// 4 rbn line
	// 5 psk line
	// 6 p92 line
	// 7 path-only line
	// 8 primary/secondary line
	// 9 corrections line
	// Section markers are used to slice cache/path/network blocks.
	setOverviewHeader(d.overviewHdr, lines)
	if len(lines) > 2 {
		setBoxText(d.overviewMem, lines[2])
	}
	setOverviewIngest(d.overviewIngest, lines)
	setOverviewPipeline(d.overviewPipeline, lines)
	cacheIdx := -1
	pathIdx := -1
	networkIdx := -1
	for i, line := range lines {
		switch line {
		case "CACHES & DATA FRESHNESS":
			cacheIdx = i
		case "PATH PREDICTIONS":
			pathIdx = i
		case "NETWORK":
			networkIdx = i
		}
	}
	if cacheIdx >= 0 && pathIdx > cacheIdx+1 {
		cacheLines := lines[cacheIdx+1 : pathIdx]
		setBoxText(d.overviewCaches, strings.Join(cacheLines, "\n"))
		if d.overviewRoot != nil {
			height := len(cacheLines) + 2
			if height < 3 {
				height = 3
			}
			d.overviewRoot.ResizeItem(d.overviewCaches, height, 0)
		}
	}
	if pathIdx >= 0 && networkIdx > pathIdx+1 {
		pathLines := lines[pathIdx+1 : networkIdx]
		setBoxText(d.overviewPath, strings.Join(pathLines, "\n"))
		if d.overviewRoot != nil {
			height := len(pathLines) + 2
			if height < 3 {
				height = 3
			}
			d.overviewRoot.ResizeItem(d.overviewPath, height, 0)
		}
	}
	if networkIdx >= 0 && len(lines) > networkIdx+1 {
		networkLines := lines[networkIdx+1:]
		setBoxText(d.overviewNetwork, strings.Join(networkLines, "\n"))
		if d.overviewRoot != nil {
			const (
				networkMaxRows = 10
				baseLines      = 4 // summary + 2 latency lines + blank
				overflowLine   = 1
			)
			maxHeight := baseLines + networkMaxRows + overflowLine + 2
			height := len(networkLines) + 2
			if height > maxHeight {
				height = maxHeight
			}
			if height < 3 {
				height = 3
			}
			d.overviewRoot.ResizeItem(d.overviewNetwork, height, 0)
		}
	}
}

func (d *DashboardV2) updateIngestBoxes(lines []string) {
	if len(lines) == 0 {
		d.seedIngestPlaceholders()
		return
	}
	setOverviewHeader(d.ingestHdr, lines)
	setOverviewIngest(d.ingestIngest, lines)
}

func (d *DashboardV2) updatePipelineBoxes(lines []string) {
	if len(lines) == 0 {
		d.seedPipelinePlaceholders()
		return
	}
	setOverviewHeader(d.pipelineHdr, lines)
	setOverviewPipeline(d.pipelineQuality, lines)
}

func (d *DashboardV2) seedOverviewPlaceholders() {
	setBoxText(d.overviewHdr, "[yellow]Cluster[-]: --  [yellow]Version[-]: --  [yellow]Uptime[-]: --:--")
	setBoxText(d.overviewMem, "[yellow]Heap[-]: --  [yellow]Sys[-]: --  [yellow]GC p99[-]: --  [yellow]Last GC[-]: --  [yellow]Goroutines[-]: --")
	setBoxText(d.overviewIngest, "[yellow]RBN[-]: -- | [yellow]CW[-] -- | [yellow]RTTY[-] -- | [yellow]FT8[-] -- | [yellow]FT4[-] --\n[yellow]PSK[-]: -- | [yellow]CW[-] -- | [yellow]RTTY[-] -- | [yellow]FT8[-] -- | [yellow]FT4[-] -- | [yellow]MSK[-] --\n[yellow]P92[-]: --\n[yellow]Path[-]: -- (U) / -- (S) / -- (N) / -- (G) / -- (H) / -- (B) / -- (M)")
	setBoxText(d.overviewPipeline, "[yellow]Primary Dedupe[-]: -- | [yellow]Secondary[-]: F-- M-- S--\n[yellow]Corrections[-]: -- | [yellow]Unlicensed[-]: -- | [yellow]Harmonics[-]: -- | [yellow]Reputation[-]: --")
	setBoxText(d.overviewCaches, "[yellow]Grid cache[-]:  [[white:white]   [black:white]326,629[-:-]   [-:-]░░░░] 98.5%\n[yellow]Meta cache[-]:  [[white:white]  [black:white] 5,479[-:-]  [-:-]] 99.5%\n[yellow]Known calls[-]: [[white:white] [black:white]50,314[-:-] [-:-]░░░░░░░░] 49.4%\n\n[yellow]CTY[-]: --  [yellow]SCP[-]: --  [yellow]FCC[-]: --  [yellow]Skew[-]: --")
	setBoxText(d.overviewPath, "[yellow]Path pairs[-]: -- (L2) / -- (L1)\n[yellow]160m[-]: -- / --   [yellow]80m[-]: -- / --")
	setBoxText(d.overviewNetwork, "[yellow]Telnet[-]: -- clients   [yellow]Drops[-]: Q-- C-- W--")
}

func (d *DashboardV2) seedIngestPlaceholders() {
	if d == nil || d.ingestHdr == nil || d.ingestIngest == nil {
		return
	}
	setBoxText(d.ingestHdr, "[yellow]Cluster[-]: --  [yellow]Version[-]: --  [yellow]Uptime[-]: --:--")
	setBoxText(d.ingestIngest, "[yellow]RBN[-]: -- | [yellow]CW[-] -- | [yellow]RTTY[-] -- | [yellow]FT8[-] -- | [yellow]FT4[-] --\n[yellow]PSK[-]: -- | [yellow]CW[-] -- | [yellow]RTTY[-] -- | [yellow]FT8[-] -- | [yellow]FT4[-] -- | [yellow]MSK[-] --\n[yellow]P92[-]: --\n[yellow]Path[-]: -- (U) / -- (S) / -- (N) / -- (G) / -- (H) / -- (B) / -- (M)")
	if d.ingestValidation != nil {
		setBoxText(d.ingestValidation, "CTY drop: --")
	}
	if d.ingestUnlicensed != nil {
		setBoxText(d.ingestUnlicensed, "Unlicensed drop: --")
	}
	d.setIngestFocus(d.ingestFocus)
}

func (d *DashboardV2) seedPipelinePlaceholders() {
	if d == nil || d.pipelineHdr == nil || d.pipelineQuality == nil {
		return
	}
	setBoxText(d.pipelineHdr, "[yellow]Cluster[-]: --  [yellow]Version[-]: --  [yellow]Uptime[-]: --:--")
	setBoxText(d.pipelineQuality, "[yellow]Primary Dedupe[-]: -- | [yellow]Secondary[-]: F-- M-- S--\n[yellow]Corrections[-]: -- | [yellow]Unlicensed[-]: -- | [yellow]Harmonics[-]: -- | [yellow]Reputation[-]: --")
	if d.pipelineCorrected != nil {
		setBoxText(d.pipelineCorrected, "Corrected: --")
	}
	if d.pipelineHarmonics != nil {
		setBoxText(d.pipelineHarmonics, "Harmonics: --")
	}
	d.setPipelineFocus(d.pipelineFocus)
}

func setBoxText(tv *tview.TextView, text string) {
	if tv == nil {
		return
	}
	tv.SetText(padLines(text))
}

func setOverviewHeader(tv *tview.TextView, lines []string) {
	if len(lines) > 0 {
		setBoxText(tv, lines[0])
	}
}

func setOverviewIngest(tv *tview.TextView, lines []string) {
	if len(lines) > 7 {
		setBoxText(tv, lines[4]+"\n"+lines[5]+"\n"+lines[6]+"\n"+lines[7])
	}
}

func setOverviewPipeline(tv *tview.TextView, lines []string) {
	if len(lines) > 9 {
		setBoxText(tv, lines[8]+"\n"+lines[9])
	}
}

func addOverviewTopSections(root *tview.Flex, ingest *tview.TextView) {
	if root == nil || ingest == nil {
		return
	}
	root.AddItem(ingest, 6, 0, false)
}

func (d *DashboardV2) cycleIngestFocus(delta int) {
	if d == nil {
		return
	}
	count := 0
	if d.ingestValidation != nil {
		count++
	}
	if d.ingestUnlicensed != nil {
		count++
	}
	if count == 0 {
		return
	}
	next := d.ingestFocus + delta
	if next < 0 {
		next = count - 1
	} else if next >= count {
		next = 0
	}
	d.setIngestFocus(next)
}

func (d *DashboardV2) setIngestFocus(idx int) {
	if d == nil {
		return
	}
	d.ingestFocus = idx
	if d.ingestValidation != nil {
		title := d.validationTitleBase
		if idx == 0 {
			title += " *"
		}
		d.ingestValidation.SetTitle(accentText(title))
	}
	if d.ingestUnlicensed != nil {
		title := d.unlicensedTitleBase
		if idx == 1 {
			title += " *"
		}
		d.ingestUnlicensed.SetTitle(accentText(title))
	}
	switch idx {
	case 0:
		if d.ingestValidation != nil {
			d.app.SetFocus(d.ingestValidation)
		}
	case 1:
		if d.ingestUnlicensed != nil {
			d.app.SetFocus(d.ingestUnlicensed)
		}
	}
}

func (d *DashboardV2) handleIngestScroll(event *tcell.EventKey) bool {
	if d == nil || event == nil {
		return false
	}
	focused := d.app.GetFocus()
	var target *tview.TextView
	switch focused {
	case d.ingestValidation:
		target = d.ingestValidation
	case d.ingestUnlicensed:
		target = d.ingestUnlicensed
	default:
		return false
	}
	return scrollTextView(target, event)
}

func (d *DashboardV2) handleOverviewScroll(event *tcell.EventKey) bool {
	if d == nil || event == nil {
		return false
	}
	if d.app.GetFocus() != d.overviewNetwork {
		return false
	}
	return scrollTextView(d.overviewNetwork, event)
}

func (d *DashboardV2) cycleOverviewFocus(delta int) {
	if d == nil || d.overviewNetwork == nil {
		return
	}
	next := d.overviewFocus + delta
	if next < 0 {
		next = 0
	} else if next > 0 {
		next = 0
	}
	d.setOverviewFocus(next)
}

func (d *DashboardV2) setOverviewFocus(idx int) {
	if d == nil || d.overviewNetwork == nil {
		return
	}
	d.overviewFocus = idx
	title := d.overviewNetworkBase
	if idx == 0 {
		title += " *"
	}
	d.overviewNetwork.SetTitle(accentText(title))
	if d.app != nil {
		d.app.SetFocus(d.overviewNetwork)
	}
}

func (d *DashboardV2) cyclePipelineFocus(delta int) {
	if d == nil {
		return
	}
	count := 0
	if d.pipelineCorrected != nil {
		count++
	}
	if d.pipelineHarmonics != nil {
		count++
	}
	if count == 0 {
		return
	}
	next := d.pipelineFocus + delta
	if next < 0 {
		next = count - 1
	} else if next >= count {
		next = 0
	}
	d.setPipelineFocus(next)
}

func (d *DashboardV2) setPipelineFocus(idx int) {
	if d == nil {
		return
	}
	d.pipelineFocus = idx
	if d.pipelineCorrected != nil {
		title := d.correctedTitleBase
		if idx == 0 {
			title += " *"
		}
		d.pipelineCorrected.SetTitle(accentText(title))
	}
	if d.pipelineHarmonics != nil {
		title := d.harmonicsTitleBase
		if idx == 1 {
			title += " *"
		}
		d.pipelineHarmonics.SetTitle(accentText(title))
	}
	switch idx {
	case 0:
		if d.pipelineCorrected != nil {
			d.app.SetFocus(d.pipelineCorrected)
		}
	case 1:
		if d.pipelineHarmonics != nil {
			d.app.SetFocus(d.pipelineHarmonics)
		}
	}
}

func scrollTextView(target *tview.TextView, event *tcell.EventKey) bool {
	if target == nil || event == nil {
		return false
	}
	row, col := target.GetScrollOffset()
	page := 10
	_, _, _, height := target.GetInnerRect()
	if height > 0 {
		page = height - 1
		if page < 1 {
			page = 1
		}
	}
	switch event.Key() {
	case tcell.KeyUp:
		if row > 0 {
			row--
		}
	case tcell.KeyDown:
		row++
	case tcell.KeyPgUp:
		row -= page
		if row < 0 {
			row = 0
		}
	case tcell.KeyPgDn:
		row += page
	case tcell.KeyHome:
		row = 0
	case tcell.KeyEnd:
		row = 1 << 30
	default:
		return false
	}
	target.ScrollTo(row, col)
	return true
}

func (d *DashboardV2) handlePipelineScroll(event *tcell.EventKey) bool {
	if d == nil || event == nil {
		return false
	}
	focused := d.app.GetFocus()
	var target *tview.TextView
	switch focused {
	case d.pipelineCorrected:
		target = d.pipelineCorrected
	case d.pipelineHarmonics:
		target = d.pipelineHarmonics
	default:
		return false
	}
	return scrollTextView(target, event)
}

func buildHelpOverlay() tview.Primitive {
	help := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	help.SetText(strings.TrimSpace(fmt.Sprintf(`
KEYBOARD HELP

NAVIGATION
  %sF1%s  Help   %sF2%s Overview   %sF3%s Ingest   %sF4%s Pipeline
  Tab Next page   Shift+Tab Previous page   q / Ctrl+C Quit

EVENTS/DEBUG
  ↑/↓ or k/j Scroll   PageUp/Down Fast scroll   Home/End Top/Bottom
  1-6 Filter tabs (Events)   / Search   Esc Clear search / close
`, accentTag, accentReset, accentTag, accentReset, accentTag, accentReset, accentTag, accentReset)))
	help.SetBorder(true).SetTitle("Help")
	help.SetBorderColor(uiBorderColor)
	help.SetTitleColor(uiTitleColor)
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
	header := tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetText(accentText("PIPELINE"))
	root := tview.NewFlex().SetDirection(tview.FlexRow)
	root.AddItem(header, 1, 0, false)

	makeColumn := func(title string) (*tview.Flex, *VirtualList) {
		label := tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetText(accentText(title))
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

func padLines(text string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = " " + line
	}
	return strings.Join(lines, "\n")
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
		"[#ff69b4]", "",
		"[magenta]", "",
		"[cyan]", "",
		"[white]", "",
		"[-]", "",
	)
	return replacer.Replace(s)
}

func accentText(text string) string {
	if text == "" {
		return ""
	}
	return accentTag + text + accentReset
}
