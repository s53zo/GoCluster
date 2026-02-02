package ui

import (
	"sync"
	"time"

	"github.com/rivo/tview"
)

// frameScheduler coalesces UI updates and caps draw rate.
type frameScheduler struct {
	app          *tview.Application
	pending      map[string]func()
	mu           sync.Mutex
	quit         chan struct{}
	done         chan struct{}
	wg           sync.WaitGroup
	frameTime    time.Duration
	drainTimeout time.Duration
	observeDelay func(time.Duration)
}

func newFrameScheduler(app *tview.Application, targetFPS int, drainTimeout time.Duration, observeDelay func(time.Duration)) *frameScheduler {
	if targetFPS <= 0 {
		targetFPS = 30
	}
	if drainTimeout <= 0 {
		drainTimeout = 100 * time.Millisecond
	}
	return &frameScheduler{
		app:          app,
		pending:      make(map[string]func()),
		quit:         make(chan struct{}),
		done:         make(chan struct{}),
		frameTime:    time.Second / time.Duration(targetFPS),
		drainTimeout: drainTimeout,
		observeDelay: observeDelay,
	}
}

func (f *frameScheduler) Start() {
	f.wg.Add(1)
	go f.run()
}

func (f *frameScheduler) Stop() {
	close(f.quit)
	select {
	case <-f.done:
	case <-time.After(f.drainTimeout):
	}
}

func (f *frameScheduler) Schedule(id string, fn func()) {
	if f == nil {
		return
	}
	f.mu.Lock()
	f.pending[id] = fn
	f.mu.Unlock()
}

func (f *frameScheduler) run() {
	defer f.wg.Done()
	defer close(f.done)

	ticker := time.NewTicker(f.frameTime)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			f.flush()
		case <-f.quit:
			f.flushBounded(f.drainTimeout)
			return
		}
	}
}

func (f *frameScheduler) flush() {
	f.flushBounded(0)
}

func (f *frameScheduler) flushBounded(max time.Duration) {
	deadline := time.Time{}
	if max > 0 {
		deadline = time.Now().Add(max)
	}
	for {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return
		}
		f.mu.Lock()
		if len(f.pending) == 0 {
			f.mu.Unlock()
			return
		}
		batch := make([]func(), 0, len(f.pending))
		for _, fn := range f.pending {
			batch = append(batch, fn)
		}
		for key := range f.pending {
			delete(f.pending, key)
		}
		f.mu.Unlock()

		queuedAt := time.Now()
		f.app.QueueUpdateDraw(func() {
			for _, fn := range batch {
				fn()
			}
			if f.observeDelay != nil {
				f.observeDelay(time.Since(queuedAt))
			}
		})
	}
}
