package telnet

import (
	"strings"
	"testing"
	"time"
)

func TestApplyTemplateTokens_ReplacesAll(t *testing.T) {
	now := time.Date(2026, time.January, 6, 20, 12, 18, 0, time.UTC)
	start := now.Add(-(3*24*time.Hour + 4*time.Hour + 18*time.Minute + 22*time.Second))
	lastLogin := time.Date(2026, time.January, 5, 18, 0, 11, 0, time.UTC)
	data := templateData{
		now:       now,
		startTime: start,
		userCount: 12,
		callsign:  "N2WQ-2",
		cluster:   "LZ13ZZ",
		lastLogin: lastLogin,
		lastIP:    "203.0.113.9",
	}

	msg := "\nLast login: <LAST_LOGIN> from <LAST_IP>\n\n<CALL> de <CLUSTER> <DATETIME>> \nUptime: <UPTIME> | Users: <USER_COUNT>\n"
	out := applyTemplateTokens(msg, data)

	for _, want := range []string{
		"Last login: 05-Jan-2026 18:00:11 UTC from 203.0.113.9",
		"N2WQ-2 de LZ13ZZ 06-Jan-2026 20:12:18 UTC>",
		"Uptime: 3d 04:18:22 | Users: 12",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestApplyTemplateTokens_FirstLoginFallbacks(t *testing.T) {
	now := time.Date(2026, time.January, 6, 20, 12, 18, 0, time.UTC)
	data := templateData{
		now:       now,
		startTime: now,
		userCount: 0,
		callsign:  "N0CALL",
		cluster:   "NODE",
	}
	out := applyTemplateTokens("Last: <LAST_LOGIN> IP: <LAST_IP> Count: <USER_COUNT>", data)
	for _, want := range []string{"(first login)", "(unknown)", "Count: 0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}
