package uls

import (
	"errors"
	"testing"
	"time"
)

func TestReplaceDBWithRetrySucceedsAfterSharingViolation(t *testing.T) {
	orig := replaceDBOnceFn
	defer func() { replaceDBOnceFn = orig }()

	attempts := 0
	replaceDBOnceFn = func(dbPath, tmpPath string) error {
		attempts++
		if attempts < 4 {
			return errors.New("The process cannot access the file because it is being used by another process.")
		}
		return nil
	}

	start := time.Now()
	if err := replaceDBWithRetry("db", "tmp"); err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}
	if attempts != 4 {
		t.Fatalf("expected 4 attempts, got %d", attempts)
	}
	if time.Since(start) < swapRetryInitial {
		t.Fatalf("expected retry delay, got duration %s", time.Since(start))
	}
}

func TestReplaceDBWithRetryStopsOnNonRetryableError(t *testing.T) {
	orig := replaceDBOnceFn
	defer func() { replaceDBOnceFn = orig }()

	attempts := 0
	replaceDBOnceFn = func(dbPath, tmpPath string) error {
		attempts++
		return errors.New("disk full")
	}

	if err := replaceDBWithRetry("db", "tmp"); err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}
