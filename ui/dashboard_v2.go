package ui

import (
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
	accentTag   = "[#7aa2f7]"
	accentReset = "[-]"
)

const (
	placeholderHeader = "[lightgray]Cluster[-]: --  [lightgray]Version[-]: --  [lightgray]Uptime[-]: --:--"
	placeholderMem    = "[lightgray]Heap[-]: --  [lightgray]Sys[-]: --  [lightgray]GC p99 (interval)[-]: --  [lightgray]Last GC[-]: --  [lightgray]Goroutines[-]: --"
	placeholderIngest = "[lightgray]RBN[-]: -- | [lightgray]CW[-] -- | [lightgray]RTTY[-] -- | [lightgray]FT8[-] -- | [lightgray]FT4[-] --\n" +
		"[lightgray]PSK[-]: -- | [lightgray]CW[-] -- | [lightgray]RTTY[-] -- | [lightgray]FT8[-] -- | [lightgray]FT4[-] -- | [lightgray]MSK[-] --\n" +
		"[lightgray]P92[-]: --\n" +
		"[lightgray]Path[-]: -- (U) / -- (S) / -- (N) / -- (G) / -- (H) / -- (B) / -- (M)"
	placeholderPipeline = "[lightgray]Primary Dedupe[-]: -- | [lightgray]Secondary[-]: F-- M-- S--\n" +
		"[lightgray]Corrections[-]: -- | [lightgray]Unlicensed[-]: -- | [lightgray]Harmonics[-]: -- | [lightgray]Reputation[-]: --"
	placeholderCaches = "[lightgray]Grid cache[-]:  [[white:white]   [black:white]326,629[-:-]   [-:-]░░░░] 98.5%\n" +
		"[lightgray]Meta cache[-]:  [[white:white]  [black:white] 5,479[-:-]  [-:-]] 99.5%\n" +
		"[lightgray]Known calls[-]: [[white:white] [black:white]50,314[-:-] [-:-]░░░░░░░░] 49.4%\n\n" +
		"[lightgray]CTY[-]: --  [lightgray]SCP[-]: --  [lightgray]FCC[-]: --  [lightgray]Skew[-]: --"
	placeholderPath       = "[lightgray]Path pairs[-]: -- (L2) / -- (L1)\n[lightgray]160m[-]: -- / --   [lightgray]80m[-]: -- / --"
	placeholderNetwork    = "[lightgray]Telnet[-]: -- clients   [lightgray]Drops[-]: Q-- C-- W--"
	placeholderValidation = "CTY drop: --"
	placeholderUnlicensed = "Unlicensed drop: --"
	placeholderCorrected  = "Corrected: --"
	placeholderHarmonics  = "Harmonics: --"
	streamPanelMaxLines   = 200
)

var (
	uiBorderColor      = tcell.ColorGray
	uiTitleColor       = tcell.NewRGBColor(170, 180, 200)
	uiFocusBorderColor = tcell.NewRGBColor(122, 162, 247)
	uiFocusTitleColor  = tcell.NewRGBColor(122, 162, 247)
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

	validationPanel *streamPanel
	unlicensedPanel *streamPanel
	correctedPanel  *streamPanel
	harmonicsPanel  *streamPanel

	overviewGroup focusGroup
	ingestGroup   focusGroup
	pipelineGroup focusGroup

	pageOrder []string
	pageIndex int
	helpShown bool
	metrics   *Metrics

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

	d.overviewHdr = newBoxedTextView("Overview")
	d.overviewMem = newBoxedTextView("Memory / GC")
	d.overviewIngest = newBoxedTextView("Ingest Rates (per min)")
	d.overviewPipeline = newBoxedTextView("Pipeline Quality")
	d.overviewCaches = newBoxedTextView("Caches & Data Freshness")
	d.overviewPath = newBoxedTextView("Path Predictions")
	d.overviewNetwork = newBoxedTextView("Network")
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
	d.ingestValidation = newBoxedTextView("Validation")
	d.ingestUnlicensed = newBoxedTextView("Unlicensed")
	d.seedIngestPlaceholders()
	d.validationPanel = newStreamPanel(d.ingestValidation, "Validation", streamPanelMaxLines)
	d.unlicensedPanel = newStreamPanel(d.ingestUnlicensed, "Unlicensed", streamPanelMaxLines)
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
	d.pipelineCorrected = newBoxedTextView("Corrected")
	d.pipelineHarmonics = newBoxedTextView("Harmonics")
	d.seedPipelinePlaceholders()
	d.correctedPanel = newStreamPanel(d.pipelineCorrected, "Corrected", streamPanelMaxLines)
	d.harmonicsPanel = newStreamPanel(d.pipelineHarmonics, "Harmonics", streamPanelMaxLines)
	d.pipelineRoot = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(d.pipelineHdr, 3, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.pipelineQuality, 4, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.pipelineCorrected, 28, 0, false).
		AddItem(newSpacer(), 1, 0, false).
		AddItem(d.pipelineHarmonics, 28, 0, false)

	d.overviewGroup = newFocusGroup(newFocusBox(d.overviewNetwork, "Network", true))
	d.ingestGroup = newFocusGroup(d.validationPanel, d.unlicensedPanel)
	d.pipelineGroup = newFocusGroup(d.correctedPanel, d.harmonicsPanel)

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
			if d.ingestGroup.handleScroll(d.app, event) {
				return nil
			}
		} else if pageName == "overview" {
			if d.overviewGroup.handleScroll(d.app, event) {
				return nil
			}
		} else if pageName == "pipeline" {
			if d.pipelineGroup.handleScroll(d.app, event) {
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
				d.ingestGroup.cycle(d.app, 1)
			} else if pageName == "overview" {
				d.overviewGroup.cycle(d.app, 1)
			} else if pageName == "pipeline" {
				d.pipelineGroup.cycle(d.app, 1)
			} else {
				d.nextPage()
			}
			return nil
		case tcell.KeyBacktab:
			if pageName, _ := d.pages.GetFrontPage(); pageName == "ingest" {
				d.ingestGroup.cycle(d.app, -1)
			} else if pageName == "overview" {
				d.overviewGroup.cycle(d.app, -1)
			} else if pageName == "pipeline" {
				d.pipelineGroup.cycle(d.app, -1)
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
	case "overview":
		d.overviewGroup.set(d.app, 0)
	case "ingest":
		d.ingestGroup.set(d.app, 0)
	case "pipeline":
		d.pipelineGroup.set(d.app, 0)
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
		d.appendStream(d.validationPanel, "validation", line)
	}
}

func (d *DashboardV2) AppendCall(line string) {
	d.appendStream(d.correctedPanel, "corrected", line)
}

func (d *DashboardV2) AppendUnlicensed(line string) {
	d.appendStream(d.unlicensedPanel, "unlicensed", line)
}

func (d *DashboardV2) AppendHarmonic(line string) {
	d.appendStream(d.harmonicsPanel, "harmonics", line)
}

func (d *DashboardV2) appendStream(panel *streamPanel, scheduleID string, line string) {
	if d == nil || panel == nil || d.scheduler == nil {
		return
	}
	panel.Append(line)
	// Coalesce updates per frame; the scheduler keeps only the latest per ID.
	d.scheduler.Schedule(scheduleID, func() {
		panel.Render(d.app)
	})
}

func (d *DashboardV2) AppendReputation(line string) {
	if d == nil {
		return
	}
	// No-op while events/debug pages are disabled.
}

func (d *DashboardV2) AppendSystem(line string) {
	if d == nil {
		return
	}
	// No-op while events/debug pages are disabled.
}

func (d *DashboardV2) SystemWriter() io.Writer {
	if d == nil {
		return nil
	}
	// Event/debug pages are disabled; discard console logs to avoid UI disruption.
	return io.Discard
}

func newBoxedTextView(title string) *tview.TextView {
	tv := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	tv.SetBorder(true)
	applyFocusStyle(tv, title, false)
	return tv
}

func applyFocusStyle(tv *tview.TextView, title string, focused bool) {
	if tv == nil {
		return
	}
	if title != "" {
		tv.SetTitle(title).SetTitleAlign(tview.AlignLeft)
	}
	if focused {
		tv.SetBorderColor(uiFocusBorderColor)
		tv.SetTitleColor(uiFocusTitleColor)
		return
	}
	tv.SetBorderColor(uiBorderColor)
	tv.SetTitleColor(uiTitleColor)
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
	setBoxText(d.overviewHdr, placeholderHeader)
	setBoxText(d.overviewMem, placeholderMem)
	setBoxText(d.overviewIngest, placeholderIngest)
	setBoxText(d.overviewPipeline, placeholderPipeline)
	setBoxText(d.overviewCaches, placeholderCaches)
	setBoxText(d.overviewPath, placeholderPath)
	setBoxText(d.overviewNetwork, placeholderNetwork)
}

func (d *DashboardV2) seedIngestPlaceholders() {
	if d == nil || d.ingestHdr == nil || d.ingestIngest == nil {
		return
	}
	setBoxText(d.ingestHdr, placeholderHeader)
	setBoxText(d.ingestIngest, placeholderIngest)
	if d.ingestValidation != nil {
		setBoxText(d.ingestValidation, placeholderValidation)
	}
	if d.ingestUnlicensed != nil {
		setBoxText(d.ingestUnlicensed, placeholderUnlicensed)
	}
}

func (d *DashboardV2) seedPipelinePlaceholders() {
	if d == nil || d.pipelineHdr == nil || d.pipelineQuality == nil {
		return
	}
	setBoxText(d.pipelineHdr, placeholderHeader)
	setBoxText(d.pipelineQuality, placeholderPipeline)
	if d.pipelineCorrected != nil {
		setBoxText(d.pipelineCorrected, placeholderCorrected)
	}
	if d.pipelineHarmonics != nil {
		setBoxText(d.pipelineHarmonics, placeholderHarmonics)
	}
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

func buildHelpOverlay() tview.Primitive {
	help := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	help.SetText(strings.TrimSpace(fmt.Sprintf(`
KEYBOARD HELP

NAVIGATION
  %sF1%s  Help   %sF2%s Overview   %sF3%s Ingest   %sF4%s Pipeline
  Tab Next pane   Shift+Tab Previous pane   q / Ctrl+C Quit

SCROLLING
  ↑/↓ or k/j Scroll   PageUp/Down Fast scroll   Home/End Top/Bottom
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

func padLines(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(text) + 8)
	atLineStart := true
	for _, r := range text {
		if atLineStart && r != '\n' {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		if r == '\n' {
			atLineStart = true
		} else {
			atLineStart = false
		}
	}
	return b.String()
}

func accentText(text string) string {
	if text == "" {
		return ""
	}
	return accentTag + text + accentReset
}
