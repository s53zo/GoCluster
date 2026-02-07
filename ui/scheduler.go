package ui

import (
	"math/bits"
	"sync"
	"time"

	"github.com/rivo/tview"
)

type schedulerSlot struct {
	fn func()
}

// frameScheduler coalesces UI updates and caps draw rate.
// It uses stable slots with dirty bitsets to avoid per-frame map churn.
type frameScheduler struct {
	app          *tview.Application
	mu           sync.Mutex
	idToSlot     map[string]int
	slots        []schedulerSlot
	dirtyBits    []uint64
	batchScratch []func()
	quit         chan struct{}
	done         chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup
	frameTime    time.Duration
	drainTimeout time.Duration
	observeDelay func(time.Duration)
	queueDraw    func(func())
}

func newFrameScheduler(app *tview.Application, targetFPS int, drainTimeout time.Duration, observeDelay func(time.Duration)) *frameScheduler {
	if targetFPS <= 0 {
		targetFPS = 30
	}
	if drainTimeout <= 0 {
		drainTimeout = 100 * time.Millisecond
	}
	f := &frameScheduler{
		app:          app,
		idToSlot:     make(map[string]int, 16),
		slots:        make([]schedulerSlot, 0, 16),
		dirtyBits:    make([]uint64, 0, 1),
		batchScratch: make([]func(), 0, 16),
		quit:         make(chan struct{}),
		done:         make(chan struct{}),
		frameTime:    time.Second / time.Duration(targetFPS),
		drainTimeout: drainTimeout,
		observeDelay: observeDelay,
	}
	if app != nil {
		f.queueDraw = func(fn func()) { app.QueueUpdateDraw(fn) }
	} else {
		// Test fallback.
		f.queueDraw = func(fn func()) {
			if fn != nil {
				fn()
			}
		}
	}
	return f
}

func (f *frameScheduler) Start() {
	if f == nil {
		return
	}
	f.wg.Add(1)
	go f.run()
}

func (f *frameScheduler) Stop() {
	if f == nil {
		return
	}
	f.stopOnce.Do(func() {
		close(f.quit)
	})
	select {
	case <-f.done:
	case <-time.After(f.drainTimeout):
	}
}

func (f *frameScheduler) Schedule(id string, fn func()) {
	if f == nil || id == "" || fn == nil {
		return
	}
	f.mu.Lock()
	idx, ok := f.idToSlot[id]
	if !ok {
		idx = len(f.slots)
		f.idToSlot[id] = idx
		f.slots = append(f.slots, schedulerSlot{})
		word := idx / 64
		if word >= len(f.dirtyBits) {
			growth := word + 1 - len(f.dirtyBits)
			f.dirtyBits = append(f.dirtyBits, make([]uint64, growth)...)
		}
	}
	f.slots[idx].fn = fn
	word := idx / 64
	bit := uint(idx % 64)
	f.dirtyBits[word] |= uint64(1) << bit
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
			f.flush()
			return
		}
	}
}

func (f *frameScheduler) flush() {
	if f == nil || f.queueDraw == nil {
		return
	}
	batch := f.collectBatch()
	if len(batch) == 0 {
		return
	}

	queuedAt := time.Now()
	f.queueDraw(func() {
		for _, fn := range batch {
			fn()
		}
		if f.observeDelay != nil {
			f.observeDelay(time.Since(queuedAt))
		}
	})
}

func (f *frameScheduler) collectBatch() []func() {
	f.mu.Lock()
	defer f.mu.Unlock()

	pending := false
	for _, word := range f.dirtyBits {
		if word != 0 {
			pending = true
			break
		}
	}
	if !pending {
		return nil
	}

	batch := f.batchScratch[:0]
	for wordIdx, word := range f.dirtyBits {
		for word != 0 {
			bit := bits.TrailingZeros64(word)
			slot := wordIdx*64 + bit
			if slot < len(f.slots) {
				if fn := f.slots[slot].fn; fn != nil {
					batch = append(batch, fn)
				}
			}
			word &= word - 1
		}
		f.dirtyBits[wordIdx] = 0
	}
	f.batchScratch = batch
	return batch
}
