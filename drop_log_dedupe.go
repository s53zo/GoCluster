package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultDropLogDedupeMaxKeys = 512
)

type dropLogDeduper struct {
	mu      sync.Mutex
	window  time.Duration
	maxKeys int
	now     func() time.Time
	entries map[string]dropLogDedupeEntry
}

type dropLogDedupeEntry struct {
	nextEmit   time.Time
	lastSeen   time.Time
	suppressed uint64
}

func newDropLogDeduper(window time.Duration, maxKeys int) *dropLogDeduper {
	if window <= 0 || maxKeys <= 0 {
		return nil
	}
	return &dropLogDeduper{
		window:  window,
		maxKeys: maxKeys,
		now:     func() time.Time { return time.Now().UTC() },
		entries: make(map[string]dropLogDedupeEntry, maxKeys),
	}
}

func (d *dropLogDeduper) Process(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	if d == nil {
		return line, true
	}
	key, ok := dropLogDedupeKey(line)
	if !ok {
		return line, true
	}
	now := d.now()
	d.mu.Lock()
	defer d.mu.Unlock()

	entry, found := d.entries[key]
	if !found {
		d.evictOneIfNeededLocked()
		d.entries[key] = dropLogDedupeEntry{
			nextEmit: now.Add(d.window),
			lastSeen: now,
		}
		return line, true
	}
	entry.lastSeen = now
	if now.Before(entry.nextEmit) {
		entry.suppressed++
		d.entries[key] = entry
		return "", false
	}
	suppressed := entry.suppressed
	entry.suppressed = 0
	entry.nextEmit = now.Add(d.window)
	d.entries[key] = entry
	if suppressed > 0 {
		line = fmt.Sprintf("%s (suppressed=%d over %s)", line, suppressed, d.window)
	}
	return line, true
}

func (d *dropLogDeduper) evictOneIfNeededLocked() {
	if d == nil || d.maxKeys <= 0 {
		return
	}
	if len(d.entries) < d.maxKeys {
		return
	}
	var oldestKey string
	var oldestSeen time.Time
	haveOldest := false
	for key, entry := range d.entries {
		if !haveOldest || entry.lastSeen.Before(oldestSeen) {
			oldestKey = key
			oldestSeen = entry.lastSeen
			haveOldest = true
		}
	}
	if haveOldest {
		delete(d.entries, oldestKey)
	}
}

func dropLogDedupeKey(line string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 5 {
		return "", false
	}
	if fields[0] == "CTY" && fields[1] == "drop:" {
		kind := strings.ToLower(cleanLogToken(fields[2]))
		if kind != "unknown" && kind != "invalid" {
			return "", false
		}
		role := strings.ToUpper(cleanLogToken(fields[3]))
		call := strings.ToUpper(cleanLogToken(fields[4]))
		if role == "" || call == "" {
			return "", false
		}
		return "cty:" + kind + ":" + role + ":" + call, true
	}
	if fields[0] == "Unlicensed" && strings.EqualFold(fields[1], "US") {
		role := strings.ToUpper(cleanLogToken(fields[2]))
		call := strings.ToUpper(cleanLogToken(fields[3]))
		if role == "" || call == "" {
			return "", false
		}
		return "unlicensed:" + role + ":" + call, true
	}
	return "", false
}

func cleanLogToken(token string) string {
	if token == "" {
		return ""
	}
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	inTag := false
	for i := 0; i < len(trimmed); i++ {
		ch := trimmed[i]
		switch ch {
		case '[':
			inTag = true
		case ']':
			if inTag {
				inTag = false
			} else {
				b.WriteByte(ch)
			}
		default:
			if !inTag {
				b.WriteByte(ch)
			}
		}
	}
	return strings.TrimSpace(strings.Trim(b.String(), "():,;"))
}
