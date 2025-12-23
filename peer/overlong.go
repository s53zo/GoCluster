package peer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var overlongMu sync.Mutex

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
	overlongMu.Lock()
	defer overlongMu.Unlock()

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s host=%s len=%d preview=%s\n", ts, strings.TrimSpace(host), length, preview)
	_, _ = f.WriteString(line)
}
