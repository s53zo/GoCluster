package main

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"dxcluster/config"
	"dxcluster/spot"
)

func TestCloneSpotForPeerPublishAddsModeWhenCommentEmpty(t *testing.T) {
	src := spot.NewSpot("K1ABC", "W1XYZ", 7074.0, "")
	src.Mode = "FT8"
	src.Comment = ""

	peerSpot := cloneSpotForPeerPublish(src)
	if peerSpot == nil {
		t.Fatalf("expected peer spot, got nil")
	}
	if peerSpot == src {
		t.Fatalf("expected a cloned spot when adding inferred mode to comment")
	}
	if peerSpot.Comment != "FT8" {
		t.Fatalf("expected comment to carry inferred mode, got %q", peerSpot.Comment)
	}
	if src.Comment != "" {
		t.Fatalf("expected original comment to remain empty, got %q", src.Comment)
	}
}

func TestCloneSpotForPeerPublishPassthroughWhenCommentPresent(t *testing.T) {
	src := spot.NewSpot("K1ABC", "W1XYZ", 7074.0, "")
	src.Mode = "FT8"
	src.Comment = "cq test"

	peerSpot := cloneSpotForPeerPublish(src)
	if peerSpot != src {
		t.Fatalf("expected passthrough when comment present")
	}
}

// Purpose: Verify gridDBCheckOnMissEnabled defaults to true.
// Key aspects: Clears env override before test.
// Upstream: go test execution.
// Downstream: gridDBCheckOnMissEnabled.
func TestGridDBCheckOnMissEnabled_DefaultsTrue(t *testing.T) {
	t.Setenv(envGridDBCheckOnMiss, "")

	got, source := gridDBCheckOnMissEnabled(&config.Config{})
	if !got {
		t.Fatalf("expected default grid DB check to be enabled, got %v (source=%s)", got, source)
	}
}

// Purpose: Verify config can disable grid DB check.
// Key aspects: Uses explicit config override.
// Upstream: go test execution.
// Downstream: gridDBCheckOnMissEnabled.
func TestGridDBCheckOnMissEnabled_ConfigFalse(t *testing.T) {
	t.Setenv(envGridDBCheckOnMiss, "")
	cfg := &config.Config{GridDBCheckOnMiss: boolPtr(false)}

	got, source := gridDBCheckOnMissEnabled(cfg)
	if got {
		t.Fatalf("expected grid DB check to be disabled by config, got %v (source=%s)", got, source)
	}
}

// Purpose: Verify env override takes precedence over config.
// Key aspects: Sets env to false and checks source.
// Upstream: go test execution.
// Downstream: gridDBCheckOnMissEnabled.
func TestGridDBCheckOnMissEnabled_EnvOverridesConfig(t *testing.T) {
	cfg := &config.Config{GridDBCheckOnMiss: boolPtr(true)}
	t.Setenv(envGridDBCheckOnMiss, "false")

	got, source := gridDBCheckOnMissEnabled(cfg)
	if got {
		t.Fatalf("expected env override to disable grid DB check, got %v (source=%s)", got, source)
	}
	if source != envGridDBCheckOnMiss {
		t.Fatalf("expected source=%q, got %q", envGridDBCheckOnMiss, source)
	}
}

// Purpose: Verify invalid env override is ignored.
// Key aspects: Uses non-boolean env value.
// Upstream: go test execution.
// Downstream: gridDBCheckOnMissEnabled.
func TestGridDBCheckOnMissEnabled_InvalidEnvIgnored(t *testing.T) {
	cfg := &config.Config{GridDBCheckOnMiss: boolPtr(false)}
	t.Setenv(envGridDBCheckOnMiss, "notabool")

	got, _ := gridDBCheckOnMissEnabled(cfg)
	if got {
		t.Fatalf("expected invalid env override to be ignored, got %v", got)
	}
}

// Purpose: Verify LoadedFrom is reported as the config source.
// Key aspects: Leaves env unset to test config source reporting.
// Upstream: go test execution.
// Downstream: gridDBCheckOnMissEnabled.
func TestGridDBCheckOnMissEnabled_UsesLoadedFromWhenSet(t *testing.T) {
	cfg := &config.Config{GridDBCheckOnMiss: boolPtr(true), LoadedFrom: "data/config"}
	t.Setenv(envGridDBCheckOnMiss, "")

	_, source := gridDBCheckOnMissEnabled(cfg)
	if source != "data/config" {
		t.Fatalf("expected source=data/config, got %s", source)
	}
}

// Purpose: Ensure SCP-known calls promote '?' confidence to 'S' after correction.
// Key aspects: Applies the known-call floor only when confidence is '?'.
// Upstream: go test execution.
// Downstream: applyKnownCallFloor and spot.LoadKnownCallsigns.
func TestApplyKnownCallFloorPromotesKnownDX(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known.txt")
	if err := os.WriteFile(path, []byte("K1KI\n"), 0o644); err != nil {
		t.Fatalf("write known calls: %v", err)
	}
	known, err := spot.LoadKnownCallsigns(path)
	if err != nil {
		t.Fatalf("load known calls: %v", err)
	}
	var knownPtr atomic.Pointer[spot.KnownCallsigns]
	knownPtr.Store(known)

	s := spot.NewSpot("K1KI", "W2TT", 1831.3, "CW")
	s.Confidence = "?"

	if !applyKnownCallFloor(s, &knownPtr) {
		t.Fatalf("expected known-call floor to mark confidence")
	}
	if s.Confidence != "S" {
		t.Fatalf("expected confidence S, got %q", s.Confidence)
	}
}

// Purpose: Ensure SCP floor does not override explicit P/V/C confidence.
// Key aspects: Keeps existing confidence when it is not '?'.
// Upstream: go test execution.
// Downstream: applyKnownCallFloor.
func TestApplyKnownCallFloorKeepsNonUnknownConfidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known.txt")
	if err := os.WriteFile(path, []byte("K1KI\n"), 0o644); err != nil {
		t.Fatalf("write known calls: %v", err)
	}
	known, err := spot.LoadKnownCallsigns(path)
	if err != nil {
		t.Fatalf("load known calls: %v", err)
	}
	var knownPtr atomic.Pointer[spot.KnownCallsigns]
	knownPtr.Store(known)

	s := spot.NewSpot("K1KI", "W2TT", 1831.3, "CW")
	s.Confidence = "P"

	if applyKnownCallFloor(s, &knownPtr) {
		t.Fatalf("did not expect known-call floor to override P")
	}
	if s.Confidence != "P" {
		t.Fatalf("expected confidence P, got %q", s.Confidence)
	}
}

// Purpose: Ensure SCP floor ignores modes without confidence glyphs.
// Key aspects: FT8 remains without S promotion even when known.
// Upstream: go test execution.
// Downstream: applyKnownCallFloor.
func TestApplyKnownCallFloorSkipsUnsupportedMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known.txt")
	if err := os.WriteFile(path, []byte("K1KI\n"), 0o644); err != nil {
		t.Fatalf("write known calls: %v", err)
	}
	known, err := spot.LoadKnownCallsigns(path)
	if err != nil {
		t.Fatalf("load known calls: %v", err)
	}
	var knownPtr atomic.Pointer[spot.KnownCallsigns]
	knownPtr.Store(known)

	s := spot.NewSpot("K1KI", "W2TT", 14074.0, "FT8")
	s.Confidence = "?"

	if applyKnownCallFloor(s, &knownPtr) {
		t.Fatalf("did not expect known-call floor to apply to FT8")
	}
	if s.Confidence != "?" {
		t.Fatalf("expected confidence ?, got %q", s.Confidence)
	}
}

// Purpose: Validate SSID collapsing rules for broadcast formatting.
// Key aspects: Covers numeric, non-numeric, and composite suffixes.
// Upstream: go test execution.
// Downstream: collapseSSIDForBroadcast.
func TestCollapseSSIDForBroadcast(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"N2WQ-1-#", "N2WQ-#"},
		{"N2WQ-#", "N2WQ-#"},
		{"N2WQ-1", "N2WQ"},
		{"N2WQ-12", "N2WQ"},
		{"N2WQ-TEST", "N2WQ-TEST"},
		{"N2WQ-1/P", "N2WQ-1/P"},
		{"", ""},
	}

	for _, tc := range cases {
		got := collapseSSIDForBroadcast(tc.input)
		if got != tc.want {
			t.Fatalf("collapseSSIDForBroadcast(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// Purpose: Helper to allocate a bool pointer.
// Key aspects: Avoids inline address-of literals.
// Upstream: grid DB check tests in this file.
// Downstream: None (returns pointer).
func boolPtr(v bool) *bool {
	b := v
	return &b
}
