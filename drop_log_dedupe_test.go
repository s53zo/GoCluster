package main

import (
	"strings"
	"testing"
	"time"
)

func TestDropLogDedupeKey(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
		ok   bool
	}{
		{
			name: "cty unknown",
			line: "CTY drop: unknown DX K1ABC at 14074.0 kHz (source=RBN total=10)",
			want: "cty:unknown:DX:K1ABC",
			ok:   true,
		},
		{
			name: "cty invalid",
			line: "CTY drop: invalid DE ABC1 (>=3 leading letters) at 7074.0 kHz (source=PSK total=5)",
			want: "cty:invalid:DE:ABC1",
			ok:   true,
		},
		{
			name: "unlicensed ui color",
			line: "Unlicensed US DE [red]K1ABC[-] dropped from PSKREPORTER FT8 @ 14074.0 kHz",
			want: "unlicensed:DE:K1ABC",
			ok:   true,
		},
		{
			name: "other line",
			line: "PC61 drop: reason=invalid_dx de=W1AAA dx=XXX band=20m freq=14074.0 source=peerA",
			ok:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := dropLogDedupeKey(tc.line)
			if ok != tc.ok {
				t.Fatalf("expected ok=%v, got %v (key=%q)", tc.ok, ok, got)
			}
			if tc.ok && got != tc.want {
				t.Fatalf("expected key %q, got %q", tc.want, got)
			}
		})
	}
}

func TestDropLogDeduperSuppressesWithinWindow(t *testing.T) {
	now := time.Date(2026, 2, 6, 0, 0, 0, 0, time.UTC)
	d := newDropLogDeduper(10*time.Second, 16)
	if d == nil {
		t.Fatal("expected deduper")
	}
	d.now = func() time.Time { return now }

	line := "Unlicensed US DE K1ABC dropped from PSKREPORTER FT8 @ 14074.0 kHz"
	out, ok := d.Process(line)
	if !ok || out != line {
		t.Fatalf("expected first line to pass through, got ok=%v out=%q", ok, out)
	}

	out, ok = d.Process(line)
	if ok || out != "" {
		t.Fatalf("expected second line to be suppressed, got ok=%v out=%q", ok, out)
	}

	now = now.Add(11 * time.Second)
	out, ok = d.Process(line)
	if !ok {
		t.Fatalf("expected line after window, got suppressed")
	}
	if !strings.Contains(out, "suppressed=1") {
		t.Fatalf("expected suppression summary, got %q", out)
	}
}

func TestDropLogDeduperEvictsOldestKey(t *testing.T) {
	now := time.Date(2026, 2, 6, 0, 0, 0, 0, time.UTC)
	d := newDropLogDeduper(30*time.Second, 2)
	if d == nil {
		t.Fatal("expected deduper")
	}
	d.now = func() time.Time { return now }

	line1 := "CTY drop: unknown DX K1AAA at 14000.0 kHz (source=RBN total=1)"
	line2 := "CTY drop: unknown DX K1BBB at 14000.0 kHz (source=RBN total=2)"
	line3 := "CTY drop: unknown DX K1CCC at 14000.0 kHz (source=RBN total=3)"

	if _, ok := d.Process(line1); !ok {
		t.Fatal("expected line1 to pass")
	}
	now = now.Add(time.Second)
	if _, ok := d.Process(line2); !ok {
		t.Fatal("expected line2 to pass")
	}
	now = now.Add(time.Second)
	if _, ok := d.Process(line3); !ok {
		t.Fatal("expected line3 to pass")
	}

	if len(d.entries) != 2 {
		t.Fatalf("expected 2 entries after eviction, got %d", len(d.entries))
	}
	if _, ok := d.entries["cty:unknown:DX:K1AAA"]; ok {
		t.Fatalf("expected oldest key K1AAA to be evicted")
	}
}
