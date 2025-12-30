package spot

import (
	"testing"
	"time"
)

func TestCallCooldownBlocksAndExpires(t *testing.T) {
	now := time.Now()
	cc := NewCallCooldown(CallCooldownConfig{
		Enabled:      true,
		MinReporters: 3,
		Duration:     2 * time.Minute,
		TTL:          5 * time.Minute,
		BinHz:        500,
		MaxReporters: 8,
	})
	reporters := map[string]struct{}{
		"AA1ZZ": {},
		"BB2YY": {},
		"CC3XX": {},
	}

	cc.Record("K1ABC", 7000000, reporters, 3, 45*time.Second, now)

	block, count := cc.ShouldBlock("K1ABC", 7000000, 3, 45*time.Second, 3, 40, 5, 70, now.Add(30*time.Second))
	if !block {
		t.Fatalf("expected cooldown to block correction")
	}
	if count != 3 {
		t.Fatalf("expected reporter count 3, got %d", count)
	}

	block, _ = cc.ShouldBlock("K1ABC", 7000000, 3, 45*time.Second, 3, 40, 5, 70, now.Add(6*time.Minute))
	if block {
		t.Fatalf("expected cooldown to expire after TTL")
	}
}

func TestCallCooldownBypass(t *testing.T) {
	now := time.Now()
	cc := NewCallCooldown(CallCooldownConfig{
		Enabled:          true,
		MinReporters:     3,
		Duration:         3 * time.Minute,
		TTL:              10 * time.Minute,
		BinHz:            500,
		MaxReporters:     8,
		BypassAdvantage:  2,
		BypassConfidence: 10,
	})
	reporters := map[string]struct{}{
		"AA1ZZ": {},
		"BB2YY": {},
		"CC3XX": {},
	}

	cc.Record("K1ABC", 7000000, reporters, 3, 45*time.Second, now)

	block, _ := cc.ShouldBlock("K1ABC", 7000000, 3, 45*time.Second, 3, 30, 6, 70, now.Add(30*time.Second))
	if block {
		t.Fatalf("expected strong winner to bypass cooldown")
	}
}

func TestCallCooldownBelowMinReporters(t *testing.T) {
	now := time.Now()
	cc := NewCallCooldown(CallCooldownConfig{
		Enabled:      true,
		MinReporters: 3,
		Duration:     time.Minute,
		TTL:          5 * time.Minute,
		BinHz:        500,
		MaxReporters: 8,
	})
	reporters := map[string]struct{}{
		"AA1ZZ": {},
		"BB2YY": {},
	}
	cc.Record("K1ABC", 7000000, reporters, 3, 45*time.Second, now)
	block, _ := cc.ShouldBlock("K1ABC", 7000000, 3, 45*time.Second, 2, 40, 5, 70, now.Add(30*time.Second))
	if block {
		t.Fatalf("expected no block when below min reporters")
	}
}

func TestCallCooldownBinIsolation(t *testing.T) {
	now := time.Now()
	cc := NewCallCooldown(CallCooldownConfig{
		Enabled:      true,
		MinReporters: 2,
		Duration:     time.Minute,
		TTL:          5 * time.Minute,
		BinHz:        500,
		MaxReporters: 8,
	})
	reporters := map[string]struct{}{
		"AA1ZZ": {},
		"BB2YY": {},
	}
	cc.Record("K1ABC", 7000000, reporters, 2, 45*time.Second, now)
	block, _ := cc.ShouldBlock("K1ABC", 7000500, 2, 45*time.Second, 2, 40, 3, 60, now.Add(30*time.Second))
	if block {
		t.Fatalf("expected no block in different bin")
	}
}

func TestCallCooldownRecencyPrune(t *testing.T) {
	now := time.Now()
	cc := NewCallCooldown(CallCooldownConfig{
		Enabled:      true,
		MinReporters: 2,
		Duration:     time.Minute,
		TTL:          5 * time.Minute,
		BinHz:        500,
		MaxReporters: 8,
	})
	reporters := map[string]struct{}{
		"AA1ZZ": {},
		"BB2YY": {},
	}
	cc.Record("K1ABC", 7000000, reporters, 2, 45*time.Second, now.Add(-2*time.Minute))
	block, count := cc.ShouldBlock("K1ABC", 7000000, 2, 45*time.Second, 2, 40, 3, 60, now)
	if block || count != 0 {
		t.Fatalf("expected reporters to prune out of window; block=%v count=%d", block, count)
	}
}

func TestCallCooldownMaxReportersEviction(t *testing.T) {
	now := time.Now()
	cc := NewCallCooldown(CallCooldownConfig{
		Enabled:      true,
		MinReporters: 2,
		Duration:     time.Minute,
		TTL:          5 * time.Minute,
		BinHz:        500,
		MaxReporters: 2,
	})
	reporters := map[string]struct{}{
		"AA1ZZ": {},
		"BB2YY": {},
		"CC3XX": {},
	}
	cc.Record("K1ABC", 7000000, reporters, 2, 45*time.Second, now)
	if len(cc.entries) != 1 {
		t.Fatalf("expected one entry")
	}
	entry := cc.entries[callCooldownKey{call: "K1ABC", bin: cc.binFor(7000000)}]
	if entry == nil {
		t.Fatalf("missing entry")
	}
	if len(entry.reporters) != 2 {
		t.Fatalf("expected reporters capped at 2, got %d", len(entry.reporters))
	}
}

func TestCallCooldownBypassAND(t *testing.T) {
	now := time.Now()
	cc := NewCallCooldown(CallCooldownConfig{
		Enabled:          true,
		MinReporters:     3,
		Duration:         3 * time.Minute,
		TTL:              10 * time.Minute,
		BinHz:            500,
		MaxReporters:     8,
		BypassAdvantage:  2,
		BypassConfidence: 10,
	})
	reporters := map[string]struct{}{
		"AA1ZZ": {},
		"BB2YY": {},
		"CC3XX": {},
	}
	cc.Record("K1ABC", 7000000, reporters, 3, 45*time.Second, now)

	// Advantage only should not bypass (needs both).
	block, _ := cc.ShouldBlock("K1ABC", 7000000, 3, 45*time.Second, 3, 30, 5, 35, now.Add(30*time.Second))
	if !block {
		t.Fatalf("expected block when only advantage condition is met")
	}
	// Both should bypass.
	block, _ = cc.ShouldBlock("K1ABC", 7000000, 3, 45*time.Second, 3, 30, 6, 45, now.Add(30*time.Second))
	if block {
		t.Fatalf("expected bypass when both advantage and confidence are met")
	}
}
