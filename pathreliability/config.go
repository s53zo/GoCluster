package pathreliability

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds tuning knobs for path reliability aggregation and display.
type Config struct {
	Enabled             bool               `yaml:"enabled"`
	ClampMin            float64            `yaml:"clamp_min"`                 // FT8-equiv floor (dB)
	ClampMax            float64            `yaml:"clamp_max"`                 // FT8-equiv ceiling (dB)
	DefaultHalfLifeSec  int                `yaml:"default_half_life_seconds"` // fallback half-life when band not listed
	BandHalfLifeSec     map[string]int     `yaml:"band_half_life_seconds"`    // per-band overrides
	StaleAfterSeconds   int                `yaml:"stale_after_seconds"`       // purge when older than this
	MinEffectiveWeight  float64            `yaml:"min_effective_weight"`      // minimum decayed weight to report
	NeighborRadius      int                `yaml:"neighbor_radius"`           // 0 or 1 (N/S/E/W)
	ReverseHintDiscount float64            `yaml:"reverse_hint_discount"`     // multiplier when using reverse direction
	MergeReceiveWeight  float64            `yaml:"merge_receive_weight"`      // merge weight for DX->user
	MergeTransmitWeight float64            `yaml:"merge_transmit_weight"`     // merge weight for user->DX
	BeaconWeightCap     float64            `yaml:"beacon_weight_cap"`         // cap per-beacon contribution
	DisplayEnabled      bool               `yaml:"display_enabled"`           // toggle glyph rendering
	ModeOffsets         ModeOffsets        `yaml:"mode_offsets"`              // per-mode FT8-equiv offsets
	ModeThresholds      map[string]GlyphThresholds `yaml:"mode_thresholds"`   // per-mode glyph thresholds in FT8-equiv dB
	GlyphThresholds     GlyphThresholds    `yaml:"glyph_thresholds"`          // fallback glyph thresholds in FT8-equiv dB
	NoiseOffsets        map[string]float64 `yaml:"noise_offsets"`             // noise class -> dB penalty
}

// ModeOffsets normalizes non-FT8 modes to FT8-equivalent dB.
type ModeOffsets struct {
	FT4  float64 `yaml:"ft4"`
	CW   float64 `yaml:"cw"`
	RTTY float64 `yaml:"rtty"`
	PSK  float64 `yaml:"psk"`
}

// GlyphThresholds defines FT8-equiv dB cutoffs for glyphs.
type GlyphThresholds struct {
	Excellent float64 `yaml:"excellent"` // >= Excellent
	Good      float64 `yaml:"good"`      // >= Good
	Marginal  float64 `yaml:"marginal"`  // >= Marginal
}

// DefaultConfig returns a safe, enabled configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:             true,
		ClampMin:            -25,
		ClampMax:            15,
		DefaultHalfLifeSec:  300,
		BandHalfLifeSec:     map[string]int{},
		StaleAfterSeconds:   1800,
		MinEffectiveWeight:  1.0,
		NeighborRadius:      1,
		ReverseHintDiscount: 0.5,
		MergeReceiveWeight:  0.6,
		MergeTransmitWeight: 0.4,
		BeaconWeightCap:     1.0,
		DisplayEnabled:      true,
		ModeOffsets: ModeOffsets{
			FT4:  -3,
			CW:   -7,
			RTTY: -7,
			PSK:  -7,
		},
		ModeThresholds: map[string]GlyphThresholds{
			"FT8":  {Excellent: -13, Good: -17, Marginal: -21},
			"FT4":  {Excellent: -13, Good: -17, Marginal: -21},
			"CW":   {Excellent: 5, Good: -1, Marginal: -5},
			"RTTY": {Excellent: 5, Good: -1, Marginal: -5},
			"PSK":  {Excellent: 5, Good: -1, Marginal: -5},
		},
		GlyphThresholds: GlyphThresholds{
			Excellent: -13,
			Good:      -17,
			Marginal:  -21,
		},
		NoiseOffsets: map[string]float64{
			"QUIET":    0,
			"RURAL":    6,
			"SUBURBAN": 9,
			"URBAN":    12,
		},
	}
}

// normalize fills defaults and clamps obvious invalids.
func (c *Config) normalize() {
	if c == nil {
		return
	}
	def := DefaultConfig()
	if c.ClampMax <= c.ClampMin {
		c.ClampMin = def.ClampMin
		c.ClampMax = def.ClampMax
	}
	if c.DefaultHalfLifeSec <= 0 {
		c.DefaultHalfLifeSec = def.DefaultHalfLifeSec
	}
	if c.StaleAfterSeconds <= 0 {
		c.StaleAfterSeconds = def.StaleAfterSeconds
	}
	if c.MinEffectiveWeight <= 0 {
		c.MinEffectiveWeight = def.MinEffectiveWeight
	}
	if c.NeighborRadius < 0 {
		c.NeighborRadius = 0
	}
	if c.NeighborRadius > 1 {
		c.NeighborRadius = 1
	}
	if c.ReverseHintDiscount <= 0 || c.ReverseHintDiscount > 1 {
		c.ReverseHintDiscount = def.ReverseHintDiscount
	}
	if c.MergeReceiveWeight <= 0 {
		c.MergeReceiveWeight = def.MergeReceiveWeight
	}
	if c.MergeTransmitWeight <= 0 {
		c.MergeTransmitWeight = def.MergeTransmitWeight
	}
	sum := c.MergeReceiveWeight + c.MergeTransmitWeight
	if sum <= 0 {
		c.MergeReceiveWeight = def.MergeReceiveWeight
		c.MergeTransmitWeight = def.MergeTransmitWeight
		sum = c.MergeReceiveWeight + c.MergeTransmitWeight
	}
	if sum != 1 {
		c.MergeReceiveWeight /= sum
		c.MergeTransmitWeight /= sum
	}
	if c.BeaconWeightCap <= 0 {
		c.BeaconWeightCap = def.BeaconWeightCap
	}
	// Mode offsets: leave caller-provided values unless zeroed.
	if c.ModeOffsets.FT4 == 0 {
		c.ModeOffsets.FT4 = def.ModeOffsets.FT4
	}
	if c.ModeOffsets.CW == 0 {
		c.ModeOffsets.CW = def.ModeOffsets.CW
	}
	if c.ModeOffsets.RTTY == 0 {
		c.ModeOffsets.RTTY = def.ModeOffsets.RTTY
	}
	if c.ModeOffsets.PSK == 0 {
		c.ModeOffsets.PSK = def.ModeOffsets.PSK
	}
	if c.ModeThresholds == nil {
		c.ModeThresholds = map[string]GlyphThresholds{}
	}
	normalizedThresholds := make(map[string]GlyphThresholds, len(c.ModeThresholds))
	for k, v := range c.ModeThresholds {
		key := strings.ToUpper(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		normalizedThresholds[key] = v
	}
	c.ModeThresholds = normalizedThresholds
	for mode, defThresholds := range def.ModeThresholds {
		if t, ok := c.ModeThresholds[mode]; !ok || !validGlyphThresholds(t) {
			c.ModeThresholds[mode] = defThresholds
		}
	}
	if !validGlyphThresholds(c.GlyphThresholds) {
		c.GlyphThresholds = def.GlyphThresholds
	}
	if c.NoiseOffsets == nil {
		c.NoiseOffsets = map[string]float64{}
	}
	// Ensure canonical noise keys exist.
	for k, v := range def.NoiseOffsets {
		if _, ok := c.NoiseOffsets[k]; !ok {
			c.NoiseOffsets[k] = v
		}
	}
	// Normalize noise keys to uppercase for lookup stability.
	normalized := make(map[string]float64, len(c.NoiseOffsets))
	for k, v := range c.NoiseOffsets {
		normalized[strings.ToUpper(strings.TrimSpace(k))] = v
	}
	c.NoiseOffsets = normalized
}

func validGlyphThresholds(t GlyphThresholds) bool {
	return t.Excellent > t.Good && t.Good > t.Marginal
}

// LoadFile loads YAML config and applies defaults.
func LoadFile(path string) (Config, error) {
	cfg := DefaultConfig()
	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}
	bs, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(bs, &cfg); err != nil {
		return cfg, err
	}
	cfg.normalize()
	return cfg, nil
}
