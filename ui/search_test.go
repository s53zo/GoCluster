package ui

import (
	"context"
	"testing"
	"time"
)

func TestSearchFilterDebounce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	filter := NewSearchFilter(ctx)
	filter.SetQuery("Test", nil)
	time.Sleep(300 * time.Millisecond)
	if got := filter.ActiveQuery(); got != "test" {
		t.Fatalf("expected active query 'test', got %q", got)
	}
}
