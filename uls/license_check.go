package uls

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"unicode"

	"dxcluster/spot"
)

var (
	licenseDBPath string
	licenseDB     *sql.DB
	licenseOnce   sync.Once
	licenseCache  sync.Map // call (uppercased) -> bool
)

// SetLicenseDBPath configures the path to the FCC ULS SQLite database used for license lookups.
// If the path is empty or the DB cannot be opened, lookups will be skipped and treated as allowed.
func SetLicenseDBPath(path string) {
	licenseDBPath = strings.TrimSpace(path)
}

// IsLicensedUS reports whether the callsign appears in the FCC ULS AM table (active licenses only).
// If no DB is configured or the DB cannot be opened, the call is treated as licensed (allowed).
// Results are cached for the lifetime of the process.
func IsLicensedUS(call string) bool {
	canonical := NormalizeForLicense(call)
	if canonical == "" {
		return true
	}

	if cached, ok := licenseCache.Load(canonical); ok {
		return cached.(bool)
	}

	allow := true

	db := getLicenseDB()
	if db != nil {
		var dummy int
		err := db.QueryRow("SELECT 1 FROM AM WHERE call_sign = ? LIMIT 1;", canonical).Scan(&dummy)
		if err == nil {
			allow = true
		} else if err == sql.ErrNoRows {
			allow = false
		} else {
			// On query errors, default to allow and log once.
			log.Printf("FCC ULS lookup failed for %s: %v", call, err)
			allow = true
		}
	}

	licenseCache.Store(canonical, allow)
	return allow
}

func getLicenseDB() *sql.DB {
	licenseOnce.Do(func() {
		if licenseDBPath == "" {
			return
		}
		// Open read-only with WAL/query-only pragmas and a short busy timeout so readers can wait
		// while the refresh job swaps in a new DB.
		dsn := fmt.Sprintf("file:%s?mode=ro&_busy_timeout=5000&_pragma=query_only(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", licenseDBPath)
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			log.Printf("FCC ULS: unable to open license DB at %s: %v (skipping license checks)", licenseDBPath, err)
			return
		}
		licenseDB = db
	})
	return licenseDB
}

// NormalizeForLicense strips adornments (SSID, skimmer suffix, portable prefixes)
// so FCC lookups use the underlying base callsign.
func NormalizeForLicense(call string) string {
	normalized := spot.NormalizeCallsign(call)
	if normalized == "" {
		return ""
	}
	normalized = strings.TrimSuffix(normalized, "-#") // RBN skimmer indicator

	// When slashes are present, pick the most callsign-like segment (longest slice that contains a digit)
	// so base calls like W1VF/VE3 resolve to the licensed call rather than the location suffix.
	if strings.Contains(normalized, "/") {
		segments := strings.Split(normalized, "/")
		var candidate string
		var candidateLen int
		for _, seg := range segments {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			if idx := strings.IndexFunc(seg, unicode.IsDigit); idx >= 0 {
				if len(seg) > candidateLen {
					candidate = seg
					candidateLen = len(seg)
				}
			}
		}
		if candidate != "" {
			normalized = candidate
		} else if len(segments) > 0 {
			normalized = segments[0]
		}
	}

	// Drop SSID or other hyphen suffixes for license lookup.
	if idx := strings.Index(normalized, "-"); idx > 0 {
		normalized = normalized[:idx]
	}

	return strings.TrimSpace(normalized)
}
