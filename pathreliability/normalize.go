package pathreliability

import (
	"strings"
)

// FT8Equivalent returns the FT8-equivalent dB for a mode/SNR pair.
// Returns ok=false when mode is unsupported for path reliability.
func FT8Equivalent(mode string, snr int, cfg Config) (float64, bool) {
	switch normalizeMode(mode) {
	case "FT8":
		return float64(snr), true
	case "FT4":
		return float64(snr) + cfg.ModeOffsets.FT4, true
	case "CW":
		return float64(snr) + cfg.ModeOffsets.CW, true
	case "RTTY":
		return float64(snr) + cfg.ModeOffsets.RTTY, true
	case "PSK":
		return float64(snr) + cfg.ModeOffsets.PSK, true
	default:
		return 0, false
	}
}

func normalizeMode(mode string) string {
	up := strings.ToUpper(strings.TrimSpace(mode))
	return up
}

// ApplyNoise subtracts a noise penalty from the DX->user direction.
func ApplyNoise(value float64, offset float64, clampMin, clampMax float64) float64 {
	return clamp(value-offset, clampMin, clampMax)
}

// GlyphForDB maps FT8-equiv dB to the ASCII glyph scale for a given mode.
func GlyphForDB(ft8dB float64, mode string, cfg Config) string {
	thresholds := thresholdsForMode(mode, cfg)
	switch {
	case ft8dB >= thresholds.High:
		return cfg.GlyphSymbols.High
	case ft8dB >= thresholds.Medium:
		return cfg.GlyphSymbols.Medium
	case ft8dB >= thresholds.Low:
		return cfg.GlyphSymbols.Low
	case ft8dB >= thresholds.Unlikely:
		return cfg.GlyphSymbols.Unlikely
	default:
		return cfg.GlyphSymbols.Unlikely
	}
}

const (
	classHigh     = "HIGH"
	classMedium   = "MEDIUM"
	classLow      = "LOW"
	classUnlikely = "UNLIKELY"
)

// ClassForDB maps FT8-equiv dB to the threshold class name for filtering.
func ClassForDB(ft8dB float64, mode string, cfg Config) string {
	thresholds := thresholdsForMode(mode, cfg)
	switch {
	case ft8dB >= thresholds.High:
		return classHigh
	case ft8dB >= thresholds.Medium:
		return classMedium
	case ft8dB >= thresholds.Low:
		return classLow
	default:
		return classUnlikely
	}
}

func thresholdsForMode(mode string, cfg Config) GlyphThresholds {
	key := normalizeMode(mode)
	if cfg.ModeThresholds != nil {
		if t, ok := cfg.ModeThresholds[key]; ok && validGlyphThresholds(t) {
			return t
		}
	}
	if validGlyphThresholds(cfg.GlyphThresholds) {
		return cfg.GlyphThresholds
	}
	return DefaultConfig().GlyphThresholds
}

// SelectSample blends fine/coarse samples by confidence weight.
// If fine is below minFineWeight, coarse wins to prevent weak fine data
// from overriding stronger regional evidence. Neighbor fallback is used
// only when the coarse bucket has no data.
func SelectSample(fine Sample, coarse Sample, neighbors []Sample, minFineWeight float64) Sample {
	coarseCandidate := coarse
	if coarseCandidate.Weight <= 0 {
		coarseCandidate = combineNeighborSamples(neighbors)
	}
	hasFine := fine.Weight > 0
	hasCoarse := coarseCandidate.Weight > 0
	switch {
	case hasFine && !hasCoarse:
		return fine
	case !hasFine && hasCoarse:
		return coarseCandidate
	case !hasFine && !hasCoarse:
		return Sample{}
	}
	if minFineWeight > 0 && fine.Weight < minFineWeight {
		return coarseCandidate
	}
	sum := fine.Weight + coarseCandidate.Weight
	if sum <= 0 {
		return Sample{}
	}
	age := fine.AgeSec
	if coarseCandidate.AgeSec < age {
		age = coarseCandidate.AgeSec
	}
	return Sample{
		Value:  (fine.Value*fine.Weight + coarseCandidate.Value*coarseCandidate.Weight) / sum,
		Weight: sum,
		AgeSec: age,
	}
}

func combineNeighborSamples(neighbors []Sample) Sample {
	var sumW, sumV float64
	for _, n := range neighbors {
		if n.Weight > 0 {
			sumW += n.Weight
			sumV += n.Value * n.Weight
		}
	}
	if sumW == 0 {
		return Sample{}
	}
	return Sample{Value: sumV / sumW, Weight: sumW}
}
