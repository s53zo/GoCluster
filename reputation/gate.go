package reputation

import (
	"context"
	"errors"
	"log"
	"net/netip"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dxcluster/config"
	"dxcluster/cty"
	"dxcluster/spot"
)

// Gate enforces the passwordless telnet reputation policy.
type Gate struct {
	cfg          gateConfig
	ctyLookup    func() *cty.CTYDatabase
	store        *Store
	ipinfoIndex  atomic.Pointer[ipinfoIndex]
	ipinfoPath   string
	lookupCache  *ttlCache
	cymru        *cymruResolver
	callShards   []callShard
	prefixV4     *prefixLimiter
	prefixV6     *prefixLimiter
	sweeperEvery time.Duration
}

type gateConfig struct {
	enabled                  bool
	initialWait              time.Duration
	rampWindow               time.Duration
	perBandStart             int
	perBandCap               int
	totalCapStart            int
	totalCapPostRamp         int
	totalCapRampDelay        time.Duration
	countryMismatchExtraWait time.Duration
	disagreementPenalty      time.Duration
	unknownPenalty           time.Duration
	disagreementResetOnNew   bool
	resetOnNewASN            bool
	countryFlipScope         string
	maxASNHistory            int
	maxCountryHistory        int
	stateTTL                 time.Duration
	stateMaxEntries          int
	prefixTTL                time.Duration
	prefixMaxEntries         int
	ipinfoSnapshotMaxAge     time.Duration
	ipinfoSnapshotPath       string
	ipinfoDownloadEnabled    bool
	ipinfoDownloadToken      string
	ipinfoDownloadURL        string
	ipinfoDownloadPath       string
	ipinfoRefreshUTC         string
	ipinfoDownloadTimeout    time.Duration
	cymruEnabled             bool
	cymruLookupTimeout       time.Duration
	cymruCacheTTL            time.Duration
	cymruNegativeTTL         time.Duration
	cymruWorkers             int
	lookupCacheTTL           time.Duration
	lookupCacheMaxEntries    int
	ipv4BucketSize           int
	ipv4BucketRefillPerSec   int
	ipv6BucketSize           int
	ipv6BucketRefillPerSec   int
}

type callShard struct {
	mu    sync.Mutex
	calls map[string]*callState
}

type callState struct {
	windowStart        time.Time
	nextAllowedAt      time.Time
	nextRampAt         time.Time
	totalCapUpgradeAt  time.Time
	perBandLimit       int
	totalCap           int
	perBandCounts      map[string]int
	totalCount         int
	lastSeen           time.Time
	penaltyFlags       PenaltyFlags
	ctyCountryKey      string
	ctyContinentKey    string
	currentASN         string
	currentCountryKey  string
	currentCountryCode string
	currentCountry     string
	currentSource      string
	currentPrefix      string
	asnHistory         []string
	countryHistory     []string
	continentHistory   []string
}

// NewGate builds the telnet reputation gate with normalized config defaults.
func NewGate(cfg config.ReputationConfig, ctyLookup func() *cty.CTYDatabase) (*Gate, error) {
	gcfg := normalizeGateConfig(cfg)
	if !gcfg.enabled {
		return &Gate{cfg: gcfg}, nil
	}
	store := NewStore(cfg.ReputationDir, gcfg.maxASNHistory, gcfg.maxCountryHistory)
	lookupCache := newTTLCache(32, gcfg.lookupCacheTTL, 5*time.Minute, gcfg.lookupCacheMaxEntries)
	cymru := newCymruResolver(gcfg.cymruEnabled, gcfg.cymruLookupTimeout, gcfg.cymruCacheTTL, gcfg.cymruNegativeTTL, gcfg.lookupCacheMaxEntries, gcfg.cymruWorkers)

	g := &Gate{
		cfg:          gcfg,
		ctyLookup:    ctyLookup,
		store:        store,
		ipinfoPath:   gcfg.ipinfoSnapshotPath,
		lookupCache:  lookupCache,
		cymru:        cymru,
		callShards:   make([]callShard, 32),
		prefixV4:     newPrefixLimiter(gcfg.ipv4BucketSize, gcfg.ipv4BucketRefillPerSec, gcfg.prefixTTL, gcfg.prefixMaxEntries),
		prefixV6:     newPrefixLimiter(gcfg.ipv6BucketSize, gcfg.ipv6BucketRefillPerSec, gcfg.prefixTTL, gcfg.prefixMaxEntries),
		sweeperEvery: time.Minute,
	}
	for i := range g.callShards {
		g.callShards[i].calls = make(map[string]*callState)
	}
	if g.cfg.stateTTL > 0 && g.cfg.stateTTL/2 < g.sweeperEvery {
		g.sweeperEvery = g.cfg.stateTTL / 2
	}
	return g, nil
}

// Start launches background workers (DNS fallback, sweeper, optional downloader).
func (g *Gate) Start(ctx context.Context) {
	if g == nil || !g.cfg.enabled {
		return
	}
	if err := g.LoadSnapshot(); err != nil {
		log.Printf("Warning: failed to load ipinfo snapshot: %v", err)
	}
	if g.cymru != nil {
		g.cymru.start(ctx)
	}
	go g.sweeper(ctx)
	if g.cfg.ipinfoDownloadEnabled {
		go g.downloadLoop(ctx)
	}
}

// LoadSnapshot refreshes the in-memory IPinfo index.
func (g *Gate) LoadSnapshot() error {
	if g == nil || !g.cfg.enabled {
		return nil
	}
	path := strings.TrimSpace(g.ipinfoPath)
	if path == "" {
		return errors.New("ipinfo snapshot path is empty")
	}
	idx, err := LoadIPInfoSnapshot(path)
	if err != nil {
		return err
	}
	g.ipinfoIndex.Store(idx)
	return nil
}

// RecordLogin seeds or refreshes the per-call reputation state.
func (g *Gate) RecordLogin(call, ip string, now time.Time) {
	if g == nil || !g.cfg.enabled {
		return
	}
	call = spot.NormalizeCallsign(call)
	if call == "" {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	addr, _ := netip.ParseAddr(strings.TrimSpace(ip))

	ctyCountryKey, ctyContinentKey := g.lookupCTY(call)
	ipResult, cymruResult, ipKey, ipContinent, ipKnown := g.lookupIP(addr, now)
	primary := ipResult
	if primary.Source == "" {
		primary = cymruResult
	}
	flags := PenaltyFlags(0)
	extraDelay := time.Duration(0)

	if !ipKnown {
		flags |= PenaltyUnknown
		extraDelay = maxDuration(extraDelay, g.cfg.unknownPenalty)
	}
	if g.countryMismatch(ctyCountryKey, ipKey) {
		flags |= PenaltyCountryMismatch
		extraDelay = maxDuration(extraDelay, g.cfg.countryMismatchExtraWait)
	}
	if disagreement(ipResult, cymruResult) {
		flags |= PenaltyDisagreement
		extraDelay = maxDuration(extraDelay, g.cfg.disagreementPenalty)
	}

	record, recordCreated := g.loadRecord(call)
	asnNew, geoFlip := g.evaluateHistory(record, primary, ipKey, ipContinent)
	if asnNew {
		flags |= PenaltyASNReset
	}
	if geoFlip {
		flags |= PenaltyGeoFlip
	}

	newUser := recordCreated || asnNew || geoFlip
	if g.cfg.disagreementResetOnNew && flags.Has(PenaltyDisagreement) {
		newUser = true
	}

	initialWait := time.Duration(0)
	if newUser {
		initialWait = g.cfg.initialWait
	}
	updatedRecord := g.touchRecord(call, primary, ipKey, ipContinent)

	shard := g.callShard(call)
	shard.mu.Lock()
	state := shard.calls[call]
	if state == nil {
		state = &callState{perBandCounts: make(map[string]int)}
		shard.calls[call] = state
	}
	g.resetState(state, now, initialWait, extraDelay)
	state.penaltyFlags = flags
	state.ctyCountryKey = ctyCountryKey
	state.ctyContinentKey = ctyContinentKey
	state.currentASN = primary.ASN
	state.currentCountryKey = ipKey
	state.currentCountryCode = primary.CountryCode
	state.currentCountry = primary.CountryName
	state.currentSource = primary.Source
	state.currentPrefix, _ = prefixKeyFromIP(addr)
	state.lastSeen = now
	if updatedRecord != nil {
		state.asnHistory = append([]string(nil), updatedRecord.RecentASNs...)
		state.countryHistory = append([]string(nil), updatedRecord.RecentCountries...)
		state.continentHistory = append([]string(nil), updatedRecord.RecentContinents...)
	}
	shard.mu.Unlock()
}

// Check applies the reputation policy to an incoming spot request.
func (g *Gate) Check(req Request) Decision {
	if g == nil || !g.cfg.enabled {
		return Decision{Allow: true}
	}
	call := strings.ToUpper(strings.TrimSpace(req.Call))
	if call == "" {
		return Decision{Allow: true}
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	addr, _ := netip.ParseAddr(strings.TrimSpace(req.IP))
	prefix, _ := prefixKeyFromIP(addr)

	if addr.Is4() {
		if g.prefixV4 != nil && !g.prefixV4.allow(prefix, now) {
			return g.dropDecision(call, req.Band, prefix, DropPrefixCap, now)
		}
	} else if addr.Is6() {
		if g.prefixV6 != nil && !g.prefixV6.allow(prefix, now) {
			return g.dropDecision(call, req.Band, prefix, DropPrefixCap, now)
		}
	}

	shard := g.callShard(call)
	shard.mu.Lock()
	state := shard.calls[call]
	if state == nil {
		state = &callState{perBandCounts: make(map[string]int)}
		shard.calls[call] = state
		g.resetState(state, now, g.cfg.initialWait, 0)
		state.penaltyFlags = PenaltyUnknown
	}
	state.lastSeen = now
	g.advanceState(state, now)

	if now.Before(state.nextAllowedAt) {
		decision := g.dropDecisionLocked(state, req.Band, prefix, DropProbation)
		shard.mu.Unlock()
		return decision
	}

	band := spot.NormalizeBand(req.Band)
	if band == "" {
		band = "???"
	}

	if state.totalCap > 0 && state.totalCount >= state.totalCap {
		decision := g.dropDecisionLocked(state, band, prefix, DropTotalCap)
		shard.mu.Unlock()
		return decision
	}

	if state.perBandLimit > 0 {
		count := state.perBandCounts[band]
		if count >= state.perBandLimit {
			decision := g.dropDecisionLocked(state, band, prefix, DropBandCap)
			shard.mu.Unlock()
			return decision
		}
		state.perBandCounts[band] = count + 1
	}
	state.totalCount++
	decision := Decision{
		Allow:       true,
		ASN:         state.currentASN,
		CountryCode: state.currentCountryCode,
		CountryName: state.currentCountry,
		Source:      state.currentSource,
		Prefix:      prefix,
		Flags:       state.penaltyFlags,
	}
	shard.mu.Unlock()
	return decision
}

func (g *Gate) dropDecision(call, band, prefix string, reason DropReason, now time.Time) Decision {
	shard := g.callShard(call)
	shard.mu.Lock()
	state := shard.calls[call]
	if state == nil {
		state = &callState{}
	}
	decision := g.dropDecisionLocked(state, band, prefix, reason)
	shard.mu.Unlock()
	decision.Drop = true
	decision.Allow = false
	return decision
}

func (g *Gate) dropDecisionLocked(state *callState, band, prefix string, reason DropReason) Decision {
	decision := Decision{
		Allow:       false,
		Drop:        true,
		Reason:      reason,
		Flags:       state.penaltyFlags,
		ASN:         state.currentASN,
		CountryCode: state.currentCountryCode,
		CountryName: state.currentCountry,
		Source:      state.currentSource,
		Prefix:      prefix,
	}
	return decision
}

func (g *Gate) resetState(state *callState, now time.Time, initialWait, extraDelay time.Duration) {
	if state == nil {
		return
	}
	state.windowStart = now.Add(initialWait)
	state.nextAllowedAt = state.windowStart
	state.nextRampAt = state.windowStart.Add(g.cfg.rampWindow + extraDelay)
	state.perBandLimit = g.cfg.perBandStart
	state.totalCap = g.cfg.totalCapStart
	state.totalCapUpgradeAt = time.Time{}
	state.perBandCounts = make(map[string]int)
	state.totalCount = 0
	if g.cfg.perBandCap <= state.perBandLimit {
		state.perBandLimit = g.cfg.perBandCap
		if g.cfg.totalCapPostRamp > state.totalCap {
			state.totalCapUpgradeAt = state.windowStart.Add(g.cfg.totalCapRampDelay)
		}
	}
}

func (g *Gate) advanceState(state *callState, now time.Time) {
	if state == nil {
		return
	}
	if state.windowStart.IsZero() {
		state.windowStart = now
	}
	if now.After(state.windowStart) && g.cfg.rampWindow > 0 {
		elapsed := now.Sub(state.windowStart)
		if elapsed >= g.cfg.rampWindow {
			steps := int(elapsed / g.cfg.rampWindow)
			state.windowStart = state.windowStart.Add(time.Duration(steps) * g.cfg.rampWindow)
			state.perBandCounts = make(map[string]int)
			state.totalCount = 0
		}
	}
	if g.cfg.perBandCap > state.perBandLimit && !state.nextRampAt.IsZero() && !now.Before(state.nextRampAt) {
		elapsed := now.Sub(state.nextRampAt)
		steps := int(elapsed/g.cfg.rampWindow) + 1
		for i := 0; i < steps && state.perBandLimit < g.cfg.perBandCap; i++ {
			state.perBandLimit++
			state.nextRampAt = state.nextRampAt.Add(g.cfg.rampWindow)
		}
		if state.perBandLimit >= g.cfg.perBandCap && state.totalCapUpgradeAt.IsZero() {
			state.totalCapUpgradeAt = now.Add(g.cfg.totalCapRampDelay)
		}
	}
	if state.totalCapUpgradeAt.IsZero() {
		return
	}
	if now.After(state.totalCapUpgradeAt) || now.Equal(state.totalCapUpgradeAt) {
		state.totalCap = g.cfg.totalCapPostRamp
		state.totalCapUpgradeAt = time.Time{}
	}
}

func (g *Gate) callShard(call string) *callShard {
	hash := fnv32a(call)
	return &g.callShards[int(hash%uint32(len(g.callShards)))]
}

func (g *Gate) lookupCTY(call string) (string, string) {
	if g.ctyLookup == nil {
		return "", ""
	}
	db := g.ctyLookup()
	if db == nil {
		return "", ""
	}
	info, ok := db.LookupCallsignPortable(call)
	if !ok || info == nil {
		return "", ""
	}
	countryKey, _ := countryKeyFromName(info.Country)
	continentKey, _ := continentKey(info.Continent)
	return countryKey, continentKey
}

func (g *Gate) lookupIP(addr netip.Addr, now time.Time) (LookupResult, LookupResult, string, string, bool) {
	if !addr.IsValid() {
		return LookupResult{}, LookupResult{}, "", "", false
	}
	key := addr.String()
	ipinfo := LookupResult{}
	if cached, ok, _ := g.lookupCache.get(key, now); ok {
		if g.cfg.ipinfoSnapshotMaxAge == 0 || now.Sub(cached.FetchedAt) <= g.cfg.ipinfoSnapshotMaxAge {
			countryKey, _ := countryKeyFromCode(cached.CountryCode)
			if countryKey == "" {
				countryKey, _ = countryKeyFromName(cached.CountryName)
			}
			continentKey, _ := continentKey(cached.ContinentCode)
			return cached, LookupResult{}, countryKey, continentKey, true
		}
	}
	ipinfo = g.lookupIPInfo(addr, now)
	if ipinfo.Source != "" {
		g.lookupCache.set(key, ipinfo, now, false)
	}
	cymru := LookupResult{}
	if g.cymru != nil {
		if result, ok, _ := g.cymru.lookup(addr, now); ok {
			cymru = result
		}
	}

	primary := ipinfo
	if primary.Source == "" {
		primary = cymru
	}
	countryKey, _ := countryKeyFromCode(primary.CountryCode)
	if countryKey == "" {
		countryKey, _ = countryKeyFromName(primary.CountryName)
	}
	continentKey, _ := continentKey(primary.ContinentCode)
	if primary.Source == "" {
		return ipinfo, cymru, "", "", false
	}
	return ipinfo, cymru, countryKey, continentKey, true
}

func (g *Gate) lookupIPInfo(addr netip.Addr, now time.Time) LookupResult {
	if g == nil {
		return LookupResult{}
	}
	idx := g.ipinfoIndex.Load()
	if idx == nil {
		return LookupResult{}
	}
	if g.cfg.ipinfoSnapshotMaxAge > 0 && now.Sub(idx.loadedAt) > g.cfg.ipinfoSnapshotMaxAge {
		return LookupResult{}
	}
	result, ok := idx.lookup(addr)
	if !ok {
		return LookupResult{}
	}
	return result
}

func (g *Gate) countryMismatch(ctyCountryKey, ipCountryKey string) bool {
	if ctyCountryKey == "" || ipCountryKey == "" {
		return false
	}
	return ctyCountryKey != ipCountryKey
}

func disagreement(ipinfo, cymru LookupResult) bool {
	if ipinfo.Source == "" || cymru.Source == "" {
		return false
	}
	if ipinfo.ASN != "" && cymru.ASN != "" && ipinfo.ASN != cymru.ASN {
		return true
	}
	ipCountryKey, _ := countryKeyFromCode(ipinfo.CountryCode)
	if ipCountryKey == "" {
		ipCountryKey, _ = countryKeyFromName(ipinfo.CountryName)
	}
	cymruCountryKey, _ := countryKeyFromCode(cymru.CountryCode)
	if ipCountryKey != "" && cymruCountryKey != "" && ipCountryKey != cymruCountryKey {
		return true
	}
	return false
}

func (g *Gate) evaluateHistory(record *Record, ip LookupResult, ipCountryKey, ipContinentKey string) (bool, bool) {
	if record == nil {
		return false, false
	}
	asnNew := false
	if g.cfg.resetOnNewASN && ip.ASN != "" {
		if !contains(record.RecentASNs, ip.ASN) {
			asnNew = true
		}
	}
	geoFlip := false
	switch strings.ToLower(g.cfg.countryFlipScope) {
	case "continent":
		if ipContinentKey != "" && len(record.RecentContinents) > 0 && record.RecentContinents[0] != ipContinentKey {
			geoFlip = true
		}
	default:
		if ipCountryKey != "" && len(record.RecentCountries) > 0 && record.RecentCountries[0] != ipCountryKey {
			geoFlip = true
		}
	}
	return asnNew, geoFlip
}

func (g *Gate) loadRecord(call string) (*Record, bool) {
	if g.store == nil {
		return nil, true
	}
	record, err := g.store.Load(call)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("Warning: reputation store read failed for %s: %v", call, err)
		}
		return nil, true
	}
	return record, false
}

func (g *Gate) touchRecord(call string, ip LookupResult, ipCountryKey, ipContinentKey string) *Record {
	if g.store == nil {
		return nil
	}
	record, _, err := g.store.Touch(call, ip.ASN, ipCountryKey, ipContinentKey)
	if err != nil {
		log.Printf("Warning: reputation store update failed for %s: %v", call, err)
		return nil
	}
	return record
}

func (g *Gate) sweeper(ctx context.Context) {
	ticker := time.NewTicker(g.sweeperEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			g.sweepCalls(now)
			if g.prefixV4 != nil {
				g.prefixV4.sweep(now)
			}
			if g.prefixV6 != nil {
				g.prefixV6.sweep(now)
			}
		}
	}
}

func (g *Gate) sweepCalls(now time.Time) {
	if g.cfg.stateTTL <= 0 {
		return
	}
	for i := range g.callShards {
		shard := &g.callShards[i]
		shard.mu.Lock()
		for call, state := range shard.calls {
			if state == nil || now.Sub(state.lastSeen) > g.cfg.stateTTL {
				delete(shard.calls, call)
			}
		}
		if g.cfg.stateMaxEntries > 0 && len(shard.calls) > g.cfg.stateMaxEntries {
			for call := range shard.calls {
				delete(shard.calls, call)
				if len(shard.calls) <= g.cfg.stateMaxEntries {
					break
				}
			}
		}
		shard.mu.Unlock()
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if b > a {
		return b
	}
	return a
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func normalizeGateConfig(cfg config.ReputationConfig) gateConfig {
	g := gateConfig{
		enabled:                  cfg.Enabled,
		initialWait:              time.Duration(cfg.InitialWaitSeconds) * time.Second,
		rampWindow:               time.Duration(cfg.RampWindowSeconds) * time.Second,
		perBandStart:             cfg.PerBandStart,
		perBandCap:               cfg.PerBandCap,
		totalCapStart:            cfg.TotalCapStart,
		totalCapPostRamp:         cfg.TotalCapPostRamp,
		totalCapRampDelay:        time.Duration(cfg.TotalCapRampDelaySeconds) * time.Second,
		countryMismatchExtraWait: time.Duration(cfg.CountryMismatchExtraWaitSeconds) * time.Second,
		disagreementPenalty:      time.Duration(cfg.DisagreementPenaltySeconds) * time.Second,
		unknownPenalty:           time.Duration(cfg.UnknownPenaltySeconds) * time.Second,
		disagreementResetOnNew:   cfg.DisagreementResetOnNew,
		resetOnNewASN:            cfg.ResetOnNewASN,
		countryFlipScope:         cfg.CountryFlipScope,
		maxASNHistory:            cfg.MaxASNHistory,
		maxCountryHistory:        cfg.MaxCountryHistory,
		stateTTL:                 time.Duration(cfg.StateTTLSeconds) * time.Second,
		stateMaxEntries:          cfg.StateMaxEntries,
		prefixTTL:                time.Duration(cfg.PrefixTTLSeconds) * time.Second,
		prefixMaxEntries:         cfg.PrefixMaxEntries,
		ipinfoSnapshotMaxAge:     time.Duration(cfg.SnapshotMaxAgeSeconds) * time.Second,
		ipinfoSnapshotPath:       cfg.IPInfoSnapshotPath,
		ipinfoDownloadToken:      cfg.IPInfoDownloadToken,
		ipinfoDownloadURL:        cfg.IPInfoDownloadURL,
		ipinfoDownloadPath:       cfg.IPInfoDownloadPath,
		ipinfoRefreshUTC:         cfg.IPInfoRefreshUTC,
		ipinfoDownloadTimeout:    time.Duration(cfg.IPInfoDownloadTimeoutMS) * time.Millisecond,
		ipinfoDownloadEnabled:    cfg.Enabled,
		cymruEnabled:             cfg.FallbackTeamCymru,
		cymruLookupTimeout:       time.Duration(cfg.CymruLookupTimeoutMS) * time.Millisecond,
		cymruCacheTTL:            time.Duration(cfg.CymruCacheTTLSeconds) * time.Second,
		cymruNegativeTTL:         time.Duration(cfg.CymruNegativeTTLSeconds) * time.Second,
		cymruWorkers:             cfg.CymruWorkers,
		lookupCacheTTL:           time.Duration(cfg.LookupCacheTTLSeconds) * time.Second,
		lookupCacheMaxEntries:    cfg.LookupCacheMaxEntries,
		ipv4BucketSize:           cfg.IPv4BucketSize,
		ipv4BucketRefillPerSec:   cfg.IPv4BucketRefillPerSec,
		ipv6BucketSize:           cfg.IPv6BucketSize,
		ipv6BucketRefillPerSec:   cfg.IPv6BucketRefillPerSec,
	}
	if g.rampWindow <= 0 {
		g.rampWindow = time.Minute
	}
	if g.initialWait < 0 {
		g.initialWait = time.Minute
	}
	if g.perBandStart <= 0 {
		g.perBandStart = 1
	}
	if g.perBandCap <= 0 {
		g.perBandCap = 5
	}
	if g.perBandStart > g.perBandCap {
		g.perBandStart = g.perBandCap
	}
	if g.totalCapStart <= 0 {
		g.totalCapStart = 5
	}
	if g.totalCapPostRamp <= 0 {
		g.totalCapPostRamp = g.totalCapStart
	}
	if g.totalCapPostRamp < g.totalCapStart {
		g.totalCapPostRamp = g.totalCapStart
	}
	if g.lookupCacheTTL <= 0 {
		g.lookupCacheTTL = time.Hour
	}
	if g.lookupCacheMaxEntries <= 0 {
		g.lookupCacheMaxEntries = 200000
	}
	if g.stateTTL <= 0 {
		g.stateTTL = 2 * time.Hour
	}
	if g.stateMaxEntries <= 0 {
		g.stateMaxEntries = 100000
	}
	if g.prefixTTL <= 0 {
		g.prefixTTL = time.Hour
	}
	if g.prefixMaxEntries <= 0 {
		g.prefixMaxEntries = 200000
	}
	if g.countryFlipScope == "" {
		g.countryFlipScope = "country"
	}
	if g.ipinfoSnapshotPath == "" {
		g.ipinfoSnapshotPath = "data/ipinfo/location.csv"
	}
	if g.ipinfoDownloadPath == "" {
		g.ipinfoDownloadPath = "data/ipinfo/location.csv.gz"
	}
	if g.ipinfoDownloadURL == "" {
		g.ipinfoDownloadURL = "https://ipinfo.io/data/location.csv.gz?token=$TOKEN"
	}
	if g.ipinfoDownloadToken == "" {
		g.ipinfoDownloadToken = "8a74cd36c1905b"
	}
	if g.ipinfoDownloadTimeout <= 0 {
		g.ipinfoDownloadTimeout = 15 * time.Second
	}
	if g.ipinfoSnapshotMaxAge <= 0 {
		g.ipinfoSnapshotMaxAge = 26 * time.Hour
	}
	if g.cymruLookupTimeout <= 0 {
		g.cymruLookupTimeout = 250 * time.Millisecond
	}
	if g.cymruCacheTTL <= 0 {
		g.cymruCacheTTL = time.Hour
	}
	if g.cymruNegativeTTL <= 0 {
		g.cymruNegativeTTL = 5 * time.Minute
	}
	if g.countryMismatchExtraWait <= 0 {
		g.countryMismatchExtraWait = time.Minute
	}
	if g.disagreementPenalty <= 0 {
		g.disagreementPenalty = time.Minute
	}
	if g.unknownPenalty <= 0 {
		g.unknownPenalty = time.Minute
	}
	if g.maxASNHistory <= 0 {
		g.maxASNHistory = 5
	}
	if g.maxCountryHistory <= 0 {
		g.maxCountryHistory = 5
	}
	if g.ipv4BucketSize <= 0 {
		g.ipv4BucketSize = 64
	}
	if g.ipv4BucketRefillPerSec <= 0 {
		g.ipv4BucketRefillPerSec = 8
	}
	if g.ipv6BucketSize <= 0 {
		g.ipv6BucketSize = 32
	}
	if g.ipv6BucketRefillPerSec <= 0 {
		g.ipv6BucketRefillPerSec = 4
	}
	g.ipinfoDownloadEnabled = g.enabled &&
		g.ipinfoDownloadToken != "" &&
		g.ipinfoDownloadURL != "" &&
		g.ipinfoDownloadPath != "" &&
		g.ipinfoSnapshotPath != ""
	return g
}
