package spot

import "strings"

// normalizeUpperASCIITrim trims whitespace and uppercases ASCII letters.
// It returns the original string when already trimmed/uppercased.
// Non-ASCII input falls back to strings.ToUpper to preserve Unicode semantics.
func normalizeUpperASCIITrim(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	needsUpper := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			return strings.ToUpper(s)
		}
		if c >= 'a' && c <= 'z' {
			needsUpper = true
		}
	}
	if !needsUpper {
		return s
	}
	buf := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		buf[i] = c
	}
	return string(buf)
}

// canonicalPSKModeUpper assumes the input is already uppercased and trimmed.
// It avoids re-normalizing when callers already normalized the mode token.
func canonicalPSKModeUpper(upper string) (canonical string, variant string, isPSK bool) {
	if upper == "" {
		return "", "", false
	}
	canonical, ok := pskVariantMap[upper]
	if !ok {
		return upper, upper, false
	}
	return canonical, upper, true
}
