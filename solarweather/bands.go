package solarweather

import (
	"fmt"
	"strings"
)

type bandMask uint16

const (
	band160 bandMask = 1 << iota
	band80
	band60
	band40
	band30
	band20
	band17
	band15
	band12
	band10
)

var bandNames = []string{
	"160m",
	"80m",
	"60m",
	"40m",
	"30m",
	"20m",
	"17m",
	"15m",
	"12m",
	"10m",
}

var bandByName = map[string]bandMask{
	"160m": band160,
	"80m":  band80,
	"60m":  band60,
	"40m":  band40,
	"30m":  band30,
	"20m":  band20,
	"17m":  band17,
	"15m":  band15,
	"12m":  band12,
	"10m":  band10,
}

func normalizeBand(band string) (string, bool) {
	b := strings.ToLower(strings.TrimSpace(band))
	if b == "" {
		return "", false
	}
	if _, ok := bandByName[b]; ok {
		return b, true
	}
	return "", false
}

func bandsToMask(bands []string) (bandMask, error) {
	var mask bandMask
	for _, entry := range bands {
		b, ok := normalizeBand(entry)
		if !ok {
			return 0, fmt.Errorf("unknown band %q", entry)
		}
		mask |= bandByName[b]
	}
	if mask == 0 {
		return 0, fmt.Errorf("band list cannot be empty")
	}
	return mask, nil
}

func bandAllowed(mask bandMask, band string) bool {
	b, ok := normalizeBand(band)
	if !ok {
		return false
	}
	return mask&bandByName[b] != 0
}

func bandListFromMask(mask bandMask) []string {
	if mask == 0 {
		return nil
	}
	out := make([]string, 0, len(bandNames))
	for i, name := range bandNames {
		if mask&(1<<uint(i)) != 0 {
			out = append(out, name)
		}
	}
	return out
}

func bandListString(mask bandMask) string {
	list := bandListFromMask(mask)
	if len(list) == 0 {
		return ""
	}
	return strings.Join(list, "/")
}
