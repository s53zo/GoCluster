package main

import (
	"context"
	"io"
	"log"
	"testing"
	"time"
)

type stubReportRunner struct {
	started chan propReportJob
	block   chan struct{}
	err     error
}

func (s *stubReportRunner) Run(ctx context.Context, job propReportJob) error {
	if s.started != nil {
		s.started <- job
	}
	if s.block != nil {
		select {
		case <-s.block:
			return s.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.err
}

func TestPropReportSchedulerSkipsDuplicate(t *testing.T) {
	runner := &stubReportRunner{
		started: make(chan propReportJob, 1),
		block:   make(chan struct{}),
	}
	logger := log.New(io.Discard, "", 0)
	scheduler := newPropReportScheduler(true, runner, logger, 250*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler.Start(ctx)

	jobDate := time.Date(2026, time.January, 23, 0, 0, 0, 0, time.UTC)
	if !scheduler.Enqueue(propReportJob{Date: jobDate, LogPath: "data/logs/23-Jan-2026.log"}) {
		t.Fatalf("expected initial enqueue to succeed")
	}

	select {
	case <-runner.started:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("runner did not start")
	}

	if scheduler.Enqueue(propReportJob{Date: jobDate, LogPath: "data/logs/23-Jan-2026.log"}) {
		t.Fatalf("expected duplicate enqueue to be skipped")
	}

	close(runner.block)
	cancel()
	scheduler.Wait()
}

func TestPropReportSchedulerDropsWhenQueueFull(t *testing.T) {
	runner := &stubReportRunner{}
	logger := log.New(io.Discard, "", 0)
	scheduler := newPropReportScheduler(true, runner, logger, 250*time.Millisecond)

	job1 := propReportJob{Date: time.Date(2026, time.January, 24, 0, 0, 0, 0, time.UTC), LogPath: "data/logs/24-Jan-2026.log"}
	job2 := propReportJob{Date: time.Date(2026, time.January, 25, 0, 0, 0, 0, time.UTC), LogPath: "data/logs/25-Jan-2026.log"}

	if !scheduler.Enqueue(job1) {
		t.Fatalf("expected first enqueue to succeed")
	}
	if scheduler.Enqueue(job2) {
		t.Fatalf("expected second enqueue to drop when queue is full")
	}
}
