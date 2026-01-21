// Program gocluster wires together all ingest clients (RBN, PSKReporter),
// protections (deduplication, call correction, harmonics), persistence layers
// (ring buffer, grid store), and the telnet server UI.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	httppprof "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	pprof "runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"dxcluster/archive"
	"dxcluster/bandmap"
	"dxcluster/buffer"
	"dxcluster/commands"
	"dxcluster/config"
	"dxcluster/cty"
	"dxcluster/dedup"
	"dxcluster/download"
	"dxcluster/filter"
	"dxcluster/gridstore"
	"dxcluster/pathreliability"
	"dxcluster/peer"
	"dxcluster/pskreporter"
	"dxcluster/rbn"
	"dxcluster/reputation"
	"dxcluster/skew"
	"dxcluster/spot"
	"dxcluster/stats"
	"dxcluster/telnet"
	"dxcluster/uls"

	"github.com/dustin/go-humanize"
	"golang.org/x/term"
)

const (
	dedupeEntryBytes          = 32
	callMetaEntryBytes        = 96
	knownCallEntryBytes       = 24
	sourceModeDelimiter       = "|"
	defaultConfigPath         = "data/config"
	pathReliabilityConfigFile = "path_reliability.yaml"
	envConfigPath             = "DXC_CONFIG_PATH"

	// envGridDBCheckOnMiss overrides the config-driven grid_db_check_on_miss at runtime.
	// When true, grid updates may synchronously consult SQLite on cache miss to avoid
	// redundant writes. When false, the hot path never blocks on that read and may
	// perform extra batched writes instead.
	envGridDBCheckOnMiss = "DXC_GRID_DB_CHECK_ON_MISS"
)

var licCache = newLicenseCache(5 * time.Minute)

// licenseCache caches FCC license checks to avoid repeated lookups on hot paths.
type licenseCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]licenseEntry
}

type licenseEntry struct {
	licensed bool
	at       time.Time
}

// Purpose: Construct a licenseCache with a bounded TTL.
// Key aspects: Normalizes non-positive TTL to a safe default.
// Upstream: package init for licCache and main wiring.
// Downstream: map allocation for cache entries.
func newLicenseCache(ttl time.Duration) *licenseCache {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &licenseCache{
		ttl:     ttl,
		entries: make(map[string]licenseEntry),
	}
}

// Purpose: Lookup a cached FCC license decision for a callsign.
// Key aspects: Enforces TTL expiration and returns (value, ok).
// Upstream: applyLicenseGate.
// Downstream: time comparisons and map access under lock.
func (lc *licenseCache) get(call string, now time.Time) (bool, bool) {
	if lc == nil || call == "" {
		return false, false
	}
	lc.mu.Lock()
	defer lc.mu.Unlock()
	entry, ok := lc.entries[call]
	if !ok {
		return false, false
	}
	if lc.ttl > 0 && now.Sub(entry.at) > lc.ttl {
		delete(lc.entries, call)
		return false, false
	}
	return entry.licensed, true
}

// Purpose: Store a callsign license decision in the cache.
// Key aspects: Overwrites existing entries with updated timestamp.
// Upstream: applyLicenseGate.
// Downstream: map assignment under lock.
func (lc *licenseCache) set(call string, licensed bool, now time.Time) {
	if lc == nil || call == "" {
		return
	}
	lc.mu.Lock()
	lc.entries[call] = licenseEntry{licensed: licensed, at: now}
	lc.mu.Unlock()
}

// sweepExpired removes entries older than ttl. Returns the number removed.
func (lc *licenseCache) sweepExpired(now time.Time) int {
	if lc == nil || lc.ttl <= 0 {
		return 0
	}
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if len(lc.entries) == 0 {
		return 0
	}
	var removed int
	for k, v := range lc.entries {
		if now.Sub(v.at) > lc.ttl {
			delete(lc.entries, k)
			removed++
		}
	}
	return removed
}

// startLicenseCacheSweeper launches a periodic TTL sweep that stops when ctx is done.
func startLicenseCacheSweeper(ctx context.Context, lc *licenseCache) {
	if lc == nil || lc.ttl <= 0 {
		return
	}
	interval := lc.ttl / 2
	if interval < time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				lc.sweepExpired(time.Now())
			}
		}
	}()
}

// Version will be set at build time
var Version = "dev"

type gridMetrics struct {
	learnedTotal atomic.Uint64
	cacheLookups atomic.Uint64
	cacheHits    atomic.Uint64
}

const (
	gridSyncLookupWorkers   = 4
	gridSyncLookupQueueDepth = 512
	gridSyncLookupTimeout   = 8 * time.Millisecond
)

type gridLookupRequest struct {
	baseCall string
	rawCall  string
	resp     chan gridLookupResult
}

type gridLookupResult struct {
	grid string
	ok   bool
}

// Purpose: Report whether stdout is a TTY for UI gating.
// Key aspects: Uses term.IsTerminal on stdout fd.
// Upstream: main UI selection.
// Downstream: term.IsTerminal.
func isStdoutTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// Purpose: Load configuration from env/default directories.
// Key aspects: Tries env override first, then the default config dir.
// Upstream: main startup.
// Downstream: config.Load and os.IsNotExist.
func loadClusterConfig() (*config.Config, string, error) {
	candidates := make([]string, 0, 2)
	if envPath := strings.TrimSpace(os.Getenv(envConfigPath)); envPath != "" {
		candidates = append(candidates, envPath)
	}
	candidates = append(candidates, defaultConfigPath)

	var lastErr error
	for _, path := range candidates {
		if path == "" {
			continue
		}
		cfg, err := config.Load(path)
		if err != nil {
			if os.IsNotExist(err) {
				lastErr = err
				continue
			}
			return nil, path, err
		}
		return cfg, cfg.LoadedFrom, nil
	}
	return nil, "", fmt.Errorf("unable to load config; tried %s (last error: %v)", strings.Join(candidates, ", "), lastErr)
}

// Purpose: Resolve grid_db_check_on_miss behavior and its source.
// Key aspects: Env DXC_GRID_DB_CHECK_ON_MISS overrides config defaults.
// Upstream: main grid cache setup.
// Downstream: strconv.ParseBool and logging on invalid input.
func gridDBCheckOnMissEnabled(cfg *config.Config) (bool, string) {
	enabled := true
	source := "default"
	if cfg != nil && cfg.GridDBCheckOnMiss != nil {
		enabled = *cfg.GridDBCheckOnMiss
		source = strings.TrimSpace(cfg.LoadedFrom)
		if source == "" {
			source = "config"
		}
	}

	raw := strings.TrimSpace(os.Getenv(envGridDBCheckOnMiss))
	if raw == "" {
		return enabled, source
	}

	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		log.Printf("Gridstore: ignoring invalid %s=%q; using %s value=%v", envGridDBCheckOnMiss, raw, source, enabled)
		return enabled, source
	}

	return parsed, envGridDBCheckOnMiss
}

// Purpose: Program entrypoint; wires configuration, ingest, and output pipeline.
// Key aspects: Initializes caches/clients/UI and manages graceful shutdown.
// Upstream: OS process start.
// Downstream: Startup helpers, goroutines, and network services.
func main() {
	// Load configuration
	cfg, configSource, err := loadClusterConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}
	log.Printf("Loaded configuration from %s", configSource)
	if err := spot.SetDXClusterLineLength(cfg.Telnet.OutputLineLength); err != nil {
		log.Fatalf("Invalid telnet output line length: %v", err)
	}

	// Load path reliability config from dedicated file in the config directory.
	pathCfgPath := filepath.Join(configSource, pathReliabilityConfigFile)
	pathCfg, pathCfgErr := pathreliability.LoadFile(pathCfgPath)
	if pathCfgErr != nil {
		if os.IsNotExist(pathCfgErr) {
			pathCfg = pathreliability.DefaultConfig()
			pathCfg.Enabled = false
			log.Printf("Path reliability config not found at %s; feature disabled", pathCfgPath)
		} else {
			log.Printf("Warning: failed to load path reliability config (%s): %v", pathCfgPath, pathCfgErr)
			pathCfg = pathreliability.DefaultConfig()
		}
	}
	pathPredictor := pathreliability.NewPredictor(pathCfg, spot.SupportedBandNames())

	uiMode := strings.ToLower(strings.TrimSpace(cfg.UI.Mode))
	renderAllowed := isStdoutTTY()

	var ui uiSurface
	switch uiMode {
	case "headless":
		log.Printf("UI disabled (mode=headless)")
	case "tview":
		if !renderAllowed {
			log.Printf("UI disabled (tview requires an interactive console)")
		} else {
			ui = newDashboard(cfg.UI, true)
		}
	case "ansi":
		if !renderAllowed {
			log.Printf("UI disabled (ansi renderer requires an interactive console)")
		} else {
			ui = newANSIConsole(cfg.UI, renderAllowed)
		}
	default:
		log.Printf("UI mode %q not recognized; defaulting to headless", uiMode)
	}

	if ui != nil {
		ui.WaitReady()
		defer ui.Stop()
		// Dashboard handles its own timestamp formatting; disable the default log prefixes.
		log.SetFlags(0)
		log.SetOutput(ui.SystemWriter())
		ui.SetStats([]string{"Initializing..."})
	} else {
		log.SetOutput(os.Stdout)
	}

	log.Printf("DX Cluster Server v%s starting...", Version)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startLicenseCacheSweeper(ctx, licCache)
	if pathCfg.Enabled && pathPredictor != nil {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					pathPredictor.PurgeStale(time.Now().UTC())
				}
			}
		}()
	}

	callCacheTTL := time.Duration(cfg.CallCache.TTLSeconds) * time.Second
	spot.ConfigureNormalizeCallCache(cfg.CallCache.Size, callCacheTTL)
	rbn.ConfigureCallCache(cfg.CallCache.Size, callCacheTTL)
	pskreporter.ConfigureCallCache(cfg.CallCache.Size, callCacheTTL)
	filter.SetDefaultModeSelection(cfg.Filter.DefaultModes)
	filter.SetDefaultSourceSelection(cfg.Filter.DefaultSources)
	if err := filter.EnsureUserDataDir(); err != nil {
		log.Printf("Warning: unable to initialize filter directory: %v", err)
	}

	metaCache := newCallMetaCache(cfg.GridCacheSize, time.Duration(cfg.GridCacheTTLSec)*time.Second)

	// Print the configuration (stdout only when not using the dashboard)
	if ui == nil {
		cfg.Print()
	} else {
		log.Printf("Configuration loaded for %s (%s)", cfg.Server.Name, cfg.Server.NodeID)
	}

	// Optional call-correction decision logger (asynchronous SQLite writer).
	var corrLogger spot.CorrectionTraceLogger
	if cfg.CallCorrection.DebugLog {
		logger, err := spot.NewDecisionLogger(cfg.CallCorrection.DebugLogFile, 0)
		if err != nil {
			log.Printf("Warning: unable to start call-correction decision logger: %v", err)
		} else {
			corrLogger = logger
			path := spot.DecisionLogPath(cfg.CallCorrection.DebugLogFile, time.Now().UTC())
			log.Printf("Call correction decision logging to %s (SQLite, non-blocking)", path)
		}
	}

	// Toggle FCC ULS lookups independently of the downloader so disabled configs
	// can keep the DB on disk without performing license checks.
	uls.SetLicenseChecksEnabled(cfg.FCCULS.Enabled)

	// Start the FCC ULS downloader in the background (does not block spot processing)
	uls.StartBackground(ctx, cfg.FCCULS)

	// Load CTY database for callsign validation, track refresh age, and schedule retries.
	var ctyDB atomic.Pointer[cty.CTYDatabase]
	ctyState := newCTYRefreshState()
	ctyPath := strings.TrimSpace(cfg.CTY.File)
	ctyURL := strings.TrimSpace(cfg.CTY.URL)
	if cfg.CTY.Enabled && ctyPath != "" {
		if _, err := os.Stat(ctyPath); err != nil && errors.Is(err, os.ErrNotExist) && ctyURL != "" {
			if fresh, updated, refreshErr := refreshCTYDatabase(cfg.CTY); refreshErr != nil {
				log.Printf("Warning: CTY download failed: %v", refreshErr)
				ctyState.recordFailure(time.Now().UTC(), refreshErr)
			} else if updated && fresh != nil {
				ctyDB.Store(fresh)
				ctyState.recordSuccess(time.Now().UTC())
				log.Printf("Downloaded CTY database from %s", ctyURL)
			} else {
				ctyState.recordSuccess(time.Now().UTC())
				log.Printf("CTY database already up to date (%s)", ctyPath)
			}
		}
	}
	if cfg.CTY.Enabled && ctyDB.Load() == nil && ctyPath != "" {
		if loaded, loadErr := cty.LoadCTYDatabase(ctyPath); loadErr != nil {
			log.Printf("Warning: failed to load CTY database: %v", loadErr)
			ctyState.recordFailure(time.Now().UTC(), loadErr)
		} else {
			ctyDB.Store(loaded)
			ctyState.recordSuccess(time.Now().UTC())
			log.Printf("Loaded CTY database from %s", ctyPath)
		}
	}
	ctyLookup := func() *cty.CTYDatabase {
		return ctyDB.Load()
	}
	if cfg.CTY.Enabled && ctyURL != "" && ctyPath != "" {
		startCTYScheduler(ctx, cfg.CTY, &ctyDB, metaCache, ctyState)
	} else if cfg.CTY.Enabled {
		log.Printf("Warning: CTY download enabled but url or file missing")
	}
	spot.ConfigureMorseWeights(cfg.CallCorrection.MorseWeights.Insert, cfg.CallCorrection.MorseWeights.Delete, cfg.CallCorrection.MorseWeights.Sub, cfg.CallCorrection.MorseWeights.Scale)
	spot.ConfigureBaudotWeights(cfg.CallCorrection.BaudotWeights.Insert, cfg.CallCorrection.BaudotWeights.Delete, cfg.CallCorrection.BaudotWeights.Sub, cfg.CallCorrection.BaudotWeights.Scale)
	if priors := strings.TrimSpace(cfg.CallCorrection.QualityPriorsFile); priors != "" {
		if n, err := spot.LoadCallQualityPriors(priors, cfg.CallCorrection.QualityBinHz); err != nil {
			log.Printf("Warning: failed to load quality priors from %s: %v", priors, err)
		} else {
			log.Printf("Loaded %d call quality priors from %s", n, priors)
		}
	}
	var spotterReliability spot.SpotterReliability
	if relPath := strings.TrimSpace(cfg.CallCorrection.SpotterReliabilityFile); relPath != "" {
		if rel, n, err := spot.LoadSpotterReliability(relPath); err != nil {
			log.Printf("Warning: failed to load spotter reliability from %s: %v", relPath, err)
		} else {
			spotterReliability = rel
			log.Printf("Loaded %d spotter reliability weights from %s", n, relPath)
		}
	}
	adaptiveMinReports := spot.NewAdaptiveMinReports(cfg.CallCorrection)
	refresher := newAdaptiveRefresher(adaptiveMinReports, cfg.CallCorrection.AdaptiveRefreshByBand, noopRefresh)
	if refresher != nil {
		refresher.Start()
		defer refresher.Stop()
	}
	if cfg.FCCULS.Enabled && strings.TrimSpace(cfg.FCCULS.DBPath) != "" {
		uls.SetLicenseDBPath(cfg.FCCULS.DBPath)
	}

	// Create stats tracker
	statsTracker := stats.NewTracker()
	dropReporter := makeDroppedReporter(ui)
	unlicensedReporter := makeUnlicensedReporter(ui, statsTracker)

	var repGate *reputation.Gate
	var repDropReporter func(reputation.DropEvent)
	if cfg.Reputation.Enabled {
		gate, err := reputation.NewGate(cfg.Reputation, ctyLookup)
		if err != nil {
			log.Printf("Warning: reputation gate disabled: %v", err)
		} else {
			repGate = gate
			repGate.Start(ctx)
			repDropReporter = makeReputationDropReporter(dropReporter, statsTracker, cfg.Reputation)
		}
	}

	capacity := cfg.Buffer.Capacity
	if capacity <= 0 {
		capacity = 300000
	}
	// Create spot buffer (ring buffer for storing recent spots)
	spotBuffer := buffer.NewRingBuffer(capacity)
	log.Printf("Ring buffer created (capacity: %d)", capacity)

	var correctionIndex *spot.CorrectionIndex
	if cfg.CallCorrection.Enabled {
		correctionIndex = spot.NewCorrectionIndex()
		correctionIndex.StartCleanup(time.Minute, callCorrectionWindow(cfg.CallCorrection))
	}
	var callCooldown *spot.CallCooldown
	if cfg.CallCorrection.CooldownEnabled {
		callCooldown = spot.NewCallCooldown(spot.CallCooldownConfig{
			Enabled:          cfg.CallCorrection.CooldownEnabled,
			MinReporters:     cfg.CallCorrection.CooldownMinReporters,
			Duration:         time.Duration(cfg.CallCorrection.CooldownDurationSeconds) * time.Second,
			TTL:              time.Duration(cfg.CallCorrection.CooldownTTLSeconds) * time.Second,
			BinHz:            cfg.CallCorrection.CooldownBinHz,
			MaxReporters:     cfg.CallCorrection.CooldownMaxReporters,
			BypassAdvantage:  cfg.CallCorrection.CooldownBypassAdvantage,
			BypassConfidence: cfg.CallCorrection.CooldownBypassConfidence,
		})
		callCooldown.StartCleanup(time.Duration(cfg.CallCorrection.CooldownTTLSeconds) * time.Second)
	}

	var knownCalls atomic.Pointer[spot.KnownCallsigns]
	knownCallsPath := strings.TrimSpace(cfg.KnownCalls.File)
	knownCallsURL := strings.TrimSpace(cfg.KnownCalls.URL)
	if knownCallsPath != "" {
		if _, err := os.Stat(knownCallsPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if knownCallsURL != "" {
					if fresh, updated, refreshErr := refreshKnownCallsigns(cfg.KnownCalls); refreshErr != nil {
						log.Printf("Warning: known calls download failed: %v", refreshErr)
					} else if updated && fresh != nil {
						knownCalls.Store(fresh)
						log.Printf("Downloaded %d known callsigns from %s", fresh.Count(), knownCallsURL)
					} else {
						log.Printf("Known calls file already up to date (%s)", knownCallsPath)
					}
				} else {
					log.Printf("Warning: known calls file %s missing and no download URL configured", knownCallsPath)
				}
			} else {
				log.Printf("Warning: unable to access known calls file %s: %v", knownCallsPath, err)
			}
		}
		if knownCalls.Load() == nil {
			if loaded, loadErr := spot.LoadKnownCallsigns(knownCallsPath); loadErr != nil {
				log.Printf("Warning: failed to load known callsigns: %v", loadErr)
			} else {
				knownCalls.Store(loaded)
				log.Printf("Loaded %d known callsigns from %s", loaded.Count(), knownCallsPath)
			}
		}
	}
	gridOpts := gridstore.Options{
		CacheSizeBytes:        int64(cfg.GridBlockCacheMB) << 20,
		BloomFilterBitsPerKey: cfg.GridBloomFilterBits,
		MemTableSizeBytes:     uint64(cfg.GridMemTableSizeMB) << 20,
		L0CompactionThreshold: cfg.GridL0Compaction,
		L0StopWritesThreshold: cfg.GridL0StopWrites,
		WriteQueueDepth:       cfg.GridWriteQueueDepth,
	}
	gridStore, err := gridstore.Open(cfg.GridDBPath, gridOpts)
	if err != nil {
		log.Fatalf("Failed to open grid database: %v", err)
	}
	defer gridStore.Close()
	if known := knownCalls.Load(); known != nil {
		if err := seedKnownCalls(gridStore, known); err != nil {
			log.Printf("Warning: failed to seed known calls into grid database: %v", err)
		}
	}

	gridDBCheckOnMiss, gridDBCheckSource := gridDBCheckOnMissEnabled(cfg)
	log.Printf("Gridstore: db_check_on_miss=%v (source=%s)", gridDBCheckOnMiss, gridDBCheckSource)

	gridTTL := time.Duration(cfg.GridTTLDays) * 24 * time.Hour
	gridUpdater, ctyUpdater, gridUpdateState, stopGridWriter, gridLookup, gridLookupSync := startGridWriter(gridStore, time.Duration(cfg.GridFlushSec)*time.Second, metaCache, gridTTL, gridDBCheckOnMiss)
	defer func() {
		if stopGridWriter != nil {
			stopGridWriter()
		}
	}()

	if cfg.KnownCalls.Enabled && knownCallsURL != "" && knownCallsPath != "" {
		if knownCalls.Load() != nil {
			startKnownCallScheduler(ctx, cfg.KnownCalls, &knownCalls, gridStore, metaCache)
		} else {
			log.Printf("Warning: known calls scheduler disabled (no initial data); ensure %s is reachable", cfg.KnownCalls.URL)
		}
	} else if cfg.KnownCalls.Enabled {
		log.Printf("Warning: known calls download enabled but url or file missing")
	}

	var skewStore *skew.Store
	if cfg.Skew.Enabled {
		skewStore = skew.NewStore()
		if table, loadErr := skew.LoadFile(cfg.Skew.File); loadErr == nil {
			skewStore.Set(table)
			log.Printf("Loaded %d RBN skew corrections from %s", table.Count(), cfg.Skew.File)
		} else {
			log.Printf("Warning: failed to load RBN skew table (%s): %v", cfg.Skew.File, loadErr)
		}
		if skewStore.Count() == 0 {
			if count, err := refreshSkewTable(cfg.Skew, skewStore); err != nil {
				log.Printf("Warning: initial RBN skew download failed: %v", err)
			} else {
				log.Printf("Downloaded %d RBN skew corrections from %s", count, cfg.Skew.URL)
			}
		}
		if skewStore.Count() > 0 {
			startSkewScheduler(ctx, cfg.Skew, skewStore)
		} else {
			log.Printf("Warning: RBN skew scheduler disabled (no initial data); ensure %s is reachable", cfg.Skew.URL)
			skewStore = nil
		}
	}

	freqAverager := spot.NewFrequencyAverager()
	freqAverager.StartCleanup(time.Minute, frequencyAverageWindow(cfg.SpotPolicy))
	var harmonicDetector *spot.HarmonicDetector
	if cfg.Harmonics.Enabled {
		harmonicDetector = spot.NewHarmonicDetector(spot.HarmonicSettings{
			Enabled:              true,
			RecencyWindow:        time.Duration(cfg.Harmonics.RecencySeconds) * time.Second,
			MaxHarmonicMultiple:  cfg.Harmonics.MaxHarmonicMultiple,
			FrequencyToleranceHz: cfg.Harmonics.FrequencyToleranceHz,
			MinReportDelta:       cfg.Harmonics.MinReportDelta,
			MinReportDeltaStep:   cfg.Harmonics.MinReportDeltaStep,
		})
		harmonicDetector.StartCleanup(time.Minute)
	}

	// Create the deduplicator (always active; a zero-second window behaves like "disabled").
	// THIS IS THE UNIFIED DEDUP ENGINE - ALL SOURCES FEED INTO IT
	dedupWindow := time.Duration(cfg.Dedup.ClusterWindowSeconds) * time.Second
	deduplicator := dedup.NewDeduplicator(dedupWindow, cfg.Dedup.PreferStrongerSNR, cfg.Dedup.OutputBufferSize)
	deduplicator.Start()
	if dedupWindow > 0 {
		log.Printf("Deduplication active with %v window", dedupWindow)
	} else {
		log.Println("Deduplication disabled (cluster window=0); spots pass through unfiltered")
	}

	dedupInput := deduplicator.GetInputChannel()
	ingestValidator := newIngestValidator(ctyLookup, metaCache, ctyUpdater, gridUpdater, dedupInput, unlicensedReporter, dropReporter, cfg.CTY.Enabled)
	ingestValidator.Start()
	ingestInput := ingestValidator.Input()

	secondaryFastWindow := time.Duration(cfg.Dedup.SecondaryFastWindowSeconds) * time.Second
	secondarySlowWindow := time.Duration(cfg.Dedup.SecondarySlowWindowSeconds) * time.Second
	var secondaryFast *dedup.SecondaryDeduper
	var secondarySlow *dedup.SecondaryDeduper
	if secondaryFastWindow > 0 {
		secondaryFast = dedup.NewSecondaryDeduper(secondaryFastWindow, cfg.Dedup.SecondaryFastPreferStrong)
		secondaryFast.Start()
		log.Printf("Secondary dedupe (fast) active with %v window", secondaryFastWindow)
	} else {
		log.Println("Secondary dedupe (fast) disabled")
	}
	if secondarySlowWindow > 0 {
		secondarySlow = dedup.NewSecondaryDeduper(secondarySlowWindow, cfg.Dedup.SecondarySlowPreferStrong)
		secondarySlow.Start()
		log.Printf("Secondary dedupe (slow) active with %v window", secondarySlowWindow)
	} else {
		log.Println("Secondary dedupe (slow) disabled")
	}
	if secondaryFastWindow <= 0 && secondarySlowWindow <= 0 {
		log.Println("Warning: secondary dedupe disabled (fast+slow=0); spots broadcast without secondary suppression")
	}

	modeSeeds := make([]spot.ModeSeed, 0, len(cfg.ModeInference.DigitalSeeds))
	for _, seed := range cfg.ModeInference.DigitalSeeds {
		modeSeeds = append(modeSeeds, spot.ModeSeed{
			FrequencyKHz: seed.FrequencyKHz,
			Mode:         seed.Mode,
		})
	}
	modeAssigner := spot.NewModeAssigner(spot.ModeInferenceSettings{
		DXFreqCacheTTL:        time.Duration(cfg.ModeInference.DXFreqCacheTTLSeconds) * time.Second,
		DXFreqCacheSize:       cfg.ModeInference.DXFreqCacheSize,
		DigitalWindow:         time.Duration(cfg.ModeInference.DigitalWindowSeconds) * time.Second,
		DigitalMinCorroborate: cfg.ModeInference.DigitalMinCorroborators,
		DigitalSeedTTL:        time.Duration(cfg.ModeInference.DigitalSeedTTLSeconds) * time.Second,
		DigitalCacheSize:      cfg.ModeInference.DigitalCacheSize,
		DigitalSeeds:          modeSeeds,
	})
	log.Printf("Mode inference: dx_cache=%d ttl=%ds digital_window=%ds min_corrob=%d seeds=%d seed_ttl=%ds",
		cfg.ModeInference.DXFreqCacheSize,
		cfg.ModeInference.DXFreqCacheTTLSeconds,
		cfg.ModeInference.DigitalWindowSeconds,
		cfg.ModeInference.DigitalMinCorroborators,
		len(modeSeeds),
		cfg.ModeInference.DigitalSeedTTLSeconds)

	// Start peering manager (DXSpider PC protocol) if enabled.
	var peerManager *peer.Manager
	if cfg.Peering.Enabled {
		pm, err := peer.NewManager(cfg.Peering, cfg.Peering.LocalCallsign, ingestInput, cfg.SpotPolicy.MaxAgeSeconds, dropReporter)
		if err != nil {
			log.Fatalf("Failed to init peering manager: %v", err)
		}
		if err := pm.Start(ctx); err != nil {
			log.Fatalf("Failed to start peering manager: %v", err)
		}
		peerManager = pm
		log.Printf("Peering: listen_port=%d peers=%d hop=%d keepalive=%ds", cfg.Peering.ListenPort, len(cfg.Peering.Peers), cfg.Peering.HopCount, cfg.Peering.KeepaliveSeconds)
	}

	// Initialize archive writer (optional) before wiring consumers that need read access.
	var archiveWriter *archive.Writer
	if cfg.Archive.Enabled {
		if w, err := archive.NewWriter(cfg.Archive); err != nil {
			log.Printf("Warning: archive disabled due to init error: %v", err)
		} else {
			archiveWriter = w
			archiveWriter.Start()
			log.Printf("Archive: writing to %s (batch=%d/%dms queue=%d cleanup=%ds ft_retention=%ds other_retention=%ds)", cfg.Archive.DBPath, cfg.Archive.BatchSize, cfg.Archive.BatchIntervalMS, cfg.Archive.QueueSize, cfg.Archive.CleanupIntervalSeconds, cfg.Archive.RetentionFTSeconds, cfg.Archive.RetentionDefaultSeconds)
			defer archiveWriter.Stop()
		}
	}

	// Create command processor (SHOW/DX reads from archive when available, otherwise ring buffer)
	processor := commands.NewProcessor(spotBuffer, archiveWriter, ingestInput, ctyLookup, repGate, repDropReporter)

	// Create and start telnet server
	telnetServer := telnet.NewServer(telnet.ServerOptions{
		Port:                    cfg.Telnet.Port,
		WelcomeMessage:          cfg.Telnet.WelcomeMessage,
		DuplicateLoginMsg:       cfg.Telnet.DuplicateLoginMsg,
		LoginGreeting:           cfg.Telnet.LoginGreeting,
		LoginPrompt:             cfg.Telnet.LoginPrompt,
		LoginEmptyMessage:       cfg.Telnet.LoginEmptyMessage,
		LoginInvalidMessage:     cfg.Telnet.LoginInvalidMessage,
		InputTooLongMessage:     cfg.Telnet.InputTooLongMessage,
		InputInvalidCharMessage: cfg.Telnet.InputInvalidCharMessage,
		DialectWelcomeMessage:   cfg.Telnet.DialectWelcomeMessage,
		DialectSourceDefault:    cfg.Telnet.DialectSourceDefaultLabel,
		DialectSourcePersisted:  cfg.Telnet.DialectSourcePersistedLabel,
		PathStatusMessage:       cfg.Telnet.PathStatusMessage,
		ClusterCall:             cfg.Server.NodeID,
		MaxConnections:          cfg.Telnet.MaxConnections,
		BroadcastWorkers:        cfg.Telnet.BroadcastWorkers,
		BroadcastQueue:          cfg.Telnet.BroadcastQueue,
		WorkerQueue:             cfg.Telnet.WorkerQueue,
		ClientBuffer:            cfg.Telnet.ClientBuffer,
		BroadcastBatchInterval:  time.Duration(cfg.Telnet.BroadcastBatchIntervalMS) * time.Millisecond,
		Transport:               cfg.Telnet.Transport,
		EchoMode:                cfg.Telnet.EchoMode,
		SkipHandshake:           cfg.Telnet.SkipHandshake,
		LoginLineLimit:          cfg.Telnet.LoginLineLimit,
		CommandLineLimit:        cfg.Telnet.CommandLineLimit,
		ReputationGate:          repGate,
		PathPredictor:           pathPredictor,
		PathDisplayEnabled:      pathCfg.DisplayEnabled,
		NoiseOffsets:            pathCfg.NoiseOffsets,
		GridLookup:              gridLookup,
		CTYLookup:               ctyLookup,
		DedupeFastEnabled:       secondaryFastWindow > 0,
		DedupeSlowEnabled:       secondarySlowWindow > 0,
	}, processor)

	err = telnetServer.Start()
	if err != nil {
		log.Fatalf("Failed to start telnet server: %v", err)
	}
	// Hook peering raw passthrough (e.g., PC26) into telnet broadcast once available.
	if peerManager != nil {
		peerManager.SetRawBroadcast(telnetServer.BroadcastRaw)
		peerManager.SetWWVBroadcast(telnetServer.BroadcastWWV)
		peerManager.SetAnnouncementBroadcast(telnetServer.BroadcastAnnouncement)
		peerManager.SetDirectMessage(telnetServer.SendDirectMessage)
		peerManager.SetUserCountProvider(telnetServer.GetClientCount)
	}

	// Start the unified output processor once the telnet server is ready
	var lastOutput atomic.Int64
	var secondaryStageCount atomic.Uint64
	// Purpose: Run the single-threaded output pipeline for deduped spots.
	// Key aspects: Handles corrections, licensing, secondary dedupe, and fan-out.
	// Upstream: main startup after wiring dependencies.
	// Downstream: processOutputSpots.
	go processOutputSpots(deduplicator, secondaryFast, secondarySlow, &secondaryStageCount, modeAssigner, spotBuffer, telnetServer, peerManager, statsTracker, correctionIndex, cfg.CallCorrection, ctyLookup, metaCache, harmonicDetector, cfg.Harmonics, &knownCalls, freqAverager, cfg.SpotPolicy, ui, gridUpdater, gridLookup, gridLookupSync, unlicensedReporter, corrLogger, callCooldown, adaptiveMinReports, refresher, spotterReliability, cfg.RBN.KeepSSIDSuffix, archiveWriter, &lastOutput, pathPredictor)
	startPipelineHealthMonitor(ctx, deduplicator, &lastOutput, peerManager)

	// Connect to RBN CW/RTTY feed if enabled (port 7000)
	// RBN spots go INTO the deduplicator input channel
	var rbnClient *rbn.Client
	if cfg.RBN.Enabled {
		rbnClient = rbn.NewClient(cfg.RBN.Host, cfg.RBN.Port, cfg.RBN.Callsign, cfg.RBN.Name, skewStore, cfg.RBN.KeepSSIDSuffix, cfg.RBN.SlotBuffer)
		rbnClient.SetTelnetTransport(cfg.RBN.TelnetTransport)
		if cfg.RBN.KeepaliveSec > 0 {
			rbnClient.EnableKeepalive(time.Duration(cfg.RBN.KeepaliveSec) * time.Second)
		}
		err = rbnClient.Connect()
		if err != nil {
			log.Printf("Warning: Failed to connect to RBN CW/RTTY: %v", err)
		} else {
			// Purpose: Pump CW/RTTY RBN spots into the dedup input channel.
			// Key aspects: Runs in its own goroutine to keep ingest non-blocking.
			// Upstream: main startup after RBN connect.
			// Downstream: processRBNSpots.
			go processRBNSpots(rbnClient, ingestInput, "RBN-CW", cfg.SpotPolicy)
			log.Println("RBN CW/RTTY client feeding spots into unified dedup engine")
		}
	}

	// Connect to RBN Digital feed if enabled (port 7001 - FT4/FT8)
	// RBN Digital spots go INTO the deduplicator input channel
	var rbnDigitalClient *rbn.Client
	if cfg.RBNDigital.Enabled {
		rbnDigitalClient = rbn.NewClient(cfg.RBNDigital.Host, cfg.RBNDigital.Port, cfg.RBNDigital.Callsign, cfg.RBNDigital.Name, skewStore, cfg.RBNDigital.KeepSSIDSuffix, cfg.RBNDigital.SlotBuffer)
		rbnDigitalClient.SetTelnetTransport(cfg.RBNDigital.TelnetTransport)
		if cfg.RBNDigital.KeepaliveSec > 0 {
			rbnDigitalClient.EnableKeepalive(time.Duration(cfg.RBNDigital.KeepaliveSec) * time.Second)
		}
		err = rbnDigitalClient.Connect()
		if err != nil {
			log.Printf("Warning: Failed to connect to RBN Digital: %v", err)
		} else {
			// Purpose: Pump FT4/FT8 RBN digital spots into the dedup input channel.
			// Key aspects: Runs in its own goroutine to keep ingest non-blocking.
			// Upstream: main startup after RBN Digital connect.
			// Downstream: processRBNSpots.
			go processRBNSpots(rbnDigitalClient, ingestInput, "RBN-FT", cfg.SpotPolicy)
			log.Println("RBN Digital (FT4/FT8) client feeding spots into unified dedup engine")
		}
	}

	// Connect to human/relay telnet feed if enabled (upstream cluster or operator-submitted spots)
	var humanTelnetClient *rbn.Client
	if cfg.HumanTelnet.Enabled {
		rawPassthrough := make(chan string, 256)
		// Purpose: Forward non-DX lines (WWV/announcements) to telnet clients.
		// Key aspects: Filters by line type; exits when channel closes.
		// Upstream: humanTelnetClient raw passthrough channel.
		// Downstream: telnetServer.BroadcastWWV and BroadcastAnnouncement.
		go func() {
			for line := range rawPassthrough {
				if telnetServer == nil {
					continue
				}
				if kind := wwvKindFromLine(line); kind != "" {
					telnetServer.BroadcastWWV(kind, line)
					continue
				}
				if announcement := announcementFromLine(line); announcement != "" {
					telnetServer.BroadcastAnnouncement(announcement)
				}
			}
		}()

		humanTelnetClient = rbn.NewClient(cfg.HumanTelnet.Host, cfg.HumanTelnet.Port, cfg.HumanTelnet.Callsign, cfg.HumanTelnet.Name, skewStore, cfg.HumanTelnet.KeepSSIDSuffix, cfg.HumanTelnet.SlotBuffer)
		humanTelnetClient.SetTelnetTransport(cfg.HumanTelnet.TelnetTransport)
		humanTelnetClient.UseMinimalParser()
		humanTelnetClient.SetRawPassthrough(rawPassthrough)
		if cfg.HumanTelnet.KeepaliveSec > 0 {
			// Prevent idle disconnects on upstream telnet feeds by sending periodic CRLF.
			humanTelnetClient.EnableKeepalive(time.Duration(cfg.HumanTelnet.KeepaliveSec) * time.Second)
		}
		err = humanTelnetClient.Connect()
		if err != nil {
			log.Printf("Warning: Failed to connect to human/relay telnet feed: %v", err)
		} else {
			// Purpose: Pump human/relay telnet spots into the dedup input channel.
			// Key aspects: Runs in its own goroutine to keep ingest non-blocking.
			// Upstream: main startup after human telnet connect.
			// Downstream: processHumanTelnetSpots.
			go processHumanTelnetSpots(humanTelnetClient, ingestInput, "HUMAN-TELNET", cfg.SpotPolicy)
			log.Println("Human/relay telnet client feeding spots into unified dedup engine")
		}
	}

	// Connect to PSKReporter if enabled
	// PSKReporter spots go INTO the deduplicator input channel
	var (
		pskrClient *pskreporter.Client
		pskrTopics []string
	)
	if cfg.PSKReporter.Enabled {
		pskrTopics = cfg.PSKReporter.SubscriptionTopics()
		pskrClient = pskreporter.NewClient(cfg.PSKReporter.Broker, cfg.PSKReporter.Port, pskrTopics, cfg.PSKReporter.Modes, cfg.PSKReporter.Name, cfg.PSKReporter.Workers, skewStore, cfg.PSKReporter.AppendSpotterSSID, cfg.PSKReporter.SpotChannelSize, cfg.PSKReporter.MaxPayloadBytes)
		err = pskrClient.Connect()
		if err != nil {
			log.Printf("Warning: Failed to connect to PSKReporter: %v", err)
		} else {
			// Purpose: Pump PSKReporter spots into the dedup input channel.
			// Key aspects: Runs in its own goroutine to keep ingest non-blocking.
			// Upstream: main startup after PSKReporter connect.
			// Downstream: processPSKRSpots.
			go processPSKRSpots(pskrClient, ingestInput, cfg.SpotPolicy)
			log.Println("PSKReporter client feeding spots into unified dedup engine")
		}
	}

	// Start stats display goroutine
	statsInterval := time.Duration(cfg.Stats.DisplayIntervalSeconds) * time.Second
	// Purpose: Periodically emit stats to UI or logs.
	// Key aspects: Runs on ticker interval until shutdown.
	// Upstream: main startup.
	// Downstream: displayStatsWithFCC.
	go displayStatsWithFCC(statsInterval, statsTracker, ingestValidator, deduplicator, secondaryFast, secondarySlow, &secondaryStageCount, spotBuffer, ctyLookup, metaCache, ctyState, &knownCalls, telnetServer, ui, gridUpdateState, gridStore, cfg.FCCULS.DBPath, pathPredictor)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Println("Cluster is running. Press Ctrl+C to stop.")
	log.Printf("Connect via: telnet localhost %d", cfg.Telnet.Port)
	if cfg.RBN.Enabled {
		log.Println("Receiving CW/RTTY spots from RBN (port 7000)...")
	}
	if cfg.RBNDigital.Enabled {
		log.Println("Receiving FT4/FT8 spots from RBN Digital (port 7001)...")
	}
	if cfg.PSKReporter.Enabled {
		topicList := strings.Join(pskrTopics, ", ")
		if topicList == "" {
			topicList = "<none>"
		}
		log.Printf("Receiving digital mode spots from PSKReporter (topics: %s)...", topicList)
	}
	if cfg.HumanTelnet.Enabled {
		log.Printf("Receiving human/relay spots from %s:%d...", cfg.HumanTelnet.Host, cfg.HumanTelnet.Port)
	}
	if cfg.Dedup.ClusterWindowSeconds > 0 {
		log.Printf("Unified deduplication active: %d second window", cfg.Dedup.ClusterWindowSeconds)
	} else {
		log.Println("Unified deduplication bypassed (window=0); duplicates are not filtered")
	}
	log.Println("Architecture: ALL sources -> Dedup Engine -> Ring Buffer -> Clients")
	log.Printf("Statistics will be displayed every %d seconds...", cfg.Stats.DisplayIntervalSeconds)
	log.Println("---")
	maybeStartHeapLogger()
	maybeStartDiagServer()

	// Wait for shutdown signal
	sig := <-sigChan
	log.Printf("Received signal: %v", sig)
	log.Println("Shutting down gracefully...")

	// Stop periodic cleanup loops
	if freqAverager != nil {
		freqAverager.StopCleanup()
	}
	if harmonicDetector != nil {
		harmonicDetector.StopCleanup()
	}
	if correctionIndex != nil {
		correctionIndex.StopCleanup()
	}

	// Stop deduplicator
	if deduplicator != nil {
		deduplicator.Stop()
	}
	if peerManager != nil {
		peerManager.Stop()
	}
	if secondaryFast != nil {
		secondaryFast.Stop()
	}
	if secondarySlow != nil {
		secondarySlow.Stop()
	}

	// Stop RBN CW/RTTY client
	if rbnClient != nil {
		rbnClient.Stop()
	}

	// Stop RBN Digital client
	if rbnDigitalClient != nil {
		rbnDigitalClient.Stop()
	}

	// Stop PSKReporter client
	if pskrClient != nil {
		pskrClient.Stop()
	}

	// Stop the telnet server
	telnetServer.Stop()

	if corrLogger != nil {
		if err := corrLogger.Close(); err != nil {
			log.Printf("Warning: call-correction decision logger close: %v", err)
		}
		dropped := corrLogger.Dropped()
		if dropped > 0 {
			log.Printf("Call-correction decision logger dropped %d entries under load", dropped)
		}
	}

	log.Println("Cluster stopped")
}

// Purpose: Build a reporter callback for unlicensed drops.
// Key aspects: Returns a closure that increments stats and formats output.
// Upstream: main wiring for applyLicenseGate reporting.
// Downstream: tracker.IncrementUnlicensedDrops and dash.AppendUnlicensed/log.Println.
func makeUnlicensedReporter(dash uiSurface, tracker *stats.Tracker) func(source, role, call, mode string, freq float64) {
	// Purpose: Emit an unlicensed drop event with consistent formatting.
	// Key aspects: Normalizes fields and routes to UI or log.
	// Upstream: applyLicenseGate.
	// Downstream: tracker.IncrementUnlicensedDrops, dash.AppendUnlicensed, log.Println.
	return func(source, role, call, mode string, freq float64) {
		if tracker != nil {
			tracker.IncrementUnlicensedDrops()
		}
		source = strings.ToUpper(strings.TrimSpace(source))
		role = strings.ToUpper(strings.TrimSpace(role))
		mode = strings.ToUpper(strings.TrimSpace(mode))
		call = strings.TrimSpace(strings.ToUpper(call))

		message := fmt.Sprintf("Unlicensed US %s %s dropped from %s %s @ %.1f kHz", role, call, source, mode, freq)
		if dash != nil {
			colored := fmt.Sprintf("Unlicensed US %s [red]%s[-] dropped from %s %s @ %.1f kHz", role, call, source, mode, freq)
			dash.AppendUnlicensed(colored)
			return
		}
		log.Println(message)
	}
}

// Purpose: Build a reporter callback for dropped events.
// Key aspects: Routes to dropped pane when UI is active, otherwise logs.
// Upstream: CTY/PC61/reputation drop paths.
// Downstream: dash.AppendDropped and log.Print.
func makeDroppedReporter(dash uiSurface) func(line string) {
	return func(line string) {
		if line == "" {
			return
		}
		if dash != nil {
			dash.AppendDropped(line)
			return
		}
		log.Print(line)
	}
}

// Purpose: Build a reporter for reputation gate drops.
// Key aspects: Updates counters and routes to the dropped pane or logs.
// Upstream: Reputation gate in telnet command path.
// Downstream: stats tracker and dropped/system logs.
func makeReputationDropReporter(dropReporter func(string), tracker *stats.Tracker, cfg config.ReputationConfig) func(reputation.DropEvent) {
	if tracker == nil {
		return nil
	}
	sampleEvery := sampleEveryN(cfg.DropLogSampleRate)
	var counter atomic.Uint64
	return func(ev reputation.DropEvent) {
		tracker.IncrementReputationDrop(string(ev.Reason))
		if !cfg.ConsoleDropDisplay || sampleEvery == 0 {
			return
		}
		if sampleEvery > 1 {
			if counter.Add(1)%uint64(sampleEvery) != 0 {
				return
			}
		}
		line := formatReputationDropLine(ev)
		if dropReporter != nil {
			dropReporter(line)
			return
		}
		log.Print(line)
	}
}

func formatReputationDropLine(ev reputation.DropEvent) string {
	call := strings.TrimSpace(ev.Call)
	if max := spot.MaxCallsignLength(); max > 0 && len(call) > max {
		call = call[:max]
	}
	band := spot.NormalizeBand(ev.Band)
	if band == "" {
		band = "???"
	}
	reason := string(ev.Reason)
	if reason == "" {
		reason = "unknown"
	}
	flags := formatPenaltyFlags(ev.Flags)
	asn := strings.TrimSpace(ev.ASN)
	country := strings.TrimSpace(ev.CountryCode)
	if country == "" {
		country = strings.TrimSpace(ev.CountryName)
	}
	prefix := strings.TrimSpace(ev.Prefix)
	if prefix == "" {
		prefix = "unknown"
	}
	return fmt.Sprintf("Reputation drop: %s band=%s reason=%s ip=%s asn=%s country=%s flags=%s",
		call, band, reason, prefix, emptyOr(asn, "unknown"), emptyOr(country, "unknown"), flags)
}

func formatPenaltyFlags(flags reputation.PenaltyFlags) string {
	if flags == 0 {
		return "none"
	}
	out := make([]string, 0, 4)
	if flags.Has(reputation.PenaltyCountryMismatch) {
		out = append(out, "mismatch")
	}
	if flags.Has(reputation.PenaltyASNReset) {
		out = append(out, "asn_new")
	}
	if flags.Has(reputation.PenaltyGeoFlip) {
		out = append(out, "geo_flip")
	}
	if flags.Has(reputation.PenaltyDisagreement) {
		out = append(out, "disagree")
	}
	if flags.Has(reputation.PenaltyUnknown) {
		out = append(out, "unknown")
	}
	return strings.Join(out, ",")
}

func sampleEveryN(rate float64) int {
	if rate <= 0 {
		return 0
	}
	if rate >= 1 {
		return 1
	}
	n := int(1.0 / rate)
	if n < 1 {
		return 1
	}
	return n
}

func emptyOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formatReputationDropSummary(total uint64, reasons map[string]uint64) string {
	if total == 0 {
		return "Reputation drops: 0"
	}
	type pair struct {
		key   string
		count uint64
	}
	items := make([]pair, 0, len(reasons))
	for key, count := range reasons {
		items = append(items, pair{key: key, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].key < items[j].key
		}
		return items[i].count > items[j].count
	})
	limit := 4
	if len(items) < limit {
		limit = len(items)
	}
	var b strings.Builder
	b.WriteString("Reputation drops: ")
	b.WriteString(humanize.Comma(int64(total)))
	if limit == 0 {
		return b.String()
	}
	b.WriteString(" (")
	for i := 0; i < limit; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s=%d", items[i].key, items[i].count)
	}
	b.WriteString(")")
	return b.String()
}

// Purpose: Periodically emit stats with FCC metadata refresh.
// Key aspects: Uses a ticker, diff counters, and optional secondary dedupe stats.
// Upstream: main stats goroutine.
// Downstream: tracker accessors, loadFCCSnapshot, and UI/log output.
func displayStatsWithFCC(interval time.Duration, tracker *stats.Tracker, ingestStats *ingestValidator, dedup *dedup.Deduplicator, secondaryFast *dedup.SecondaryDeduper, secondarySlow *dedup.SecondaryDeduper, secondaryStage *atomic.Uint64, buf *buffer.RingBuffer, ctyLookup func() *cty.CTYDatabase, metaCache *callMetaCache, ctyState *ctyRefreshState, knownPtr *atomic.Pointer[spot.KnownCallsigns], telnetSrv *telnet.Server, dash uiSurface, gridStats *gridMetrics, gridDB *gridstore.Store, fccDBPath string, pathPredictor *pathreliability.Predictor) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	prevSourceCounts := make(map[string]uint64)
	prevSourceModeCounts := make(map[string]uint64)
	fccSnap := loadFCCSnapshot(fccDBPath)

	for range ticker.C {
		// Refresh FCC snapshot each interval to reflect completed downloads/builds.
		fccSnap = loadFCCSnapshot(fccDBPath)

		sourceTotals := tracker.GetSourceCounts()
		sourceModeTotals := tracker.GetSourceModeCounts()

		rbnTotal := diffCounter(sourceTotals, prevSourceCounts, "RBN")
		rbnCW := diffSourceMode(sourceModeTotals, prevSourceModeCounts, "RBN", "CW")
		rbnRTTY := diffSourceMode(sourceModeTotals, prevSourceModeCounts, "RBN", "RTTY")

		rbnFTTotal := diffCounter(sourceTotals, prevSourceCounts, "RBN-DIGITAL")
		rbnFT8 := diffSourceMode(sourceModeTotals, prevSourceModeCounts, "RBN-DIGITAL", "FT8")
		rbnFT4 := diffSourceMode(sourceModeTotals, prevSourceModeCounts, "RBN-DIGITAL", "FT4")

		// PSKReporter includes a per-mode breakdown in the stats ticker.
		pskTotal := diffCounter(sourceTotals, prevSourceCounts, "PSKREPORTER")
		pskCW := diffSourceMode(sourceModeTotals, prevSourceModeCounts, "PSKREPORTER", "CW")
		pskRTTY := diffSourceMode(sourceModeTotals, prevSourceModeCounts, "PSKREPORTER", "RTTY")
		pskFT8 := diffSourceMode(sourceModeTotals, prevSourceModeCounts, "PSKREPORTER", "FT8")
		pskFT4 := diffSourceMode(sourceModeTotals, prevSourceModeCounts, "PSKREPORTER", "FT4")
		pskMSK144 := diffSourceMode(sourceModeTotals, prevSourceModeCounts, "PSKREPORTER", "MSK144")

		totalCorrections := tracker.CallCorrections()
		totalUnlicensed := tracker.UnlicensedDrops()
		totalFreqCorrections := tracker.FrequencyCorrections()
		totalHarmonics := tracker.HarmonicSuppressions()
		reputationTotal := tracker.ReputationDrops()
		reputationReasons := tracker.ReputationDropReasons()

		ingestTotal := uint64(0)
		if ingestStats != nil {
			ingestTotal = ingestStats.IngestCount()
		}

		var pipelineLine string
		if dedup == nil {
			pipelineLine = "Pipeline: primary dedup disabled"
		} else {
			primaryProcessed, _, _ := dedup.GetStats()

			secondaryStageCount := uint64(0)
			if secondaryStage != nil {
				secondaryStageCount = secondaryStage.Load()
			}

			fastForwarded := secondaryStageCount
			slowForwarded := secondaryStageCount
			if secondaryFast != nil {
				secProcessed, secDupes, _ := secondaryFast.GetStats()
				if secDupes < secProcessed {
					fastForwarded = secProcessed - secDupes
				} else {
					fastForwarded = 0
				}
			}
			if secondarySlow != nil {
				secProcessed, secDupes, _ := secondarySlow.GetStats()
				if secDupes < secProcessed {
					slowForwarded = secProcessed - secDupes
				} else {
					slowForwarded = 0
				}
			}
			if secondaryFast == nil && secondarySlow != nil {
				fastForwarded = slowForwarded
			}
			if secondarySlow == nil && secondaryFast != nil {
				slowForwarded = fastForwarded
			}

			fastPercent := 0
			slowPercent := 0
			if ingestTotal > 0 {
				fastPercent = int((fastForwarded * 100) / ingestTotal)
				slowPercent = int((slowForwarded * 100) / ingestTotal)
			}
			pipelineLine = fmt.Sprintf("Pipeline: %s | %s | %s/%d%% (F) / %s/%d%% (S)",
				humanize.Comma(int64(ingestTotal)),
				humanize.Comma(int64(primaryProcessed)),
				humanize.Comma(int64(fastForwarded)),
				fastPercent,
				humanize.Comma(int64(slowForwarded)),
				slowPercent)
		}

		var queueDrops, clientDrops, senderFailures uint64
		var clientCount int
		if telnetSrv != nil {
			queueDrops, clientDrops, senderFailures = telnetSrv.BroadcastMetricSnapshot()
			clientCount = telnetSrv.GetClientCount()
		}

		combinedRBN := rbnTotal + rbnFTTotal
		lines := []string{
			fmt.Sprintf("%s   %s", formatUptimeLine(tracker.GetUptime()), formatMemoryLine(buf, dedup, secondaryFast, secondarySlow, metaCache, knownPtr)), // 1
			formatGridLineOrPlaceholder(gridStats, gridDB, pathPredictor),                                                                                  // 2
			formatCTYLineOrPlaceholder(ctyLookup, ctyState),                                                                                                // 3
			formatFCCLineOrPlaceholder(fccSnap),                                                                                                            // 4
			fmt.Sprintf("RBN: %d TOTAL / %d CW / %d RTTY / %d FT8 / %d FT4", combinedRBN, rbnCW, rbnRTTY, rbnFT8, rbnFT4),                                  // 5
			fmt.Sprintf("PSKReporter: %s TOTAL / %s CW / %s RTTY / %s FT8 / %s FT4 / %s MSK144",
				humanize.Comma(int64(pskTotal)),
				humanize.Comma(int64(pskCW)),
				humanize.Comma(int64(pskRTTY)),
				humanize.Comma(int64(pskFT8)),
				humanize.Comma(int64(pskFT4)),
				humanize.Comma(int64(pskMSK144)),
			), // 6
			fmt.Sprintf("Corrected calls: %d (C) / %d (U) / %d (F) / %d (H)", totalCorrections, totalUnlicensed, totalFreqCorrections, totalHarmonics), // 7
			formatReputationDropSummary(reputationTotal, reputationReasons),                                                                            // 8
			pipelineLine, // 9
			fmt.Sprintf("Telnet: %d clients. Drops: %d (Q) / %d (C) / %d (W)", clientCount, queueDrops, clientDrops, senderFailures), // 10
		}

		prevSourceCounts = sourceTotals
		prevSourceModeCounts = sourceModeTotals

		if dash != nil {
			dash.SetStats(lines)
		} else {
			for _, line := range lines {
				log.Print(line)
			}
			log.Print("") // spacer between stats and status/messages
		}
	}
}

// processRBNSpots receives spots from RBN and sends to deduplicator
// This is the UNIFIED ARCHITECTURE path
// RBN → Deduplicator Input Channel
// Purpose: Feed RBN spots into the unified deduplicator input.
// Key aspects: Drops stale spots and avoids blocking on dedup input.
// Upstream: RBN client ingest goroutine.
// Downstream: deduplicator.GetInputChannel and isStale.
func processRBNSpots(client *rbn.Client, ingest chan<- *spot.Spot, source string, spotPolicy config.SpotPolicy) {
	spotChan := client.GetSpotChannel()
	var drops atomic.Uint64

	for spot := range spotChan {
		if isStale(spot, spotPolicy) {
			continue
		}
		// Non-blocking send to avoid wedging ingest if dedup blocks.
		select {
		case ingest <- spot:
		default:
			count := drops.Add(1)
			if count == 1 || count%100 == 0 {
				log.Printf("%s: Ingest input full, dropping spot (total drops=%d)", source, count)
			}
		}
	}
	log.Printf("%s: Spot processing stopped", source)
}

// processHumanTelnetSpots marks incoming telnet spots as human-sourced and sends them into dedup.
// Purpose: Feed upstream human telnet spots into dedup after tagging as human.
// Key aspects: Ensures SourceType/Mode defaults and enforces staleness guard.
// Upstream: human telnet client ingest.
// Downstream: deduplicator.GetInputChannel and isStale.
func processHumanTelnetSpots(client *rbn.Client, ingest chan<- *spot.Spot, source string, spotPolicy config.SpotPolicy) {
	spotChan := client.GetSpotChannel()
	var drops atomic.Uint64

	for sp := range spotChan {
		if sp != nil {
			sp.IsHuman = true
			sp.SourceType = spot.SourceUpstream
			if strings.TrimSpace(sp.SourceNode) == "" {
				sp.SourceNode = source
			}
			if strings.TrimSpace(sp.Mode) == "" {
				sp.Mode = "RTTY" // temporary default until mode parser is added
				sp.EnsureNormalized()
			}
			if isStale(sp, spotPolicy) {
				continue
			}
		}
		select {
		case ingest <- sp:
		default:
			count := drops.Add(1)
			if count == 1 || count%100 == 0 {
				log.Printf("%s: Ingest input full, dropping spot (total drops=%d)", source, count)
			}
		}
	}
	log.Printf("%s: Spot processing stopped", source)
}

// processPSKRSpots receives spots from PSKReporter and sends to deduplicator
// PSKReporter → Deduplicator Input Channel
// Purpose: Feed PSKReporter spots into the unified deduplicator input.
// Key aspects: Drops stale spots and avoids blocking on dedup input.
// Upstream: PSKReporter client worker pool.
// Downstream: deduplicator.GetInputChannel and isStale.
func processPSKRSpots(client *pskreporter.Client, ingest chan<- *spot.Spot, spotPolicy config.SpotPolicy) {
	spotChan := client.GetSpotChannel()
	var drops atomic.Uint64

	for spot := range spotChan {
		if isStale(spot, spotPolicy) {
			continue
		}
		// Non-blocking send to avoid backing up the PSK worker pool when dedup is slow.
		select {
		case ingest <- spot:
		default:
			count := drops.Add(1)
			if count == 1 || count%100 == 0 {
				log.Printf("PSKReporter: Ingest input full, dropping spot (total drops=%d)", count)
			}
		}
	}
}

// isStale enforces the global max_age_seconds guard before deduplication so old
// spots are dropped early and do not consume dedupe/window resources.
// Purpose: Enforce the global max_age_seconds guard.
// Key aspects: Drops old spots early to reduce work.
// Upstream: ingest pipelines and output stage.
// Downstream: time.Since and policy.MaxAgeSeconds.
func isStale(s *spot.Spot, policy config.SpotPolicy) bool {
	if s == nil || policy.MaxAgeSeconds <= 0 {
		return false
	}
	if s.Time.IsZero() {
		return false
	}
	return time.Since(s.Time) > time.Duration(policy.MaxAgeSeconds)*time.Second
}

// processOutputSpots receives deduplicated spots and distributes them
// Deduplicator Output  Ring Buffer  Broadcast to Clients
// Purpose: Process deduplicated spots and distribute to ring buffer and outputs.
// Key aspects: Applies corrections, caching, licensing, secondary dedupe, and fan-out.
// Upstream: deduplicator output channel.
// Downstream: grid updates, telnet broadcast, archive writer, peer publish.
func processOutputSpots(
	deduplicator *dedup.Deduplicator,
	secondaryFast *dedup.SecondaryDeduper,
	secondarySlow *dedup.SecondaryDeduper,
	secondaryStage *atomic.Uint64,
	modeAssigner *spot.ModeAssigner,
	buf *buffer.RingBuffer,
	telnet *telnet.Server,
	peerManager *peer.Manager,
	tracker *stats.Tracker,
	correctionIdx *spot.CorrectionIndex,
	correctionCfg config.CallCorrectionConfig,
	ctyLookup func() *cty.CTYDatabase,
	metaCache *callMetaCache,
	harmonicDetector *spot.HarmonicDetector,
	harmonicCfg config.HarmonicConfig,
	knownCalls *atomic.Pointer[spot.KnownCallsigns],
	freqAvg *spot.FrequencyAverager,
	spotPolicy config.SpotPolicy,
	dash uiSurface,
	gridUpdate func(call, grid string),
	gridLookup func(call string) (string, bool),
	gridLookupSync func(call string) (string, bool),
	unlicensedReporter func(source, role, call, mode string, freq float64),
	corrLogger spot.CorrectionTraceLogger,
	callCooldown *spot.CallCooldown,
	adaptiveMinReports *spot.AdaptiveMinReports,
	refresher *adaptiveRefresher,
	spotterReliability spot.SpotterReliability,
	broadcastKeepSSID bool,
	archiveWriter *archive.Writer,
	lastOutput *atomic.Int64,
	pathPredictor *pathreliability.Predictor,
) {
	outputChan := deduplicator.GetOutputChannel()
	secondaryActive := secondaryFast != nil || secondarySlow != nil

	for s := range outputChan {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("processOutputSpots panic: %v\n%s", r, debug.Stack())
				}
			}()

			if s == nil {
				return
			}
			s.EnsureNormalized()
			spot.ApplySourceHumanFlag(s)
			explicitMode := strings.TrimSpace(s.Mode) != ""
			if modeAssigner != nil {
				modeAssigner.Assign(s, explicitMode)
			}
			s.EnsureNormalized()
			ctyDB := ctyLookup()
			dirty := false
			modeUpper := s.ModeNorm
			if refresher != nil {
				refresher.IncrementSpots()
			}

			// CTY validation happens in the ingest gate before deduplication.

			s.RefreshBeaconFlag()

			if s.IsBeacon {
				// Beacons are tagged with a strong confidence so they still display a glyph.
				s.Confidence = "V"
			}

			if !s.IsBeacon && spotPolicy.MaxAgeSeconds > 0 {
				if time.Since(s.Time) > time.Duration(spotPolicy.MaxAgeSeconds)*time.Second {
					// log.Printf("Spot dropped (stale): %s at %.1fkHz (age=%ds)", s.DXCall, s.Frequency, int(time.Since(s.Time).Seconds()))
					return
				}
			}

			var suppress bool
			if telnet != nil && !s.IsBeacon {
				suppress = maybeApplyCallCorrectionWithLogger(s, correctionIdx, correctionCfg, ctyDB, metaCache, knownCalls, tracker, dash, corrLogger, callCooldown, adaptiveMinReports, spotterReliability)
				if suppress {
					return
				}
				// Call correction can change the DX call; recompute beacon flag accordingly.
				s.RefreshBeaconFlag()
				dirty = true
			}

			if !s.IsBeacon && harmonicDetector != nil && harmonicCfg.Enabled {
				if drop, fundamental, corroborators, deltaDB := harmonicDetector.ShouldDrop(s, time.Now().UTC()); drop {
					harmonicMsg := fmt.Sprintf("Harmonic suppressed: %s %.1f -> %.1f kHz (%d / %d dB)", s.DXCall, s.Frequency, fundamental, corroborators, deltaDB)
					harmonicMsgDash := harmonicMsg
					if dash != nil {
						harmonicMsgDash = fmt.Sprintf("Harmonic suppressed: %s [red]%.1f[-] -> [green]%.1f[-] kHz (%d / %d dB)", s.DXCall, s.Frequency, fundamental, corroborators, deltaDB)
					}
					if tracker != nil {
						tracker.IncrementHarmonicSuppressions()
					}
					if dash != nil {
						dash.AppendHarmonic(harmonicMsgDash)
					} else {
						log.Println(harmonicMsg)
					}
					return
				}
			}

			if !s.IsBeacon && freqAvg != nil && shouldAverageFrequency(s) {
				window := frequencyAverageWindow(spotPolicy)
				tolerance := frequencyAverageTolerance(spotPolicy)
				dxCall := s.DXCallNorm
				if dxCall == "" {
					dxCall = s.DXCall
				}
				avg, corroborators, _ := freqAvg.Average(dxCall, s.Frequency, time.Now().UTC(), window, tolerance)
				// Half-up rounding to 0.1 kHz to avoid banker's rounding at .x5 boundaries.
				rounded := math.Floor(avg*10+0.5) / 10
				// Apply the averaged frequency when we have enough corroborators and the rounded
				// value actually differs from the reported frequency. We deliberately decouple
				// this apply threshold from the inclusion tolerance so sub-500 Hz shifts are
				// preserved instead of being discarded by the same 0.5 kHz gate.
				delta := math.Abs(rounded - s.Frequency)
				if corroborators >= spotPolicy.FrequencyAveragingMinReports && delta >= 0.05 {
					s.Frequency = rounded
					if tracker != nil {
						tracker.IncrementFrequencyCorrections()
					}
					// Frequency corrections are applied silently (no dedicated dashboard pane).
					dirty = true
				}
			}

			// Ensure confidence-capable modes carry at least a placeholder glyph when no correction applied.
			if !s.IsBeacon {
				if modeSupportsConfidenceGlyph(modeUpper) && strings.TrimSpace(s.Confidence) == "" {
					s.Confidence = "?"
					dirty = true
				}
				if applyKnownCallFloor(s, knownCalls) {
					dirty = true
				}
			}

			if dirty {
				s.EnsureNormalized()
				dirty = false
			}
			// Final license gate runs after corrections so busted calls can be fixed first.
			if applyLicenseGate(s, ctyDB, metaCache, unlicensedReporter) {
				return
			}
			// License gate refreshes metadata; normalize once more before stats/broadcast.
			if !dirty {
				// applyLicenseGate mutates metadata; mark dirty for final normalization.
				dirty = true
			}

			if dirty {
				s.EnsureNormalized()
				dirty = false
			}
			if tracker != nil {
				modeKey := modeUpper
				if modeKey == "" {
					modeKey = string(s.SourceType)
				}
				tracker.IncrementMode(modeKey)

				sourceName := strings.ToUpper(strings.TrimSpace(s.SourceNode))
				if sourceName != "" {
					tracker.IncrementSource(sourceName)
					tracker.IncrementSourceMode(sourceName, modeKey)
				}
			}

			if gridUpdate != nil {
			}

			if !broadcastKeepSSID {
				base := s.DECallNorm
				if base == "" {
					base = s.DECall
				}
				stripped := collapseSSIDForBroadcast(base)
				s.DECallStripped = stripped
				s.DECallNormStripped = stripped
			}

			// Keep test spots out of the ring buffer so SHOW DX history stays production-only.
			if buf != nil && shouldBufferSpot(s) {
				buf.Add(s)
			}

			// Ensure DE metadata is populated before secondary dedupe. Upstream CTY lookups
			// can be bypassed when spotters carry SSID tokens or CTY is missing; refresh
			// here so secondary dedupe has DXCC/zone available.
			if secondaryActive && (s.DEMetadata.ADIF <= 0 || s.DEMetadata.CQZone <= 0) && ctyDB != nil {
				call := s.DECallNorm
				if call == "" {
					call = s.DECall
				}
				call = normalizeCallForMetadata(call)
				if info := effectivePrefixInfo(ctyDB, metaCache, call); info != nil {
					deGrid := strings.TrimSpace(s.DEMetadata.Grid)
					s.DEMetadata = metadataFromPrefix(info)
					if deGrid != "" {
						s.DEMetadata.Grid = deGrid
					}
					// Metadata refresh can change continent/grid; clear cached norms and rebuild.
					s.InvalidateMetadataCache()
					s.EnsureNormalized()
				}
			}

			// Final fan-out guards (symmetry with peer belt-and-suspenders): do not
			// deliver stale spots to any downstream sink, even if an upstream stage
			// failed to drop them.
			if isStale(s, spotPolicy) {
				return
			}

			// Backfill grids before secondary dedupe so path reliability can use them.
			if gridLookup != nil {
				gridBackfilled := false
				syncGrid := gridLookupSync != nil && isRBNGridSource(s)
				dxCall := s.DXCallNorm
				if dxCall == "" {
					dxCall = s.DXCall
				}
				if strings.TrimSpace(s.DXMetadata.Grid) == "" {
					if syncGrid {
						if grid, ok := gridLookupSync(dxCall); ok {
							s.DXMetadata.Grid = grid
							gridBackfilled = true
						}
					}
					if strings.TrimSpace(s.DXMetadata.Grid) == "" {
						if grid, ok := gridLookup(dxCall); ok {
							s.DXMetadata.Grid = grid
							gridBackfilled = true
						}
					}
				}
				deCall := s.DECallNorm
				if deCall == "" {
					deCall = s.DECall
				}
				if strings.TrimSpace(s.DEMetadata.Grid) == "" {
					if syncGrid {
						if grid, ok := gridLookupSync(deCall); ok {
							s.DEMetadata.Grid = grid
							gridBackfilled = true
						}
					}
					if strings.TrimSpace(s.DEMetadata.Grid) == "" {
						if grid, ok := gridLookup(deCall); ok {
							s.DEMetadata.Grid = grid
							gridBackfilled = true
						}
					}
				}
				if gridBackfilled {
					s.InvalidateMetadataCache()
					s.EnsureNormalized()
				}
			}

			if pathPredictor != nil && pathPredictor.Config().Enabled {
				// Populate cached cells even when we skip updates so broadcast can reuse them.
				if s.DXCellID == 0 || s.DXCellID == 0xffff {
					s.DXCellID = uint16(pathreliability.EncodeCell(strings.TrimSpace(s.DXMetadata.Grid)))
				}
				if s.DECellID == 0 || s.DECellID == 0xffff {
					s.DECellID = uint16(pathreliability.EncodeCell(strings.TrimSpace(s.DEMetadata.Grid)))
				}
				if s.HasReport {
					mode := s.ModeNorm
					if strings.TrimSpace(mode) == "" {
						mode = s.Mode
					}
					if ft8, ok := pathreliability.FT8Equivalent(mode, s.Report, pathPredictor.Config()); ok {
						dxCell := pathreliability.CellID(s.DXCellID)
						deCell := pathreliability.CellID(s.DECellID)
						dxGrid2 := pathreliability.EncodeGrid2(s.DXMetadata.Grid)
						deGrid2 := pathreliability.EncodeGrid2(s.DEMetadata.Grid)
						band := s.BandNorm
						if strings.TrimSpace(band) == "" {
							band = s.Band
						}
						spotTime := s.Time.UTC()
						if spotTime.IsZero() {
							spotTime = time.Now().UTC()
						}
						bucket := pathreliability.BucketForIngest(mode)
						if bucket != pathreliability.BucketNone {
							// Spot SNR reflects DX -> DE (spotter is the receiver).
							pathPredictor.Update(bucket, deCell, dxCell, deGrid2, dxGrid2, band, ft8, 1.0, spotTime, s.IsBeacon)
						}
					}
				}
			}

			if secondaryStage != nil {
				secondaryStage.Add(1)
			}
			// Evaluate secondary dedupe per policy; slow falls back to fast when disabled.
			allowFast := true
			if secondaryFast != nil {
				allowFast = secondaryFast.ShouldForward(s)
			}
			allowSlow := allowFast
			if secondarySlow != nil {
				allowSlow = secondarySlow.ShouldForward(s)
			}

			// Broadcast-only dedupe: ring/history already updated above for non-test spots.
			if !allowFast && !allowSlow {
				if telnet != nil {
					telnet.DeliverSelfSpot(s)
				}
				return
			}

			if gridUpdate != nil {
				if dxGrid := strings.TrimSpace(s.DXMetadata.Grid); dxGrid != "" {
					dxCall := s.DXCallNorm
					if dxCall == "" {
						dxCall = s.DXCall
					}
					gridUpdate(dxCall, dxGrid)
				}
				if deGrid := strings.TrimSpace(s.DEMetadata.Grid); deGrid != "" {
					deCall := s.DECallNorm
					if deCall == "" {
						deCall = s.DECall
					}
					gridUpdate(deCall, deGrid)
				}
			}

			if lastOutput != nil {
				lastOutput.Store(time.Now().UTC().UnixNano())
			}

			if archiveWriter != nil && allowSlow && shouldArchiveSpot(s) {
				archiveWriter.Enqueue(s)
			}

			if telnet != nil {
				telnet.BroadcastSpot(s, allowFast, allowSlow)
			}
			if peerManager != nil && allowSlow && shouldPublishToPeers(s) {
				peerSpot := cloneSpotForPeerPublish(s)
				peerManager.PublishDX(peerSpot)
			}
		}()
	}
}

// Purpose: Gate ring-buffer storage for test spotters.
// Key aspects: Test spots are excluded so SHOW DX stays production-only.
// Upstream: processOutputSpots.
// Downstream: ring buffer Add.
func shouldBufferSpot(s *spot.Spot) bool {
	return s != nil && !s.IsTestSpotter
}

// Purpose: Gate archive persistence for test spotters.
// Key aspects: Test spots are excluded from Pebble history.
// Upstream: processOutputSpots.
// Downstream: archive.Writer.Enqueue.
func shouldArchiveSpot(s *spot.Spot) bool {
	return s != nil && !s.IsTestSpotter
}

// Purpose: Decide whether a spot should be forwarded to peers.
// Key aspects: Excludes upstream/peer sources and test spotters.
// Upstream: processOutputSpots.
// Downstream: peer.Manager.PublishDX.
func shouldPublishToPeers(s *spot.Spot) bool {
	if s == nil || s.IsTestSpotter {
		return false
	}
	switch s.SourceType {
	case spot.SourceUpstream, spot.SourcePeer:
		return false
	default:
		return true
	}
}

// startPipelineHealthMonitor logs warnings when the output pipeline or dedup
// goroutine appear stalled. It is intentionally lightweight and non-blocking.
// Purpose: Warn when dedup/output pipelines appear stalled.
// Key aspects: Periodic ticker checks without blocking hot paths.
// Upstream: main startup after pipeline wiring.
// Downstream: log.Printf, dedup.LastProcessedAt, peerManager.ReconnectCount.
func startPipelineHealthMonitor(ctx context.Context, dedup *dedup.Deduplicator, lastOutput *atomic.Int64, peerManager *peer.Manager) {
	const (
		checkInterval      = 30 * time.Second
		outputStallWarning = 2 * time.Minute
	)
	ticker := time.NewTicker(checkInterval)
	// Purpose: Periodically check for stalled output and dedup activity.
	// Key aspects: Exits on context cancellation and emits warnings.
	// Upstream: startPipelineHealthMonitor.
	// Downstream: ticker.Stop and log.Printf.
	go func() {
		defer ticker.Stop()
		var lastReconnects uint64
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now().UTC()
				if lastOutput != nil {
					ns := lastOutput.Load()
					if ns > 0 {
						age := now.Sub(time.Unix(0, ns))
						if age > outputStallWarning {
							reconnects := uint64(0)
							if peerManager != nil {
								reconnects = peerManager.ReconnectCount()
							}
							dedupStamp := "unknown"
							if dedup != nil {
								if last := dedup.LastProcessedAt(); !last.IsZero() {
									dedupStamp = last.UTC().Format(time.RFC3339)
								}
							}
							log.Printf("Warning: output pipeline idle for %s (dedup_last=%s, peer_reconnects=%d)", age, dedupStamp, reconnects)
						}
					}
				}
				if peerManager != nil {
					if reconnects := peerManager.ReconnectCount(); reconnects != lastReconnects {
						log.Printf("Peering: outbound reconnects=%d", reconnects)
						lastReconnects = reconnects
					}
				}
				if dedup != nil {
					if last := dedup.LastProcessedAt(); !last.IsZero() {
						if age := now.Sub(last); age > outputStallWarning {
							log.Printf("Warning: deduplicator idle for %s", age)
						}
					}
				}
			}
		}
	}()
}

// collapseSSIDForBroadcast trims SSID fragments so clients see a single
// skimmer identity (e.g., N2WQ-1-# -> N2WQ-#, N2WQ-1 -> N2WQ).
// It preserves non-numeric suffixes.
// Purpose: Normalize spotter SSIDs before telnet broadcast.
// Key aspects: Collapses numeric suffixes while preserving non-numeric tokens.
// Upstream: processOutputSpots.
// Downstream: stripNumericSSID.
func collapseSSIDForBroadcast(call string) string {
	call = strings.TrimSpace(call)
	if call == "" {
		return call
	}
	if strings.HasSuffix(call, "-#") {
		trimmed := strings.TrimSuffix(call, "-#")
		return stripNumericSSID(trimmed) + "-#"
	}
	return stripNumericSSID(call)
}

// Purpose: Remove a numeric SSID suffix (e.g., "-1") from a callsign.
// Key aspects: Leaves non-numeric suffixes intact.
// Upstream: collapseSSIDForBroadcast.
// Downstream: strings.LastIndexByte.
func stripNumericSSID(call string) string {
	idx := strings.LastIndexByte(call, '-')
	if idx <= 0 || idx == len(call)-1 {
		return call
	}
	suffix := call[idx+1:]
	for i := 0; i < len(suffix); i++ {
		if suffix[i] < '0' || suffix[i] > '9' {
			return call
		}
	}
	return call[:idx]
}

// stripTrailingHyphenSuffix removes any trailing hyphen suffix after the last slash.
// It preserves portable segments (e.g., "K1ABC-1/P") by only trimming when the
// suffix is the final segment.
func stripTrailingHyphenSuffix(call string) string {
	slash := strings.LastIndexByte(call, '/')
	start := 0
	if slash >= 0 {
		start = slash + 1
	}
	idx := strings.IndexByte(call[start:], '-')
	if idx < 0 {
		return call
	}
	trimAt := start + idx
	if trimAt <= 0 {
		return call
	}
	return call[:trimAt]
}

// normalizeCallForMetadata strips skimmer and hyphen suffixes before metadata lookups.
// It preserves portable segments (e.g., "/P") and does not mutate canonical calls.
func normalizeCallForMetadata(call string) string {
	call = strings.ToUpper(strings.TrimSpace(call))
	if call == "" {
		return call
	}
	return stripTrailingHyphenSuffix(call)
}

// isRBNGridSource reports whether a spot should use synchronous grid backfill.
// It targets RBN-originated feeds where grid data is often missing at ingest.
func isRBNGridSource(s *spot.Spot) bool {
	if s == nil {
		return false
	}
	switch s.SourceType {
	case spot.SourceRBN, spot.SourceFT8, spot.SourceFT4:
		return true
	default:
		return false
	}
}

// cloneSpotForPeerPublish ensures manual spots carry an inferred mode to peers
// even when the user omitted a comment. Peers only see the comment field in
// PC61/PC11 frames, so we fall back to the inferred mode when the comment is
// blank. Other sources and spots with comments are passed through as-is.
func cloneSpotForPeerPublish(src *spot.Spot) *spot.Spot {
	if src == nil {
		return nil
	}
	if src.SourceType != spot.SourceManual {
		return src
	}
	if strings.TrimSpace(src.Comment) != "" {
		return src
	}
	mode := strings.TrimSpace(src.Mode)
	if mode == "" {
		return src
	}
	clone := &spot.Spot{
		ID:                 src.ID,
		DXCall:             src.DXCall,
		DECall:             src.DECall,
		Frequency:          src.Frequency,
		Band:               src.Band,
		Mode:               src.Mode,
		Report:             src.Report,
		HasReport:          src.HasReport,
		Time:               src.Time,
		Comment:            src.Comment,
		SourceType:         src.SourceType,
		SourceNode:         src.SourceNode,
		SpotterIP:          src.SpotterIP,
		TTL:                src.TTL,
		IsHuman:            src.IsHuman,
		IsTestSpotter:      src.IsTestSpotter,
		IsBeacon:           src.IsBeacon,
		DXMetadata:         src.DXMetadata,
		DEMetadata:         src.DEMetadata,
		Confidence:         src.Confidence,
		ModeNorm:           src.ModeNorm,
		BandNorm:           src.BandNorm,
		DXCallNorm:         src.DXCallNorm,
		DECallNorm:         src.DECallNorm,
		DXContinentNorm:    src.DXContinentNorm,
		DEContinentNorm:    src.DEContinentNorm,
		DXGridNorm:         src.DXGridNorm,
		DEGridNorm:         src.DEGridNorm,
		DXGrid2:            src.DXGrid2,
		DEGrid2:            src.DEGrid2,
		DECallStripped:     src.DECallStripped,
		DECallNormStripped: src.DECallNormStripped,
	}
	clone.Comment = mode
	return clone
}

// applyLicenseGate runs the FCC license check after all corrections and returns true when the spot should be dropped.
// Purpose: Enforce FCC ULS licensing gates for US calls (DX only; DE checked at ingest).
// Key aspects: Uses cache, CTY metadata refresh for corrected calls, and reporter callback on drops.
// Upstream: processOutputSpots before broadcast.
// Downstream: licCache, uls.IsLicensedUS, reporter.
func applyLicenseGate(s *spot.Spot, ctyDB *cty.CTYDatabase, metaCache *callMetaCache, reporter func(source, role, call, mode string, freq float64)) bool {
	if s == nil {
		return false
	}
	if s.IsBeacon {
		return false
	}
	if ctyDB == nil {
		return false
	}

	dxCall := s.DXCallNorm
	if dxCall == "" {
		dxCall = s.DXCall
	}
	deCall := s.DECallNorm
	if deCall == "" {
		deCall = s.DECall
	}
	dxLookupCall := normalizeCallForMetadata(dxCall)
	deLookupCall := normalizeCallForMetadata(deCall)
	needsMetadata := s.DXMetadata.ADIF == 0 || s.DEMetadata.ADIF == 0 || s.Confidence == "C"
	if needsMetadata {
		dxInfo := effectivePrefixInfo(ctyDB, metaCache, dxLookupCall)
		deInfo := effectivePrefixInfo(ctyDB, metaCache, deLookupCall)

		// Refresh metadata from the final CTY match but preserve any grid data we already attached.
		dxGrid := strings.TrimSpace(s.DXMetadata.Grid)
		deGrid := strings.TrimSpace(s.DEMetadata.Grid)
		s.DXMetadata = metadataFromPrefix(dxInfo)
		s.DEMetadata = metadataFromPrefix(deInfo)
		if dxGrid != "" {
			s.DXMetadata.Grid = dxGrid
		}
		if deGrid != "" {
			s.DEMetadata.Grid = deGrid
		}
		// Metadata refresh can change continent/grid; clear cached norms and rebuild.
		s.InvalidateMetadataCache()
		s.EnsureNormalized()
	}

	// License checks use the base callsign (portable segment order-independent) so
	// location prefixes like /VE3 still map to the operator's home license.
	dxLicenseCall := strings.TrimSpace(uls.NormalizeForLicense(dxCall))
	var dxLicenseInfo *cty.PrefixInfo
	if dxLicenseCall != "" {
		dxLicenseInfo = effectivePrefixInfo(ctyDB, metaCache, dxLicenseCall)
	}

	now := time.Now()
	if dxLicenseInfo != nil && dxLicenseInfo.ADIF == 291 {
		callKey := dxLicenseCall
		if callKey == "" {
			callKey = dxCall
		}
		if licensed, ok := licCache.get(callKey, now); ok {
			if !licensed {
				if reporter != nil {
					reporter(s.SourceNode, "DX", callKey, s.ModeNorm, s.Frequency)
				}
				return true
			}
		} else if !uls.IsLicensedUS(callKey) {
			licCache.set(callKey, false, now)
			if reporter != nil {
				reporter(s.SourceNode, "DX", callKey, s.ModeNorm, s.Frequency)
			}
			return true
		} else {
			licCache.set(callKey, true, now)
		}
	}
	return false
}

// Purpose: Resolve prefix metadata for a callsign using cache + CTY database.
// Key aspects: Prefers portable slash prefixes (location) over base calls.
// Upstream: processOutputSpots DE metadata refresh and corrections.
// Downstream: callMetaCache.LookupCTY or cty.LookupCallsignPortable.
func effectivePrefixInfo(ctyDB *cty.CTYDatabase, metaCache *callMetaCache, call string) *cty.PrefixInfo {
	if ctyDB == nil {
		return nil
	}
	if call == "" {
		return nil
	}
	if metaCache != nil {
		if info, ok, _ := metaCache.LookupCTY(call, ctyDB); ok {
			return info
		}
		return nil
	}
	info, ok := ctyDB.LookupCallsignPortable(call)
	if !ok {
		return nil
	}
	return info
}

// Purpose: Convert CTY prefix info into spot.CallMetadata.
// Key aspects: Copies continent/country/zone fields into a struct.
// Upstream: effectivePrefixInfo consumers.
// Downstream: None (pure mapping).
func metadataFromPrefix(info *cty.PrefixInfo) spot.CallMetadata {
	if info == nil {
		return spot.CallMetadata{}
	}
	return spot.CallMetadata{
		Continent: info.Continent,
		Country:   info.Country,
		CQZone:    info.CQZone,
		ITUZone:   info.ITUZone,
		ADIF:      info.ADIF,
	}
}

// Purpose: Apply call correction and optionally log decision details.
// Key aspects: Evaluates corrections, updates stats, and can suppress spots.
// Upstream: processOutputSpots call correction stage.
// Downstream: spot.ApplyCallCorrection, traceLogger, tracker updates.
func maybeApplyCallCorrectionWithLogger(spotEntry *spot.Spot, idx *spot.CorrectionIndex, cfg config.CallCorrectionConfig, ctyDB *cty.CTYDatabase, metaCache *callMetaCache, knownPtr *atomic.Pointer[spot.KnownCallsigns], tracker *stats.Tracker, dash uiSurface, traceLogger spot.CorrectionTraceLogger, cooldown *spot.CallCooldown, adaptive *spot.AdaptiveMinReports, spotterReliability spot.SpotterReliability) bool {
	if spotEntry == nil {
		return false
	}
	if !spot.IsCallCorrectionCandidate(spotEntry.Mode) {
		// Leave any pre-seeded confidence intact for non-correction modes.
		return false
	}
	if idx == nil || !cfg.Enabled {
		if strings.TrimSpace(spotEntry.Confidence) == "" {
			spotEntry.Confidence = "?"
		}
		return false
	}

	now := time.Now().UTC()
	window := callCorrectionWindow(cfg)
	defer idx.Add(spotEntry, now, window)

	modeUpper := strings.ToUpper(strings.TrimSpace(spotEntry.Mode))
	// Voice signals are wider, so use sideband-specific correction windows.
	isVoice := modeUpper == "USB" || modeUpper == "LSB"

	if adaptive != nil && (modeUpper == "CW" || modeUpper == "RTTY") {
		reporter := spotEntry.DECallNorm
		if reporter == "" {
			reporter = spotEntry.DECall
		}
		adaptive.Observe(spotEntry.Band, reporter, now)
	}

	minReports := cfg.MinConsensusReports
	cooldownMinReports := cfg.CooldownMinReporters
	if adaptive != nil && (modeUpper == "CW" || modeUpper == "RTTY") {
		if dyn := adaptive.MinReportsForBand(spotEntry.Band, now); dyn > 0 {
			minReports = dyn
			cooldownMinReports = dyn
		}
	}
	state := "normal"
	if adaptive != nil {
		state = adaptive.StateForBand(spotEntry.Band, now)
	}
	qualityBinHz := cfg.QualityBinHz
	freqToleranceHz := cfg.FrequencyToleranceHz
	if isVoice {
		freqToleranceHz = cfg.VoiceFrequencyToleranceHz
	} else if params, ok := resolveBandStateParams(cfg.BandStateOverrides, spotEntry.Band, state); ok {
		if params.QualityBinHz > 0 {
			qualityBinHz = params.QualityBinHz
		}
		if params.FrequencyToleranceHz > 0 {
			freqToleranceHz = params.FrequencyToleranceHz
		}
	}

	settings := spot.CorrectionSettings{
		MinConsensusReports:      minReports,
		MinAdvantage:             cfg.MinAdvantage,
		MinConfidencePercent:     cfg.MinConfidencePercent,
		MaxEditDistance:          cfg.MaxEditDistance,
		RecencyWindow:            window,
		Strategy:                 cfg.Strategy,
		MinSNRCW:                 cfg.MinSNRCW,
		MinSNRRTTY:               cfg.MinSNRRTTY,
		MinSNRVoice:              cfg.MinSNRVoice,
		DistanceModelCW:          cfg.DistanceModelCW,
		DistanceModelRTTY:        cfg.DistanceModelRTTY,
		Distance3ExtraReports:    cfg.Distance3ExtraReports,
		Distance3ExtraAdvantage:  cfg.Distance3ExtraAdvantage,
		Distance3ExtraConfidence: cfg.Distance3ExtraConfidence,
		DebugLog:                 cfg.DebugLog,
		TraceLogger:              traceLogger,
		FrequencyToleranceHz:     freqToleranceHz,
		QualityBinHz:             qualityBinHz,
		QualityGoodThreshold:     cfg.QualityGoodThreshold,
		QualityNewCallIncrement:  cfg.QualityNewCallIncrement,
		QualityBustedDecrement:   cfg.QualityBustedDecrement,
		SpotterReliability:       spotterReliability,
		MinSpotterReliability:    cfg.MinSpotterReliability,
		Cooldown:                 cooldown,
		CooldownMinReporters:     cooldownMinReports,
	}
	candidateWindowKHz := 0.5
	if isVoice {
		candidateWindowKHz = cfg.VoiceCandidateWindowKHz
	}
	others := idx.Candidates(spotEntry, now, window, candidateWindowKHz)
	entries := spotsToEntries(others)
	corrected, supporters, correctedConfidence, subjectConfidence, totalReporters, ok := spot.SuggestCallCorrection(spotEntry, entries, settings, now)

	spotEntry.Confidence = formatConfidence(subjectConfidence, totalReporters)

	if !ok {
		return false
	}

	message := fmt.Sprintf("Call corrected: %s -> %s at %.1f kHz (%d / %d%%)",
		spotEntry.DXCall, corrected, spotEntry.Frequency, supporters, correctedConfidence)
	messageDash := message
	if dash != nil {
		messageDash = fmt.Sprintf("Call corrected: [red]%s[-] -> [green]%s[-] at %.1f kHz (%d / %d%%)",
			spotEntry.DXCall, corrected, spotEntry.Frequency, supporters, correctedConfidence)
	}

	correctedNorm := ""
	if ctyDB != nil {
		correctedNorm = spot.NormalizeCallsign(corrected)
		if info := effectivePrefixInfo(ctyDB, metaCache, correctedNorm); info != nil {
			if dash != nil {
				dash.AppendCall(messageDash)
			} else {
				log.Println(message)
			}
			spotEntry.DXCall = correctedNorm
			spotEntry.DXCallNorm = correctedNorm
			spotEntry.Confidence = "C"
			if tracker != nil {
				tracker.IncrementCallCorrections()
			}
		} else {
			log.Printf("Call correction rejected (CTY miss): suggested %s at %.1f kHz", corrected, spotEntry.Frequency)
			if strings.EqualFold(cfg.InvalidAction, "suppress") {
				log.Printf("Call correction suppression engaged: dropping spot from %s at %.1f kHz", spotEntry.DXCall, spotEntry.Frequency)
				return true
			}
			spotEntry.Confidence = "B"
		}
		return false
	}

	if dash != nil {
		dash.AppendCall(messageDash)
	} else {
		log.Println(message)
	}
	correctedNorm = spot.NormalizeCallsign(corrected)
	spotEntry.DXCall = correctedNorm
	spotEntry.DXCallNorm = correctedNorm
	spotEntry.Confidence = "C"
	if tracker != nil {
		tracker.IncrementCallCorrections()
	}

	return false
}

// Purpose: Compute the time window for call correction recency.
// Key aspects: Uses config recency defaults and overrides.
// Upstream: main correctionIndex cleanup scheduling.
// Downstream: time.Duration math.
func callCorrectionWindow(cfg config.CallCorrectionConfig) time.Duration {
	if cfg.RecencySeconds <= 0 {
		return 45 * time.Second
	}
	return time.Duration(cfg.RecencySeconds) * time.Second
}

type bandStateParams struct {
	QualityBinHz         int
	FrequencyToleranceHz float64
}

// resolveBandStateParams returns per-band, per-state overrides when defined; otherwise false.
// Purpose: Resolve per-band/state overrides for call correction parameters.
// Key aspects: Matches band/state in override list; returns ok on match.
// Upstream: maybeApplyCallCorrectionWithLogger.
// Downstream: band/state override scanning.
func resolveBandStateParams(overrides []config.BandStateOverride, band, state string) (bandStateParams, bool) {
	b := strings.ToLower(strings.TrimSpace(band))
	if b == "" || len(overrides) == 0 {
		return bandStateParams{}, false
	}
	stateKey := strings.ToLower(strings.TrimSpace(state))
	for _, o := range overrides {
		for _, candidate := range o.Bands {
			if strings.ToLower(strings.TrimSpace(candidate)) != b {
				continue
			}
			switch stateKey {
			case "quiet":
				return bandStateParams{
					QualityBinHz:         o.Quiet.QualityBinHz,
					FrequencyToleranceHz: o.Quiet.FrequencyToleranceHz,
				}, true
			case "busy":
				return bandStateParams{
					QualityBinHz:         o.Busy.QualityBinHz,
					FrequencyToleranceHz: o.Busy.FrequencyToleranceHz,
				}, true
			default:
				return bandStateParams{
					QualityBinHz:         o.Normal.QualityBinHz,
					FrequencyToleranceHz: o.Normal.FrequencyToleranceHz,
				}, true
			}
		}
	}
	return bandStateParams{}, false
}

// Purpose: Compute the frequency averaging look-back window.
// Key aspects: Uses policy defaults with a minimum of zero.
// Upstream: processOutputSpots frequency averaging path.
// Downstream: time.Duration math.
func frequencyAverageWindow(policy config.SpotPolicy) time.Duration {
	seconds := policy.FrequencyAveragingSeconds
	if seconds <= 0 {
		seconds = 45
	}
	return time.Duration(seconds) * time.Second
}

// Purpose: Compute the frequency averaging tolerance in kHz.
// Key aspects: Converts Hz config to kHz float.
// Upstream: processOutputSpots frequency averaging path.
// Downstream: float math.
func frequencyAverageTolerance(policy config.SpotPolicy) float64 {
	toleranceHz := policy.FrequencyAveragingToleranceHz
	if toleranceHz <= 0 {
		toleranceHz = 300
	}
	return toleranceHz / 1000.0
}

// Purpose: Decide whether a mode should carry confidence glyphs.
// Key aspects: Treats USB/LSB as voice modes; digital modes remain exempt.
// Upstream: processOutputSpots confidence seeding and fallback.
// Downstream: strings.ToUpper/TrimSpace.
func modeSupportsConfidenceGlyph(mode string) bool {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case "CW", "RTTY", "USB", "LSB":
		return true
	default:
		return false
	}
}

// Purpose: Apply SCP known-call promotion only when confidence is still unknown.
// Key aspects: If confidence is '?', upgrade to 'S' when the DX call is in SCP.
// Upstream: processOutputSpots after correction/confidence assignment.
// Downstream: KnownCallsigns.Contains and modeSupportsConfidenceGlyph.
func applyKnownCallFloor(s *spot.Spot, knownCalls *atomic.Pointer[spot.KnownCallsigns]) bool {
	if s == nil || s.IsBeacon {
		return false
	}
	mode := s.ModeNorm
	if mode == "" {
		mode = s.Mode
	}
	if !modeSupportsConfidenceGlyph(mode) {
		return false
	}
	if strings.TrimSpace(s.Confidence) != "?" {
		return false
	}
	if knownCalls == nil {
		return false
	}
	call := s.DXCallNorm
	if call == "" {
		call = s.DXCall
	}
	if call == "" {
		return false
	}
	if known := knownCalls.Load(); known != nil && known.Contains(call) {
		s.Confidence = "S"
		return true
	}
	return false
}

// spotsToEntries converts []*spot.Spot to bandmap.SpotEntry using Hz units for frequency.
// Purpose: Convert spots into bandmap entries.
// Key aspects: Maps DX call, frequency, and time into bandmap format.
// Upstream: bandmap updates in processOutputSpots.
// Downstream: bandmap.SpotEntry allocation.
func spotsToEntries(spots []*spot.Spot) []bandmap.SpotEntry {
	if len(spots) == 0 {
		return nil
	}
	entries := make([]bandmap.SpotEntry, 0, len(spots))
	for _, s := range spots {
		if s == nil {
			continue
		}
		entries = append(entries, bandmap.SpotEntry{
			Call:    s.DXCall,
			Spotter: s.DECall,
			Mode:    s.Mode,
			FreqHz:  uint32(s.Frequency*1000 + 0.5),
			Time:    s.Time.Unix(),
			SNR:     s.Report,
		})
	}
	return entries
}

// Purpose: Format the confidence string for corrected calls.
// Key aspects: Encodes percent-only consensus buckets (P/V/?); SCP floor applied later.
// Upstream: maybeApplyCallCorrectionWithLogger.
// Downstream: None (pure mapping).
func formatConfidence(percent int, totalReporters int) string {
	if totalReporters <= 1 {
		return "?"
	}

	value := percent
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}

	switch {
	case value >= 51:
		return "V"
	case value >= 25:
		return "P"
	default:
		return "?"
	}
}

// Purpose: Decide whether a spot is eligible for frequency averaging.
// Key aspects: Skips digital modes and requires a valid call/frequency.
// Upstream: processOutputSpots frequency averaging path.
// Downstream: string checks on mode and call.
func shouldAverageFrequency(s *spot.Spot) bool {
	mode := strings.ToUpper(strings.TrimSpace(s.Mode))
	return mode == "CW" || mode == "RTTY"
}

// Purpose: Download and load the RBN skew correction table.
// Key aspects: Uses configured URL and refreshes the in-memory store.
// Upstream: startSkewScheduler and startup initialization.
// Downstream: skew.Download, skew.LoadBytes, store.Set.
func refreshSkewTable(cfg config.SkewConfig, store *skew.Store) (int, error) {
	if store == nil {
		return 0, errors.New("skew: store is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	entries, err := skew.Fetch(ctx, cfg.URL)
	if err != nil {
		return 0, fmt.Errorf("skew: fetch failed: %w", err)
	}
	filtered := skew.FilterEntries(entries, cfg.MinSpots)
	if len(filtered) == 0 {
		return 0, fmt.Errorf("skew: no entries after filtering (min_spots=%d)", cfg.MinSpots)
	}
	table, err := skew.NewTable(filtered)
	if err != nil {
		return 0, fmt.Errorf("skew: build table: %w", err)
	}
	store.Set(table)
	if err := skew.WriteJSON(filtered, cfg.File); err != nil {
		return 0, fmt.Errorf("skew: write json: %w", err)
	}
	return table.Count(), nil
}

// Purpose: Periodically refresh the skew table based on configured schedule.
// Key aspects: Sleeps until next refresh time and exits on ctx.Done.
// Upstream: main startup when skew is enabled.
// Downstream: refreshSkewTable and nextSkewRefreshDelay.
func startSkewScheduler(ctx context.Context, cfg config.SkewConfig, store *skew.Store) {
	if store == nil {
		return
	}
	// Purpose: Background refresh loop for skew table updates.
	// Key aspects: Waits for computed delays and respects context cancellation.
	// Upstream: startSkewScheduler.
	// Downstream: refreshSkewTable and time.NewTimer.
	go func() {
		for {
			delay := nextSkewRefreshDelay(cfg, time.Now().UTC())
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if count, err := refreshSkewTable(cfg, store); err != nil {
				log.Printf("Warning: scheduled RBN skew download failed: %v", err)
			} else {
				log.Printf("Scheduled RBN skew download complete (%d entries)", count)
			}
		}
	}()
}

// Purpose: Compute delay until the next skew refresh time.
// Key aspects: Uses configured hour/minute and wraps to next day.
// Upstream: startSkewScheduler.
// Downstream: skewRefreshHourMinute and time math.
func nextSkewRefreshDelay(cfg config.SkewConfig, now time.Time) time.Duration {
	hour, minute := skewRefreshHourMinute(cfg)
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(now)
}

// Purpose: Resolve the target hour/minute for skew refresh.
// Key aspects: Defaults to 02:00 UTC when unset.
// Upstream: nextSkewRefreshDelay.
// Downstream: None.
func skewRefreshHourMinute(cfg config.SkewConfig) (int, int) {
	refresh := strings.TrimSpace(cfg.RefreshUTC)
	if refresh == "" {
		refresh = "00:30"
	}
	if parsed, err := time.Parse("15:04", refresh); err == nil {
		return parsed.Hour(), parsed.Minute()
	}
	return 0, 30
}

// Purpose: Periodically refresh the known callsigns dataset.
// Key aspects: Scheduled daily refresh and atomic pointer swap.
// Upstream: main startup when known calls are enabled.
// Downstream: refreshKnownCallsigns, seedKnownCalls, and time.NewTimer.
// startKnownCallScheduler downloads the known-calls file at the configured UTC
// time every day and updates the in-memory cache pointer after each refresh.
func startKnownCallScheduler(ctx context.Context, cfg config.KnownCallsConfig, knownPtr *atomic.Pointer[spot.KnownCallsigns], store *gridstore.Store, metaCache *callMetaCache) {
	if knownPtr == nil {
		return
	}
	// Purpose: Background refresh loop for known callsigns.
	// Key aspects: Waits until next scheduled time; exits on ctx.Done.
	// Upstream: startKnownCallScheduler.
	// Downstream: refreshKnownCallsigns and time.NewTimer.
	go func() {
		for {
			delay := nextKnownCallRefreshDelay(cfg, time.Now().UTC())
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if fresh, updated, err := refreshKnownCallsigns(cfg); err != nil {
				log.Printf("Warning: scheduled known calls download failed: %v", err)
			} else if updated && fresh != nil {
				knownPtr.Store(fresh)
				log.Printf("Scheduled known calls download complete (%d entries)", fresh.Count())
				if store != nil {
					if err := seedKnownCalls(store, fresh); err != nil {
						log.Printf("Warning: failed to reseed known calls into grid database: %v", err)
					}
				}
				if metaCache != nil {
					metaCache.Clear()
				}
			} else {
				log.Printf("Scheduled known calls download: up to date (%s)", cfg.File)
			}
		}
	}()
}

// Purpose: Download and parse the known calls file when updated.
// Key aspects: Uses conditional HTTP download and returns (cache, updated).
// Upstream: startKnownCallScheduler and startup.
// Downstream: download.Download and spot.LoadKnownCallsigns.
// refreshKnownCallsigns downloads the known calls file, writes it to disk, and
// returns the parsed cache when the remote content changed.
func refreshKnownCallsigns(cfg config.KnownCallsConfig) (*spot.KnownCallsigns, bool, error) {
	url := strings.TrimSpace(cfg.URL)
	path := strings.TrimSpace(cfg.File)
	if url == "" {
		return nil, false, errors.New("known calls: URL is empty")
	}
	if path == "" {
		return nil, false, errors.New("known calls: file path is empty")
	}
	result, err := download.Download(context.Background(), download.Request{
		URL:         url,
		Destination: path,
		Timeout:     1 * time.Minute,
	})
	if err != nil {
		return nil, false, fmt.Errorf("known calls: %w", err)
	}
	if result.Status != download.StatusUpdated {
		return nil, false, nil
	}
	known, err := spot.LoadKnownCallsigns(path)
	if err != nil {
		return nil, true, err
	}
	return known, true, nil
}

// Purpose: Compute delay until the next known calls refresh time.
// Key aspects: Uses configured hour/minute and wraps to next day.
// Upstream: startKnownCallScheduler.
// Downstream: knownCallRefreshHourMinute and time math.
func nextKnownCallRefreshDelay(cfg config.KnownCallsConfig, now time.Time) time.Duration {
	hour, minute := knownCallRefreshHourMinute(cfg)
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(now)
}

// Purpose: Resolve the target hour/minute for known call refresh.
// Key aspects: Defaults to 03:00 UTC when unset.
// Upstream: nextKnownCallRefreshDelay.
// Downstream: time.Parse.
func knownCallRefreshHourMinute(cfg config.KnownCallsConfig) (int, int) {
	refresh := strings.TrimSpace(cfg.RefreshUTC)
	if refresh == "" {
		refresh = "01:00"
	}
	if parsed, err := time.Parse("15:04", refresh); err == nil {
		return parsed.Hour(), parsed.Minute()
	}
	return 1, 0
}

// Purpose: Periodically refresh the CTY database from remote URL.
// Key aspects: Scheduled daily refresh, retry with backoff, atomic pointer swap.
// Upstream: main startup when CTY is enabled.
// Downstream: refreshCTYDatabase and time.NewTimer.
// startCTYScheduler downloads cty.plist at the configured UTC time every day and
// updates the in-memory CTY database pointer after each refresh.
func startCTYScheduler(ctx context.Context, cfg config.CTYConfig, ctyPtr *atomic.Pointer[cty.CTYDatabase], metaCache *callMetaCache, state *ctyRefreshState) {
	if ctyPtr == nil {
		return
	}
	// Purpose: Background refresh loop for CTY database updates.
	// Key aspects: Waits until next scheduled time, retries with backoff, records age/failures.
	// Upstream: startCTYScheduler.
	// Downstream: refreshCTYDatabase and time.NewTimer.
	go func() {
		const (
			ctyRetryBase = 1 * time.Minute
			ctyRetryMax  = 30 * time.Minute
		)
		for {
			delay := nextCTYRefreshDelay(cfg, time.Now().UTC())
			if !sleepWithContext(ctx, delay) {
				return
			}

			backoff := ctyRetryBase
			attempt := 0
			for {
				fresh, updated, err := refreshCTYDatabase(cfg)
				if err == nil {
					if updated && fresh != nil {
						ctyPtr.Store(fresh)
						if metaCache != nil {
							metaCache.Clear()
						}
						log.Printf("Scheduled CTY download complete (%d prefixes)", len(fresh.Keys))
					} else {
						log.Printf("Scheduled CTY download: up to date (%s)", cfg.File)
					}
					if state != nil {
						state.recordSuccess(time.Now().UTC())
					}
					break
				}
				attempt++
				if state != nil {
					state.recordFailure(time.Now().UTC(), err)
				}
				lastAge := "unknown"
				if state != nil {
					if age, ok := state.age(time.Now().UTC()); ok {
						lastAge = formatDurationShort(age)
					}
				}
				log.Printf("Warning: scheduled CTY download failed (attempt=%d last_success=%s next_retry=%s): %v", attempt, lastAge, backoff, err)
				if !sleepWithContext(ctx, backoff) {
					return
				}
				backoff *= 2
				if backoff > ctyRetryMax {
					backoff = ctyRetryMax
				}
			}
		}
	}()
}

// Purpose: Sleep for a duration unless the context is canceled.
// Key aspects: Timer-based wait with cancellation.
// Upstream: CTY refresh scheduler.
// Downstream: time.NewTimer.
func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Purpose: Download and parse the CTY database when updated.
// Key aspects: Uses conditional HTTP download and returns (db, updated).
// Upstream: startCTYScheduler and startup.
// Downstream: download.Download and cty.LoadCTYDatabase.
// refreshCTYDatabase downloads cty.plist, writes it atomically, and returns the parsed DB.
func refreshCTYDatabase(cfg config.CTYConfig) (*cty.CTYDatabase, bool, error) {
	url := strings.TrimSpace(cfg.URL)
	path := strings.TrimSpace(cfg.File)
	if url == "" {
		return nil, false, errors.New("cty: URL is empty")
	}
	if path == "" {
		return nil, false, errors.New("cty: file path is empty")
	}
	result, err := download.Download(context.Background(), download.Request{
		URL:         url,
		Destination: path,
		Timeout:     1 * time.Minute,
	})
	if err != nil {
		return nil, false, fmt.Errorf("cty: %w", err)
	}
	if result.Status != download.StatusUpdated {
		return nil, false, nil
	}
	db, err := cty.LoadCTYDatabase(path)
	if err != nil {
		return nil, true, fmt.Errorf("cty: load: %w", err)
	}
	return db, true, nil
}

// Purpose: Compute delay until the next CTY refresh time.
// Key aspects: Uses configured hour/minute and wraps to next day.
// Upstream: startCTYScheduler.
// Downstream: ctyRefreshHourMinute and time math.
func nextCTYRefreshDelay(cfg config.CTYConfig, now time.Time) time.Duration {
	hour, minute := ctyRefreshHourMinute(cfg)
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(now)
}

// Purpose: Resolve the target hour/minute for CTY refresh.
// Key aspects: Defaults to 04:00 UTC when unset.
// Upstream: nextCTYRefreshDelay.
// Downstream: time.Parse.
func ctyRefreshHourMinute(cfg config.CTYConfig) (int, int) {
	refresh := strings.TrimSpace(cfg.RefreshUTC)
	if refresh == "" {
		refresh = "00:45"
	}
	if parsed, err := time.Parse("15:04", refresh); err == nil {
		return parsed.Hour(), parsed.Minute()
	}
	return 0, 45
}

// Purpose: Format the grid database status line for stats output.
// Key aspects: Uses cache hit rate and DB counts when available.
// Upstream: displayStatsWithFCC.
// Downstream: gridstore.Store.Count and humanize.Comma.
func formatGridLine(metrics *gridMetrics, store *gridstore.Store, predictor *pathreliability.Predictor) string {
	updatesSinceStart := metrics.learnedTotal.Load()
	cacheLookups := metrics.cacheLookups.Load()
	cacheHits := metrics.cacheHits.Load()

	dbTotal := int64(-1)
	if store != nil {
		if count, err := store.Count(); err == nil {
			dbTotal = count
		} else {
			log.Printf("Warning: gridstore count failed: %v", err)
		}
	}
	hitRate := 0.0
	if cacheLookups > 0 {
		hitRate = float64(cacheHits) * 100 / float64(cacheLookups)
	}
	hitPercent := int(math.Ceil(hitRate))

	var propPairs string
	if predictor != nil {
		stats := predictor.Stats(time.Now().UTC())
		if stats.BaselineFine > 0 || stats.BaselineCoarse > 0 || stats.NarrowFine > 0 || stats.NarrowCoarse > 0 {
			propPairs = fmt.Sprintf(" | Prop pairs: base %s/%s nb %s/%s",
				humanize.Comma(int64(stats.BaselineFine)),
				humanize.Comma(int64(stats.BaselineCoarse)),
				humanize.Comma(int64(stats.NarrowFine)),
				humanize.Comma(int64(stats.NarrowCoarse)))
		}
	}
	if dbTotal >= 0 {
		return fmt.Sprintf("Grid database: %s TOTAL / %d%%%s",
			humanize.Comma(dbTotal),
			hitPercent,
			propPairs)
	}
	return fmt.Sprintf("Grid database: %s UPDATED / %d%%%s",
		humanize.Comma(int64(updatesSinceStart)),
		hitPercent,
		propPairs)
}

type fccSnapshot struct {
	HDCount   int64
	AMCount   int64
	DBSize    int64
	UpdatedAt time.Time
	Path      string
}

type ctyRefreshState struct {
	lastSuccess  atomic.Int64
	lastFailure  atomic.Int64
	failureCount atomic.Int64
	lastError    atomic.Value
}

func newCTYRefreshState() *ctyRefreshState {
	state := &ctyRefreshState{}
	state.lastError.Store("")
	return state
}

func (s *ctyRefreshState) recordSuccess(now time.Time) {
	if s == nil {
		return
	}
	s.lastSuccess.Store(now.Unix())
	s.failureCount.Store(0)
	s.lastError.Store("")
}

func (s *ctyRefreshState) recordFailure(now time.Time, err error) {
	if s == nil {
		return
	}
	s.lastFailure.Store(now.Unix())
	s.failureCount.Add(1)
	if err != nil {
		s.lastError.Store(err.Error())
	}
}

func (s *ctyRefreshState) age(now time.Time) (time.Duration, bool) {
	if s == nil {
		return 0, false
	}
	ts := s.lastSuccess.Load()
	if ts <= 0 {
		return 0, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now.Sub(time.Unix(ts, 0)), true
}

func (s *ctyRefreshState) failures() (int64, string) {
	if s == nil {
		return 0, ""
	}
	var errText string
	if val := s.lastError.Load(); val != nil {
		if str, ok := val.(string); ok {
			errText = str
		}
	}
	return s.failureCount.Load(), errText
}

// Purpose: Format FCC database status line for stats output.
// Key aspects: Includes counts, DB size, and update timestamp.
// Upstream: displayStatsWithFCC.
// Downstream: humanize.Comma and time formatting.
func formatFCCLine(fcc *fccSnapshot) string {
	if fcc == nil {
		return ""
	}
	ts := ""
	if !fcc.UpdatedAt.IsZero() {
		ts = fcc.UpdatedAt.UTC().Format("01-02-2006 15:04:05")
	}
	return fmt.Sprintf("FCC ULS: %s records. Last updated %s", humanize.Comma(fcc.HDCount), ts)
}

// Purpose: Format grid status or a placeholder when disabled/unavailable.
// Key aspects: Falls back to a placeholder when metrics/store missing.
// Upstream: displayStatsWithFCC.
// Downstream: formatGridLine.
func formatGridLineOrPlaceholder(metrics *gridMetrics, store *gridstore.Store, predictor *pathreliability.Predictor) string {
	if metrics == nil {
		return "Grid database: (not available)"
	}
	return formatGridLine(metrics, store, predictor)
}

// Purpose: Format FCC status or a placeholder when disabled/unavailable.
// Key aspects: Falls back to a placeholder when snapshot missing.
// Upstream: displayStatsWithFCC.
// Downstream: formatFCCLine.
func formatFCCLineOrPlaceholder(fcc *fccSnapshot) string {
	if fcc == nil {
		return "FCC ULS: (not available)"
	}
	return formatFCCLine(fcc)
}

// Purpose: Format CTY refresh status line for stats output.
// Key aspects: Reports age since last successful refresh and failure count.
// Upstream: displayStatsWithFCC.
// Downstream: ctyRefreshState.age and formatDurationShort.
func formatCTYLineOrPlaceholder(ctyLookup func() *cty.CTYDatabase, state *ctyRefreshState) string {
	if ctyLookup == nil || ctyLookup() == nil {
		return "CTY: (not loaded)"
	}
	if state == nil {
		return "CTY: loaded"
	}
	age, ok := state.age(time.Now().UTC())
	if !ok {
		return "CTY: loaded (age unknown)"
	}
	failures, _ := state.failures()
	if failures > 0 {
		return fmt.Sprintf("CTY: age %s (failures=%d)", formatDurationShort(age), failures)
	}
	return fmt.Sprintf("CTY: age %s", formatDurationShort(age))
}

// Purpose: Seed the grid database with known calls.
// Key aspects: Writes known call grids and logs failures.
// Upstream: main startup and known calls refresh scheduler.
// Downstream: gridstore.Store.Set and known calls iterator.
func seedKnownCalls(store *gridstore.Store, known *spot.KnownCallsigns) error {
	if store == nil || known == nil {
		return nil
	}
	if err := store.ClearKnownFlags(); err != nil {
		return err
	}
	calls := known.List()
	if len(calls) == 0 {
		return nil
	}
	records := make([]gridstore.Record, 0, len(calls))
	now := time.Now().UTC()
	for _, call := range calls {
		records = append(records, gridstore.Record{
			Call:         call,
			IsKnown:      true,
			Observations: 0,
			FirstSeen:    now,
			UpdatedAt:    now,
		})
	}
	return store.UpsertBatch(records)
}

// Purpose: Start the grid writer pipeline and return update hooks.
// Key aspects: Provides enqueue, metrics, stop, and lookup functions.
// Upstream: main grid store setup.
// Downstream: gridstore.Store and callMetaCache methods.
func startGridWriter(store *gridstore.Store, flushInterval time.Duration, cache *callMetaCache, ttl time.Duration, dbCheckOnMiss bool) (func(call, grid string), func(call string, info *cty.PrefixInfo), *gridMetrics, func(), func(call string) (string, bool), func(call string) (string, bool)) {
	if store == nil {
		return nil, nil, nil, nil, nil, nil
	}
	if flushInterval <= 0 {
		flushInterval = 60 * time.Second
	}
	metrics := &gridMetrics{}
	// dbCheckOnMiss gates async cache backfill and tight-timeout sync lookups.
	// Sync lookups are opt-in per caller so the main hot path stays non-blocking.
	asyncLookupEnabled := dbCheckOnMiss
	syncLookupEnabled := dbCheckOnMiss && gridSyncLookupWorkers > 0 && gridSyncLookupQueueDepth > 0 && gridSyncLookupTimeout > 0
	updates := make(chan gridstore.Record, 8192)
	done := make(chan struct{})
	lookupQueue := make(chan gridLookupRequest, 4096)
	lookupDone := make(chan struct{})
	var lookupPendingMu sync.Mutex
	lookupPending := make(map[string]struct{})
	var syncLookupQueue chan gridLookupRequest
	var syncLookupWG sync.WaitGroup
	if syncLookupEnabled {
		syncLookupQueue = make(chan gridLookupRequest, gridSyncLookupQueueDepth)
	}

	mergePending := func(existing, incoming gridstore.Record) gridstore.Record {
		merged := existing
		if incoming.Grid.Valid {
			merged.Grid = incoming.Grid
		}
		if incoming.IsKnown {
			merged.IsKnown = true
		}
		if incoming.CTYValid {
			merged.CTYValid = true
			merged.CTYADIF = incoming.CTYADIF
			merged.CTYCQZone = incoming.CTYCQZone
			merged.CTYITUZone = incoming.CTYITUZone
			merged.CTYContinent = incoming.CTYContinent
			merged.CTYCountry = incoming.CTYCountry
		}
		if incoming.Observations > 0 {
			merged.Observations += incoming.Observations
		}
		if !incoming.FirstSeen.IsZero() {
			if merged.FirstSeen.IsZero() || incoming.FirstSeen.Before(merged.FirstSeen) {
				merged.FirstSeen = incoming.FirstSeen
			}
		}
		if !incoming.UpdatedAt.IsZero() {
			if merged.UpdatedAt.IsZero() || incoming.UpdatedAt.After(merged.UpdatedAt) {
				merged.UpdatedAt = incoming.UpdatedAt
			}
		}
		if incoming.ExpiresAt != nil {
			merged.ExpiresAt = incoming.ExpiresAt
		}
		return merged
	}

	lookupRecord := func(baseCall, rawCall string) (*gridstore.Record, error) {
		if baseCall == "" {
			return nil, nil
		}
		rec, err := store.Get(baseCall)
		if err != nil || rec != nil {
			return rec, err
		}
		if rawCall != "" && rawCall != baseCall {
			return store.Get(rawCall)
		}
		return nil, nil
	}

	// Purpose: Background writer loop to batch metadata updates and periodic TTL purges.
	// Key aspects: Flushes on size/interval and closes done on exit.
	// Upstream: startGridWriter.
	// Downstream: store.UpsertBatch, store.PurgeOlderThan, and metrics updates.
	go func() {
		defer close(done)
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()
		var ttlTicker *time.Ticker
		var ttlCh <-chan time.Time
		if ttl > 0 {
			ttlTicker = time.NewTicker(ttl)
			defer ttlTicker.Stop()
			ttlCh = ttlTicker.C
		}

		pending := make(map[string]gridstore.Record)
		// Purpose: Flush pending updates to the database in a batch.
		// Key aspects: Retains batch on busy errors; clears on success.
		// Upstream: update loop and ticker ticks.
		// Downstream: store.UpsertBatch, metrics.learnedTotal.
		flush := func() {
			if len(pending) == 0 {
				return
			}
			batch := make([]gridstore.Record, 0, len(pending))
			gridUpdates := 0
			for _, rec := range pending {
				if rec.Grid.Valid {
					gridUpdates++
				}
				batch = append(batch, rec)
			}
			start := time.Now()
			if err := store.UpsertBatch(batch); err != nil {
				if gridstore.IsBusyError(err) {
					// Keep the batch in-memory so a later flush can retry after the lock clears.
					log.Printf("Warning: gridstore batch upsert busy (retaining %d pending): %v", len(batch), err)
					return
				}
				log.Printf("Warning: gridstore batch upsert failed (dropping %d pending): %v", len(batch), err)
				clear(pending)
				return
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				log.Printf("Gridstore: batch upsert %d records in %s", len(batch), elapsed)
			}
			if gridUpdates > 0 {
				metrics.learnedTotal.Add(uint64(gridUpdates))
			}
			clear(pending)
		}

		for {
			select {
			case rec, ok := <-updates:
				if !ok {
					flush()
					return
				}
				call := normalizeCallForMetadata(rec.Call)
				if call == "" {
					continue
				}
				rec.Call = call
				if existing, exists := pending[call]; exists {
					pending[call] = mergePending(existing, rec)
				} else {
					pending[call] = rec
				}
				if len(pending) >= 500 {
					flush()
				}
			case <-ticker.C:
				flush()
			case <-ttlCh:
				cutoff := time.Now().UTC().Add(-ttl)
				if removed, err := store.PurgeOlderThan(cutoff); err != nil {
					log.Printf("Warning: gridstore TTL purge failed: %v", err)
				} else if removed > 0 {
					log.Printf("Gridstore: purged %d entries older than %v", removed, ttl)
				}
			}
		}
	}()

	// Purpose: Enqueue a grid update without blocking the output pipeline.
	// Key aspects: Normalizes call/grid, uses cache to suppress duplicates.
	// Upstream: processOutputSpots gridUpdate hook.
	// Downstream: cache.UpdateGrid and updates channel.
	gridUpdateFn := func(call, grid string) {
		call = normalizeCallForMetadata(call)
		grid = strings.TrimSpace(strings.ToUpper(grid))
		if call == "" || len(grid) < 4 {
			return
		}
		if cache != nil && !cache.UpdateGrid(call, grid) {
			return
		}
		now := time.Now().UTC()
		rec := gridstore.Record{
			Call:         call,
			Grid:         sqlNullString(grid),
			Observations: 1,
			FirstSeen:    now,
			UpdatedAt:    now,
		}
		select {
		case updates <- rec:
		default:
			// Drop silently to avoid backpressure on the spot pipeline.
		}
	}

	// Purpose: Enqueue CTY metadata updates without blocking ingest.
	// Key aspects: Stores CTY fields only; relies on gridstore merge for retention.
	// Upstream: ingest validation cache misses.
	// Downstream: updates channel.
	ctyUpdateFn := func(call string, info *cty.PrefixInfo) {
		if info == nil {
			return
		}
		call = normalizeCallForMetadata(call)
		if call == "" {
			return
		}
		now := time.Now().UTC()
		rec := gridstore.Record{
			Call:         call,
			CTYValid:     true,
			CTYADIF:      info.ADIF,
			CTYCQZone:    info.CQZone,
			CTYITUZone:   info.ITUZone,
			CTYContinent: info.Continent,
			CTYCountry:   info.Country,
			FirstSeen:    now,
			UpdatedAt:    now,
		}
		select {
		case updates <- rec:
		default:
			// Drop silently to avoid backpressure on the ingest pipeline.
		}
	}

	// Purpose: Stop the grid writer and optional async lookup goroutine.
	// Key aspects: Closes channels and waits for clean shutdown.
	// Upstream: main shutdown path.
	// Downstream: channel close and done waits.
	stopFn := func() {
		close(updates)
		<-done
		if asyncLookupEnabled {
			close(lookupQueue)
			<-lookupDone
		}
		if syncLookupEnabled {
			close(syncLookupQueue)
			syncLookupWG.Wait()
		}
	}

	// Purpose: Lookup a grid entry with optional async backfill on cache miss.
	// Key aspects: Avoids synchronous DB reads on the output pipeline.
	// Upstream: processOutputSpots gridLookup hook.
	// Downstream: cache.LookupGrid and lookupQueue enqueue.
	lookupFn := func(call string) (string, bool) {
		rawCall := strings.ToUpper(strings.TrimSpace(call))
		baseCall := normalizeCallForMetadata(rawCall)
		if baseCall == "" {
			return "", false
		}
		if cache != nil {
			if grid, ok := cache.LookupGrid(baseCall, metrics); ok {
				return grid, true
			}
		}
		// Cache miss: enqueue async lookup to avoid blocking output.
		if asyncLookupEnabled {
			lookupPendingMu.Lock()
			if _, exists := lookupPending[baseCall]; !exists {
				lookupPending[baseCall] = struct{}{}
				select {
				case lookupQueue <- gridLookupRequest{baseCall: baseCall, rawCall: rawCall}:
				default:
					delete(lookupPending, baseCall)
				}
			}
			lookupPendingMu.Unlock()
		}
		return "", false
	}

	if asyncLookupEnabled {
		// Purpose: Background cache backfill for grid lookups.
		// Key aspects: Reads from lookupQueue and populates cache entries.
		// Upstream: lookupFn enqueue path.
		// Downstream: store.Get and cache.ApplyRecord.
		go func() {
			defer close(lookupDone)
			for req := range lookupQueue {
				rec, err := lookupRecord(req.baseCall, req.rawCall)
				if err == nil && rec != nil {
					if cache != nil {
						cache.ApplyRecord(*rec)
					}
				} else if err != nil && !gridstore.IsBusyError(err) {
					log.Printf("Warning: gridstore async lookup failed for %s: %v", req.baseCall, err)
				}
				lookupPendingMu.Lock()
				delete(lookupPending, req.baseCall)
				lookupPendingMu.Unlock()
			}
		}()
	} else {
		close(lookupDone)
	}

	resolveGrid := func(rec *gridstore.Record) (string, bool) {
		if rec == nil || !rec.Grid.Valid {
			return "", false
		}
		grid := strings.TrimSpace(strings.ToUpper(rec.Grid.String))
		if grid == "" {
			return "", false
		}
		return grid, true
	}

	if syncLookupEnabled {
		// Purpose: Bounded synchronous backfill worker pool for tight-timeout lookups.
		// Key aspects: Uses a fixed worker count and buffered queue to cap concurrency.
		// Upstream: gridLookupSync enqueue path.
		// Downstream: store.Get and cache.ApplyRecord.
		for i := 0; i < gridSyncLookupWorkers; i++ {
			syncLookupWG.Add(1)
			go func() {
				defer syncLookupWG.Done()
				for req := range syncLookupQueue {
					rec, err := lookupRecord(req.baseCall, req.rawCall)
					if err == nil && rec != nil {
						if cache != nil {
							cache.ApplyRecord(*rec)
						}
						if req.resp != nil {
							grid, ok := resolveGrid(rec)
							req.resp <- gridLookupResult{grid: grid, ok: ok}
						}
						continue
					}
					if err != nil && !gridstore.IsBusyError(err) {
						log.Printf("Warning: gridstore sync lookup failed for %s: %v", req.baseCall, err)
					}
					if req.resp != nil {
						req.resp <- gridLookupResult{}
					}
				}
			}()
		}
	}

	var lookupSyncFn func(call string) (string, bool)
	if syncLookupEnabled {
		// Purpose: Provide a tight-timeout synchronous lookup path for first-spot grids.
		// Key aspects: Bounds enqueue time and total wait time; falls back to miss on timeout.
		// Upstream: processOutputSpots grid backfill for RBN.
		// Downstream: syncLookupQueue and cache.ApplyRecord.
		lookupSyncFn = func(call string) (string, bool) {
			rawCall := strings.ToUpper(strings.TrimSpace(call))
			baseCall := normalizeCallForMetadata(rawCall)
			if baseCall == "" {
				return "", false
			}
			if cache != nil {
				if grid, ok := cache.LookupGrid(baseCall, metrics); ok {
					return grid, true
				}
			}
			resp := make(chan gridLookupResult, 1)
			req := gridLookupRequest{baseCall: baseCall, rawCall: rawCall, resp: resp}
			deadline := time.Now().Add(gridSyncLookupTimeout)
			timer := time.NewTimer(gridSyncLookupTimeout)
			defer timer.Stop()

			select {
			case syncLookupQueue <- req:
			case <-timer.C:
				return "", false
			}

			remaining := time.Until(deadline)
			if remaining <= 0 {
				return "", false
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(remaining)

			select {
			case res := <-resp:
				return res.grid, res.ok
			case <-timer.C:
				return "", false
			}
		}
	}

	return gridUpdateFn, ctyUpdateFn, metrics, stopFn, lookupFn, lookupSyncFn
}

// Purpose: Load FCC ULS database stats for dashboard display.
// Key aspects: Opens SQLite, counts tables, and records file metadata.
// Upstream: displayStatsWithFCC.
// Downstream: sql.Open, db.QueryRow, os.Stat.
// loadFCCSnapshot opens the FCC ULS database to report simple stats for the dashboard.
func loadFCCSnapshot(path string) *fccSnapshot {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	db, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		log.Printf("Warning: FCC ULS open failed: %v", err)
		return nil
	}
	defer db.Close()

	count := func(table string) int64 {
		var c int64
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&c); err != nil {
			log.Printf("Warning: FCC ULS count %s failed: %v", table, err)
			return 0
		}
		return c
	}

	snap := &fccSnapshot{
		HDCount:   count("HD"),
		AMCount:   count("AM"),
		DBSize:    info.Size(),
		UpdatedAt: info.ModTime(),
		Path:      path,
	}
	return snap
}

// Purpose: Convert a string into sql.NullString.
// Key aspects: Returns invalid when the input is empty.
// Upstream: startGridWriter batch creation.
// Downstream: sql.NullString initialization.
func sqlNullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

// formatMemoryLine reports memory-ish metrics in order:
// exec alloc / ring buffer / primary dedup (dup%) / secondary dedup (fast+slow) / call meta cache (hit%) / known calls (hit%).
// Purpose: Format the memory/status line for the stats pane.
// Key aspects: Reports ring buffer occupancy and cache hit stats.
// Upstream: displayStatsWithFCC.
// Downstream: buffer.RingBuffer stats and cache lookups.
func formatMemoryLine(buf *buffer.RingBuffer, dedup *dedup.Deduplicator, secondaryFast *dedup.SecondaryDeduper, secondarySlow *dedup.SecondaryDeduper, metaCache *callMetaCache, knownPtr *atomic.Pointer[spot.KnownCallsigns]) string {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	execMB := bytesToMB(mem.Alloc)

	ringMB := 0.0
	if buf != nil {
		ringMB = float64(buf.GetSizeKB()) / 1024.0
	}

	dedupeMB := 0.0
	dedupeRatio := 0.0
	secondaryMB := 0.0
	if dedup != nil {
		processed, duplicates, cacheSize := dedup.GetStats()
		dedupeMB = bytesToMB(uint64(cacheSize * dedupeEntryBytes))
		if processed > 0 {
			dedupeRatio = float64(duplicates) / float64(processed) * 100
		}
	}
	if secondaryFast != nil {
		_, _, cacheSize := secondaryFast.GetStats()
		secondaryMB += bytesToMB(uint64(cacheSize * dedupeEntryBytes))
	}
	if secondarySlow != nil {
		_, _, cacheSize := secondarySlow.GetStats()
		secondaryMB += bytesToMB(uint64(cacheSize * dedupeEntryBytes))
	}

	metaMB := 0.0
	metaRatio := 0.0
	if metaCache != nil {
		entries := metaCache.EntryCount()
		metaMB = bytesToMB(uint64(entries * callMetaEntryBytes))
		metrics := metaCache.CTYMetrics()
		if metrics.Lookups > 0 {
			metaRatio = float64(metrics.Hits) / float64(metrics.Lookups) * 100
		}
	}

	knownMB := 0.0
	knownRatio := 0.0
	var known *spot.KnownCallsigns
	if knownPtr != nil {
		known = knownPtr.Load()
	}
	if known != nil {
		knownMB = bytesToMB(uint64(known.Count() * knownCallEntryBytes))
		lookups, hits := known.StatsDX()
		if lookups > 0 {
			knownRatio = float64(hits) / float64(lookups) * 100
		}
	}

	return fmt.Sprintf("Memory MB: %.1f / %.1f / %.1f (%.1f%%) / %.1f / %.1f (%.1f%%) / %.1f (%.1f%%)",
		execMB, ringMB, dedupeMB, dedupeRatio, secondaryMB, metaMB, metaRatio, knownMB, knownRatio)
}

// Purpose: Format a human-readable uptime line.
// Key aspects: Uses days/hours/minutes formatting.
// Upstream: displayStatsWithFCC.
// Downstream: time.Duration math.
func formatUptimeLine(uptime time.Duration) string {
	hours := int(uptime.Hours())
	minutes := int(uptime.Minutes()) % 60
	return fmt.Sprintf("Uptime: %02d:%02d", hours, minutes)
}

// Purpose: Format a short duration for stats display.
// Key aspects: Uses d/h/m/s units with coarse granularity.
// Upstream: CTY stats line formatting.
// Downstream: time.Duration math.
func formatDurationShort(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	minutes := int(d / time.Minute)
	d -= time.Duration(minutes) * time.Minute
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	seconds := int(d / time.Second)
	return fmt.Sprintf("%ds", seconds)
}

// wwvKindFromLine tags non-DX lines coming from human/relay telnet ingest.
// We only forward WWV/WCY bulletins to telnet clients; upstream keepalives,
// prompts, or other control chatter (e.g., "de N2WQ-22" banners) are dropped.
// Purpose: Extract WWV/WCY bulletin kind token from a raw line.
// Key aspects: Uppercases and trims for display selection.
// Upstream: WWV handling in peer/telnet paths.
// Downstream: strings.TrimSpace/ToUpper.
func wwvKindFromLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, "WWV") {
		return "WWV"
	}
	if strings.HasPrefix(upper, "WCY") {
		return "WCY"
	}
	return ""
}

// announcementFromLine returns the raw announcement text for "To ALL" broadcasts.
// Purpose: Extract announcement text from a PC93 line.
// Key aspects: Strips known prefix and trims whitespace.
// Upstream: PC93 announcement parsing.
// Downstream: strings.TrimSpace.
func announcementFromLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, "TO ALL") {
		return trimmed
	}
	return ""
}

// Purpose: Compute per-interval delta for a counter map entry.
// Key aspects: Updates previous map in-place.
// Upstream: displayStatsWithFCC.
// Downstream: map access/mutation.
func diffCounter(current, previous map[string]uint64, key string) uint64 {
	if current == nil {
		current = map[string]uint64{}
	}
	if previous == nil {
		previous = map[string]uint64{}
	}
	key = strings.ToUpper(strings.TrimSpace(key))
	cur := current[key]
	prev := previous[key]
	if cur >= prev {
		return cur - prev
	}
	return cur
}

// Purpose: Compute per-interval delta for a source+mode counter.
// Key aspects: Uses sourceModeKey to access map keys.
// Upstream: displayStatsWithFCC.
// Downstream: sourceModeKey and map mutation.
func diffSourceMode(current, previous map[string]uint64, source, mode string) uint64 {
	key := sourceModeKey(source, mode)
	return diffCounter(current, previous, key)
}

// Purpose: Build a stable key for source+mode counters.
// Key aspects: Uppercases and concatenates with a delimiter.
// Upstream: diffSourceMode.
// Downstream: strings.ToUpper.
func sourceModeKey(source, mode string) string {
	source = strings.ToUpper(strings.TrimSpace(source))
	mode = strings.ToUpper(strings.TrimSpace(mode))
	if source == "" || mode == "" {
		return ""
	}
	return source + sourceModeDelimiter + mode
}

// Purpose: Convert bytes to megabytes (MB).
// Key aspects: Uses base-10 MB.
// Upstream: formatMemoryLine.
// Downstream: float math.
func bytesToMB(b uint64) float64 {
	return float64(b) / (1024.0 * 1024.0)
}

// maybeStartHeapLogger starts periodic heap logging when DXC_HEAP_LOG_INTERVAL is set
// (e.g., "60s"). Defaults to disabled when the variable is empty or invalid.
// Purpose: Optionally start a periodic heap profile logger.
// Key aspects: Controlled by environment variables.
// Upstream: main startup.
// Downstream: pprof.WriteHeapProfile and time.NewTicker.
func maybeStartHeapLogger() {
	intervalStr := strings.TrimSpace(os.Getenv("DXC_HEAP_LOG_INTERVAL"))
	if intervalStr == "" {
		return
	}
	interval, err := time.ParseDuration(intervalStr)
	if err != nil || interval <= 0 {
		log.Printf("Heap logger disabled (invalid DXC_HEAP_LOG_INTERVAL=%q)", intervalStr)
		return
	}
	ticker := time.NewTicker(interval)
	// Purpose: Emit periodic heap stats to the log.
	// Key aspects: Runs on ticker cadence until process exit.
	// Upstream: maybeStartHeapLogger.
	// Downstream: runtime.ReadMemStats and log.Printf.
	go func() {
		log.Printf("Heap logger enabled (every %s)", interval)
		for range ticker.C {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			log.Printf("Heap: alloc=%.1f MB sys=%.1f MB objects=%d gc=%d next_gc=%.1f MB",
				bytesToMB(m.HeapAlloc),
				bytesToMB(m.Sys),
				m.HeapObjects,
				m.NumGC,
				bytesToMB(m.NextGC))
		}
	}()
}

// maybeStartDiagServer exposes /debug/pprof/* and /debug/heapdump when DXC_PPROF_ADDR is set
// (example: DXC_PPROF_ADDR=localhost:6061). Default is off.
// Purpose: Optionally start the pprof/diagnostic HTTP server.
// Key aspects: Reads env vars and starts http server in background.
// Upstream: main startup.
// Downstream: http.ListenAndServe and net/http/pprof.
func maybeStartDiagServer() {
	addr := strings.TrimSpace(os.Getenv("DXC_PPROF_ADDR"))
	if addr == "" {
		return
	}
	mux := http.NewServeMux()
	// Purpose: Serve a heap dump endpoint that writes a pprof file to disk.
	// Key aspects: Creates diagnostics dir, forces GC, and writes heap profile.
	// Upstream: HTTP /debug/heapdump request.
	// Downstream: os.MkdirAll, os.Create, pprof.WriteHeapProfile.
	mux.HandleFunc("/debug/heapdump", func(w http.ResponseWriter, r *http.Request) {
		ts := time.Now().UTC().Format("2006-01-02T15-04-05Z")
		dir := filepath.Join("data", "diagnostics")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			http.Error(w, fmt.Sprintf("mkdir diagnostics: %v", err), http.StatusInternalServerError)
			return
		}
		path := filepath.Join(dir, fmt.Sprintf("heap-%s.pprof", ts))
		f, err := os.Create(path)
		if err != nil {
			http.Error(w, fmt.Sprintf("create heap dump: %v", err), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		runtime.GC() // collect latest data
		if err := pprof.WriteHeapProfile(f); err != nil {
			http.Error(w, fmt.Sprintf("write heap profile: %v", err), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "heap profile written to %s\n", path)
	})
	mux.Handle("/debug/pprof/", http.HandlerFunc(httppprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(httppprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(httppprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(httppprof.Symbol))
	mux.Handle("/debug/pprof/trace", http.HandlerFunc(httppprof.Trace))

	// Purpose: Run the diagnostics HTTP server.
	// Key aspects: Logs startup and reports server errors.
	// Upstream: maybeStartDiagServer.
	// Downstream: http.ListenAndServe.
	go func() {
		log.Printf("Diagnostics server listening on %s (pprof + /debug/heapdump)", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("Diagnostics server error: %v", err)
		}
	}()
}
