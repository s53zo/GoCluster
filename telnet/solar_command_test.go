package telnet

import (
	"testing"
	"time"
)

func TestParseSolarSummaryMinutes(t *testing.T) {
	cases := []struct {
		token string
		want  int
		ok    bool
	}{
		{"OFF", 0, true},
		{"15", 15, true},
		{"30", 30, true},
		{"60", 60, true},
		{"5", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseSolarSummaryMinutes(tc.token)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("token %q: expected (%d,%v) got (%d,%v)", tc.token, tc.want, tc.ok, got, ok)
		}
	}
}

func TestNextSolarSummaryAt(t *testing.T) {
	now := time.Date(2026, 1, 29, 10, 7, 30, 0, time.UTC)
	got := nextSolarSummaryAt(now, 15)
	want := time.Date(2026, 1, 29, 10, 15, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	atBoundary := time.Date(2026, 1, 29, 10, 15, 0, 0, time.UTC)
	got = nextSolarSummaryAt(atBoundary, 15)
	want = time.Date(2026, 1, 29, 10, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestHandleSolarCommand(t *testing.T) {
	server := &Server{}
	client := &Client{}
	resp, handled := server.handleSolarCommand(client, "SET SOLAR 15")
	if !handled {
		t.Fatalf("expected handled")
	}
	if resp == "" {
		t.Fatalf("expected response")
	}
	if got := client.getSolarSummaryMinutes(); got != 15 {
		t.Fatalf("expected 15, got %d", got)
	}
	resp, handled = server.handleSolarCommand(client, "SET SOLAR OFF")
	if !handled {
		t.Fatalf("expected handled")
	}
	if resp == "" {
		t.Fatalf("expected response")
	}
	if got := client.getSolarSummaryMinutes(); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}
