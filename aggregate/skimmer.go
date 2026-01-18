// Package aggregate provides DXSpider-style broadcast-only deduping for skimmer spots.
package aggregate

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"dxcluster/config"
	"dxcluster/spot"
)

// SkimmerAggregator collapses near-identical skimmer spots into a single
// consolidated spot after a dwell/limbo window. It is broadcast-only and never
// mutates the canonical spot stored in buffers or archives.
//
// Concurrency contract:
// - Only the aggregator goroutine mutates internal state.
// - Enqueue is non-blocking and drops when the input buffer is full.
// - Output is delivered on a bounded channel; drops are counted when full.
type SkimmerAggregator struct {
	cfg    config.SkimmerAggregateConfig
	now    func() time.Time
	input  chan *spot.Spot
	output chan *spot.Spot

	startedAt time.Time
	state     aggregateState
	stats     aggregateStats
	emitFn    func(*spot.Spot)
}

type aggregateState struct {
	buckets  map[string]*aggregateBucket
	queue    map[string]struct{}
	skimmers map[string]*skimmerScore
}

type aggregateBucket struct {
	createdAt   time.Time
	lastEmitAt  time.Time
	lastQuality int
	records     []aggregateRecord
}

type aggregateRecord struct {
	spot    *spot.Spot
	origin  string
	freqKHz float64
	report  int
	hasRpt  bool
	when    time.Time
	respot  bool
}

type skimmerScore struct {
	score  int
	good   int
	bad    int
	lastIn time.Time
}

type aggregateStats struct {
	enqueued     atomic.Uint64
	inputDrops   atomic.Uint64
	outputDrops  atomic.Uint64
	emitted      atomic.Uint64
	limboDrops   atomic.Uint64
	respotDrops  atomic.Uint64
	recordsDrops atomic.Uint64
	inrushDrops  atomic.Uint64
}

// Purpose: Construct a skimmer aggregator with default time source.
// Key aspects: Initializes bounded queues and state maps.
// Upstream: main startup when skimmer aggregation is enabled.
// Downstream: Start, Enqueue, Output.
func NewSkimmerAggregator(cfg config.SkimmerAggregateConfig) *SkimmerAggregator {
	return newSkimmerAggregator(cfg, time.Now)
}

func newSkimmerAggregator(cfg config.SkimmerAggregateConfig, now func() time.Time) *SkimmerAggregator {
	return &SkimmerAggregator{
		cfg:       cfg,
		now:       now,
		input:     make(chan *spot.Spot, cfg.InputBuffer),
		output:    make(chan *spot.Spot, cfg.OutputBuffer),
		startedAt: now(),
		state: aggregateState{
			buckets:  make(map[string]*aggregateBucket),
			queue:    make(map[string]struct{}),
			skimmers: make(map[string]*skimmerScore),
		},
	}
}

// Purpose: Start the aggregator loop.
// Key aspects: Uses a ticker to flush dwell/limbo buckets; exits on context cancel.
// Upstream: main startup.
// Downstream: internal processQueue and emit.
func (a *SkimmerAggregator) Start(ctx context.Context) {
	if a == nil {
		return
	}
	ticker := time.NewTicker(a.tickInterval())
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				a.flush(a.now())
				close(a.output)
				return
			case s := <-a.input:
				if s == nil {
					continue
				}
				a.ingest(s, a.now())
			case <-ticker.C:
				a.processQueue(a.now())
			}
		}
	}()
}

// Purpose: Return the input queue for non-blocking enqueue.
// Key aspects: Caller should prefer Enqueue to track drops.
// Upstream: processOutputSpots.
// Downstream: aggregator loop.
func (a *SkimmerAggregator) Input() chan<- *spot.Spot {
	if a == nil {
		return nil
	}
	return a.input
}

// Purpose: Return the aggregated output stream.
// Key aspects: Bounded channel; outputs may be dropped under pressure.
// Upstream: broadcast fan-out.
// Downstream: telnet broadcast.
func (a *SkimmerAggregator) Output() <-chan *spot.Spot {
	if a == nil {
		return nil
	}
	return a.output
}

// Purpose: Enqueue a spot for aggregation without blocking.
// Key aspects: Drops on full queue and increments counters.
// Upstream: output processor.
// Downstream: aggregator loop.
func (a *SkimmerAggregator) Enqueue(s *spot.Spot) bool {
	if a == nil || s == nil {
		return true
	}
	select {
	case a.input <- s:
		a.stats.enqueued.Add(1)
		return true
	default:
		a.stats.inputDrops.Add(1)
		return false
	}
}

// Purpose: Report aggregate counters for observability.
// Key aspects: Reads atomics; no locking.
// Upstream: stats ticker.
// Downstream: displayStatsWithFCC.
func (a *SkimmerAggregator) Stats() (enqueued, emitted, inputDrops, outputDrops, limboDrops, respotDrops, recordDrops, inrushDrops uint64) {
	if a == nil {
		return 0, 0, 0, 0, 0, 0, 0, 0
	}
	return a.stats.enqueued.Load(),
		a.stats.emitted.Load(),
		a.stats.inputDrops.Load(),
		a.stats.outputDrops.Load(),
		a.stats.limboDrops.Load(),
		a.stats.respotDrops.Load(),
		a.stats.recordsDrops.Load(),
		a.stats.inrushDrops.Load()
}

// Purpose: Decide if a spot should be aggregated.
// Key aspects: Aggregates skimmer sources only; human/manual sources pass through.
// Upstream: processOutputSpots.
// Downstream: Enqueue.
func (a *SkimmerAggregator) ShouldAggregate(s *spot.Spot) bool {
	if a == nil || s == nil {
		return false
	}
	if !a.cfg.Enabled {
		return false
	}
	if s.IsHuman {
		return false
	}
	switch s.SourceType {
	case spot.SourceRBN, spot.SourceFT8, spot.SourceFT4, spot.SourcePSKReporter:
		return true
	default:
		return false
	}
}

func (a *SkimmerAggregator) tickInterval() time.Duration {
	dwell := time.Duration(a.cfg.DwellSeconds) * time.Second
	if dwell <= 0 {
		return time.Second
	}
	if dwell > 2*time.Second {
		return time.Second
	}
	if dwell/2 < 200*time.Millisecond {
		return 200 * time.Millisecond
	}
	return dwell / 2
}

func (a *SkimmerAggregator) ingest(s *spot.Spot, now time.Time) {
	if a.cfg.InrushDelaySeconds > 0 {
		if now.Sub(a.startedAt) < time.Duration(a.cfg.InrushDelaySeconds)*time.Second {
			a.stats.inrushDrops.Add(1)
			return
		}
	}
	dxCall := strings.TrimSpace(s.DXCallNorm)
	if dxCall == "" {
		dxCall = strings.TrimSpace(s.DXCall)
	}
	if dxCall == "" {
		return
	}
	nqrg := int(math.Round(roundTo100Hz(s.Frequency) * 10))
	key, bucket := a.findBucket(dxCall, nqrg)
	if bucket != nil && len(bucket.records) == 0 {
		if now.Sub(bucket.lastEmitAt) < time.Duration(a.cfg.RespotSeconds)*time.Second {
			a.stats.respotDrops.Add(1)
			return
		}
		bucket.createdAt = now
	}
	if bucket == nil {
		if len(a.state.buckets) >= a.cfg.MaxEntries {
			a.stats.recordsDrops.Add(1)
			return
		}
		bucket = &aggregateBucket{createdAt: now}
		a.state.buckets[key] = bucket
	}
	if len(bucket.records) >= a.cfg.MaxRecordsPerKey {
		a.stats.recordsDrops.Add(1)
		return
	}
	origin := a.originForRecord(s)
	when := s.Time
	if when.IsZero() {
		when = now
	}
	record := aggregateRecord{
		spot:    s,
		origin:  origin,
		freqKHz: roundTo100Hz(s.Frequency),
		report:  s.Report,
		hasRpt:  s.HasReport,
		when:    when,
		respot:  bucket.lastEmitAt.After(time.Time{}),
	}
	bucket.records = append(bucket.records, record)
	a.state.queue[key] = struct{}{}
}

func (a *SkimmerAggregator) processQueue(now time.Time) {
	for key := range a.state.queue {
		bucket := a.state.buckets[key]
		if bucket == nil {
			delete(a.state.queue, key)
			continue
		}
		if len(bucket.records) == 0 {
			if bucket.lastEmitAt.IsZero() || now.Sub(bucket.lastEmitAt) > time.Duration(a.cfg.CacheSeconds)*time.Second {
				delete(a.state.queue, key)
				delete(a.state.buckets, key)
			}
			continue
		}
		records := bucket.records
		quality, uniqueRecords := collapseByOrigin(records)
		dwell := now.Sub(bucket.createdAt)
		if quality < a.cfg.MaxQuality && dwell < time.Duration(a.cfg.DwellSeconds)*time.Second && dwell < time.Duration(a.cfg.LimboSeconds)*time.Second {
			continue
		}
		if dwell >= time.Duration(a.cfg.LimboSeconds)*time.Second && quality < a.cfg.MinQuality {
			delete(a.state.queue, key)
			delete(a.state.buckets, key)
			a.stats.limboDrops.Add(1)
			continue
		}
		if quality < a.cfg.MinQuality {
			continue
		}
		if quality > a.cfg.MaxQuality {
			quality = a.cfg.MaxQuality
		}
		bucket.lastQuality = max(bucket.lastQuality, quality)
		consensusFreq, originsSeen := a.consensusFrequency(uniqueRecords, now)
		best := pickBestRecord(uniqueRecords)
		qualityTag := fmt.Sprintf("Q:%d", quality)
		if originsSeen > 1 {
			qualityTag += "*"
		}
		if anyRespot(uniqueRecords) {
			qualityTag += "+"
		}
		aggregateSpot := cloneForAggregate(best, consensusFreq, qualityTag)
		a.emit(aggregateSpot)
		bucket.records = nil
		bucket.lastEmitAt = now

		delete(a.state.queue, key)
		dxCall := strings.TrimSpace(best.spot.DXCallNorm)
		if dxCall == "" {
			dxCall = strings.TrimSpace(best.spot.DXCall)
		}
		consensusKey := aggregateKey(dxCall, int(math.Round(consensusFreq*10)))
		if consensusKey != "" && consensusKey != key {
			delete(a.state.buckets, key)
			a.state.buckets[consensusKey] = bucket
		}
	}

	// Purge stale skimmer scores.
	for k, score := range a.state.skimmers {
		if !score.lastIn.IsZero() && now.Sub(score.lastIn) > time.Duration(a.cfg.CacheSeconds)*time.Second {
			delete(a.state.skimmers, k)
		}
	}
}

func (a *SkimmerAggregator) flush(now time.Time) {
	a.processQueue(now)
}

func (a *SkimmerAggregator) emit(s *spot.Spot) {
	if s == nil {
		return
	}
	if a.emitFn != nil {
		a.emitFn(s)
		a.stats.emitted.Add(1)
		return
	}
	select {
	case a.output <- s:
		a.stats.emitted.Add(1)
	default:
		a.stats.outputDrops.Add(1)
	}
}

func (a *SkimmerAggregator) findBucket(call string, nqrg int) (string, *aggregateBucket) {
	key := aggregateKey(call, nqrg)
	if bucket, ok := a.state.buckets[key]; ok {
		return key, bucket
	}
	searchUnits := a.cfg.SearchKHz * 10
	for delta := 1; delta <= searchUnits; delta++ {
		upKey := aggregateKey(call, nqrg+delta)
		if bucket, ok := a.state.buckets[upKey]; ok {
			return upKey, bucket
		}
		downKey := aggregateKey(call, nqrg-delta)
		if bucket, ok := a.state.buckets[downKey]; ok {
			return downKey, bucket
		}
	}
	return key, nil
}

func (a *SkimmerAggregator) originForRecord(s *spot.Spot) string {
	origin := s.DECallNorm
	if origin == "" {
		origin = s.DECall
	}
	if a.cfg.CollapseSSID {
		return spot.CollapseSSID(origin)
	}
	return origin
}

func (a *SkimmerAggregator) consensusFrequency(records []aggregateRecord, now time.Time) (float64, int) {
	votes := make(map[float64]float64, len(records))
	origins := make(map[string]struct{})
	for _, rec := range records {
		origins[rec.origin] = struct{}{}
		key := a.skimmerKey(rec)
		score := a.state.skimmers[key]
		if score == nil {
			score = &skimmerScore{score: 1, good: 1}
			a.state.skimmers[key] = score
		}
		weight := float64(score.score)
		if weight < 0.1 {
			weight = 0.1
		}
		votes[rec.freqKHz] += weight
	}
	consensus := 0.0
	maxVote := -1.0
	for freq, vote := range votes {
		if vote > maxVote || (vote == maxVote && freq < consensus) {
			consensus = freq
			maxVote = vote
		}
	}

	for _, rec := range records {
		diff := roundTo100Hz(rec.freqKHz - consensus)
		score := a.state.skimmers[a.skimmerKey(rec)]
		if score == nil {
			score = &skimmerScore{score: 1, good: 1}
			a.state.skimmers[a.skimmerKey(rec)] = score
		}
		if diff != 0 {
			score.bad = min(score.bad+1, a.cfg.MaxDeviants)
			score.good = max(score.good-1, 0)
		} else {
			score.good = min(score.good+1, a.cfg.MaxDeviants)
			score.bad = max(score.bad-1, 0)
		}
		score.score = score.good - score.bad
		score.lastIn = now
	}
	return consensus, len(origins)
}

func (a *SkimmerAggregator) skimmerKey(rec aggregateRecord) string {
	band := rec.spot.BandNorm
	if band == "" {
		band = rec.spot.Band
	}
	if band == "" {
		band = spot.FreqToBand(rec.freqKHz)
	}
	return rec.origin + "|" + band
}

func aggregateKey(call string, nqrg int) string {
	call = strings.TrimSpace(call)
	if call == "" {
		return ""
	}
	return call + "|" + strconv.Itoa(nqrg)
}

func collapseByOrigin(records []aggregateRecord) (int, []aggregateRecord) {
	seen := make(map[string]struct{}, len(records))
	out := make([]aggregateRecord, 0, len(records))
	for _, rec := range records {
		if _, ok := seen[rec.origin]; ok {
			continue
		}
		seen[rec.origin] = struct{}{}
		out = append(out, rec)
	}
	return len(out), out
}

func pickBestRecord(records []aggregateRecord) aggregateRecord {
	best := records[0]
	for _, rec := range records[1:] {
		if rec.hasRpt && !best.hasRpt {
			best = rec
			continue
		}
		if rec.hasRpt && best.hasRpt {
			if rec.report > best.report {
				best = rec
				continue
			}
			if rec.report == best.report && rec.when.After(best.when) {
				best = rec
				continue
			}
		}
		if !rec.hasRpt && !best.hasRpt && rec.when.After(best.when) {
			best = rec
		}
	}
	return best
}

func anyRespot(records []aggregateRecord) bool {
	for _, rec := range records {
		if rec.respot {
			return true
		}
	}
	return false
}

func cloneForAggregate(best aggregateRecord, consensusFreq float64, qualityTag string) *spot.Spot {
	src := best.spot
	if src == nil {
		return nil
	}
	comment := strings.TrimSpace(src.Comment)
	if comment == "" {
		comment = qualityTag
	} else {
		comment = comment + " " + qualityTag
	}
	clone := &spot.Spot{
		ID:                 src.ID,
		DXCall:             src.DXCall,
		DECall:             src.DECall,
		Frequency:          roundTo100Hz(consensusFreq),
		Band:               src.Band,
		Mode:               src.Mode,
		Report:             best.report,
		HasReport:          best.hasRpt,
		Time:               src.Time,
		Comment:            comment,
		SourceType:         src.SourceType,
		SourceNode:         src.SourceNode,
		SpotterIP:          src.SpotterIP,
		TTL:                src.TTL,
		IsHuman:            src.IsHuman,
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
		DXCellID:           src.DXCellID,
		DECellID:           src.DECellID,
		DECallStripped:     src.DECallStripped,
		DECallNormStripped: src.DECallNormStripped,
	}
	clone.EnsureNormalized()
	clone.RefreshBeaconFlag()
	return clone
}

func roundTo100Hz(freqKHz float64) float64 {
	return math.Floor(freqKHz*10+0.5) / 10
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
