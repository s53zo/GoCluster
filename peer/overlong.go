package peer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	overlongOnce sync.Once
	overlongCh   chan overlongSample
)

type overlongSample struct {
	path    string
	host    string
	preview string
	length  int
	ts      time.Time
}

// appendOverlongSample writes a preview of an overlong line to a log file for diagnostics.
// It truncates the preview to keep the log bounded and omits empty lines.
func appendOverlongSample(path, host, preview string, length int) {
	preview = strings.TrimSpace(preview)
	if preview == "" {
		return
	}
	const maxPreview = 512
	if len(preview) > maxPreview {
		preview = preview[:maxPreview]
	}
	overlongOnce.Do(func() {
		overlongCh = make(chan overlongSample, 256)
		go overlongWorker()
	})
	if overlongCh == nil {
		return
	}
	sample := overlongSample{
		path:    path,
		host:    strings.TrimSpace(host),
		preview: preview,
		length:  length,
		ts:      time.Now().UTC(),
	}
	// Best-effort: drop if the queue is full so the read loop never blocks.
	select {
	case overlongCh <- sample:
	default:
	}
}

func overlongWorker() {
	for sample := range overlongCh {
		if sample.preview == "" {
			continue
		}
		if dir := filepath.Dir(sample.path); dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0o755)
		}
		f, err := os.OpenFile(sample.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}
		ts := sample.ts.Format(time.RFC3339)
		line := fmt.Sprintf("%s host=%s len=%d preview=%s\n", ts, sample.host, sample.length, sample.preview)
		_, _ = f.WriteString(line)
		_ = f.Close()
	}
}
