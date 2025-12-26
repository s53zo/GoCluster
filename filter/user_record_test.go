package filter

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUpdateRecentIPs(t *testing.T) {
	recent := []string{"1.1.1.1", "2.2.2.2"}
	recent = UpdateRecentIPs(recent, "3.3.3.3")
	want := []string{"3.3.3.3", "1.1.1.1", "2.2.2.2"}
	if !reflect.DeepEqual(recent, want) {
		t.Fatalf("expected %v, got %v", want, recent)
	}

	recent = UpdateRecentIPs(recent, "1.1.1.1")
	want = []string{"1.1.1.1", "3.3.3.3", "2.2.2.2"}
	if !reflect.DeepEqual(recent, want) {
		t.Fatalf("expected %v, got %v", want, recent)
	}

	recent = UpdateRecentIPs(recent, "4.4.4.4")
	recent = UpdateRecentIPs(recent, "5.5.5.5")
	recent = UpdateRecentIPs(recent, "6.6.6.6")
	want = []string{"6.6.6.6", "5.5.5.5", "4.4.4.4", "1.1.1.1", "3.3.3.3"}
	if !reflect.DeepEqual(recent, want) {
		t.Fatalf("expected %v, got %v", want, recent)
	}
}

func TestMergeRecentIPs(t *testing.T) {
	primary := []string{"198.51.100.1", "203.0.113.5"}
	fallback := []string{"203.0.113.5", "192.0.2.9", "198.51.100.1", "203.0.113.6"}
	merged := MergeRecentIPs(primary, fallback)
	want := []string{"198.51.100.1", "203.0.113.5", "192.0.2.9", "203.0.113.6"}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("expected %v, got %v", want, merged)
	}
}

func TestTouchUserRecordIP(t *testing.T) {
	tmp := t.TempDir()
	orig := UserDataDir
	UserDataDir = tmp
	t.Cleanup(func() { UserDataDir = orig })

	record, created, err := TouchUserRecordIP("k3to", "203.0.113.9")
	if err != nil {
		t.Fatalf("TouchUserRecordIP failed: %v", err)
	}
	if !created {
		t.Fatalf("expected new record to be created")
	}
	if len(record.RecentIPs) != 1 || record.RecentIPs[0] != "203.0.113.9" {
		t.Fatalf("unexpected recent IPs: %v", record.RecentIPs)
	}

	record, created, err = TouchUserRecordIP("k3to", "198.51.100.10")
	if err != nil {
		t.Fatalf("TouchUserRecordIP failed: %v", err)
	}
	if created {
		t.Fatalf("expected existing record to be reused")
	}
	want := []string{"198.51.100.10", "203.0.113.9"}
	if !reflect.DeepEqual(record.RecentIPs, want) {
		t.Fatalf("expected %v, got %v", want, record.RecentIPs)
	}
}

func TestUserRecordRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	orig := UserDataDir
	UserDataDir = tmp
	t.Cleanup(func() { UserDataDir = orig })

	f := NewFilter()
	f.SetMode("CW", true)
	record := &UserRecord{
		Filter:    *f,
		RecentIPs: []string{"203.0.113.10", "198.51.100.42"},
	}
	if err := SaveUserRecord("k3to", record); err != nil {
		t.Fatalf("SaveUserRecord failed: %v", err)
	}

	loaded, err := LoadUserRecord("k3to")
	if err != nil {
		t.Fatalf("LoadUserRecord failed: %v", err)
	}
	if !loaded.Modes["CW"] {
		t.Fatalf("expected CW mode to remain enabled after reload")
	}
	if !reflect.DeepEqual(loaded.RecentIPs, record.RecentIPs) {
		t.Fatalf("expected recent IPs %v, got %v", record.RecentIPs, loaded.RecentIPs)
	}
}

func TestLoadUserRecordLegacyFilter(t *testing.T) {
	tmp := t.TempDir()
	orig := UserDataDir
	UserDataDir = tmp
	t.Cleanup(func() { UserDataDir = orig })

	f := NewFilter()
	f.SetBand("20m", true)
	raw, err := yaml.Marshal(f)
	if err != nil {
		t.Fatalf("yaml marshal failed: %v", err)
	}
	path := filepath.Join(UserDataDir, "LEGACY.yaml")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write legacy filter failed: %v", err)
	}

	record, err := LoadUserRecord("LEGACY")
	if err != nil {
		t.Fatalf("LoadUserRecord failed: %v", err)
	}
	if !record.Bands["20m"] {
		t.Fatalf("expected legacy band filter to load into user record")
	}
}

func TestSaveUserFilterPreservesRecentIPs(t *testing.T) {
	tmp := t.TempDir()
	orig := UserDataDir
	UserDataDir = tmp
	t.Cleanup(func() { UserDataDir = orig })

	base := NewFilter()
	base.SetMode("CW", true)
	record := &UserRecord{
		Filter:    *base,
		RecentIPs: []string{"192.0.2.1"},
	}
	if err := SaveUserRecord("N0CALL", record); err != nil {
		t.Fatalf("SaveUserRecord failed: %v", err)
	}

	updated := NewFilter()
	updated.SetMode("USB", true)
	if err := SaveUserFilter("N0CALL", updated); err != nil {
		t.Fatalf("SaveUserFilter failed: %v", err)
	}

	loaded, err := LoadUserRecord("N0CALL")
	if err != nil {
		t.Fatalf("LoadUserRecord failed: %v", err)
	}
	if !reflect.DeepEqual(loaded.RecentIPs, record.RecentIPs) {
		t.Fatalf("expected recent IPs %v, got %v", record.RecentIPs, loaded.RecentIPs)
	}
	if !loaded.Modes["USB"] {
		t.Fatalf("expected updated filter settings to persist")
	}
}
