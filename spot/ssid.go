package spot

import "strings"

// Purpose: Collapse numeric SSID suffixes so skimmer variants are treated as one.
// Key aspects: Preserves non-numeric suffixes and keeps "-#" markers intact.
// Upstream: Telnet broadcast normalization and skimmer aggregation.
// Downstream: strings trimming and suffix handling.
func CollapseSSID(call string) string {
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
// Upstream: CollapseSSID.
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
