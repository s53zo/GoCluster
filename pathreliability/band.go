package pathreliability

import "strings"

// BandIndex maps band strings to compact integers.
type BandIndex struct {
	bandToIdx map[string]uint16
	idxToBand []string
}

// NewBandIndex builds a stable mapping.
func NewBandIndex(bands []string) BandIndex {
	m := make(map[string]uint16, len(bands))
	idxToBand := make([]string, 0, len(bands))
	for _, b := range bands {
		key := normalizeBand(b)
		if key == "" {
			continue
		}
		if _, exists := m[key]; !exists {
			m[key] = uint16(len(idxToBand))
			idxToBand = append(idxToBand, key)
		}
	}
	return BandIndex{bandToIdx: m, idxToBand: idxToBand}
}

func normalizeBand(b string) string {
	return strings.ToLower(strings.TrimSpace(b))
}

// Index returns the band index or zero when unknown (zero is still usable).
func (b BandIndex) Index(band string) uint16 {
	idx, _ := b.Lookup(band)
	return idx
}

// Lookup returns the band index and whether it is known.
func (b BandIndex) Lookup(band string) (uint16, bool) {
	key := normalizeBand(band)
	idx, ok := b.bandToIdx[key]
	return idx, ok
}

// HalfLifeSeconds returns the per-band override or default from config.
func (b BandIndex) HalfLifeSeconds(band string, cfg Config) int {
	if cfg.BandHalfLifeSec != nil {
		if hl, ok := cfg.BandHalfLifeSec[normalizeBand(band)]; ok && hl > 0 {
			return hl
		}
	}
	return cfg.DefaultHalfLifeSec
}
