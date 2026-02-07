package ui

import (
	"strconv"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func BenchmarkVirtualLogViewDraw(b *testing.B) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		b.Fatalf("init simulation screen: %v", err)
	}
	defer screen.Fini()

	view := newVirtualLogView("Events", 256, false)
	view.SetRect(0, 0, 120, 20)
	for i := 0; i < 256; i++ {
		view.Append("seed line " + strconv.Itoa(i))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		view.Append("event " + strconv.Itoa(i))
		view.Draw(screen)
	}
}

func BenchmarkFrameSchedulerFlush(b *testing.B) {
	f := newFrameScheduler(nil, 60, 50*time.Millisecond, nil)
	ids := []string{"snapshot", "network", "validation", "unlicensed", "corrected", "harmonics", "events"}
	callbacks := make([]func(), len(ids))
	for i := range callbacks {
		callbacks[i] = func() {}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j, id := range ids {
			f.Schedule(id, callbacks[j])
		}
		f.flush()
	}
}
