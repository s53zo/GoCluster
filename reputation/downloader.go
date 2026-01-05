package reputation

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (g *Gate) downloadLoop(ctx context.Context) {
	if g == nil || !g.cfg.enabled || !g.cfg.ipinfoDownloadEnabled {
		return
	}
	if err := g.downloadAndLoad(ctx); err != nil {
		log.Printf("Warning: ipinfo download failed: %v", err)
	}
	for {
		delay := nextRefreshDelay(g.cfg.ipinfoRefreshUTC, time.Now().UTC())
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if err := g.downloadAndLoad(ctx); err != nil {
			log.Printf("Warning: scheduled ipinfo download failed: %v", err)
		}
	}
}

func (g *Gate) downloadAndLoad(ctx context.Context) error {
	if g == nil || !g.cfg.ipinfoDownloadEnabled {
		return nil
	}
	if err := downloadSnapshot(ctx, g.cfg); err != nil {
		return err
	}
	if err := g.LoadSnapshot(); err != nil {
		return err
	}
	log.Printf("IPinfo snapshot loaded from %s", g.cfg.ipinfoSnapshotPath)
	return nil
}

func downloadSnapshot(ctx context.Context, cfg gateConfig) error {
	url := strings.TrimSpace(cfg.ipinfoDownloadURL)
	token := strings.TrimSpace(cfg.ipinfoDownloadToken)
	if url == "" || token == "" {
		return fmt.Errorf("ipinfo download url/token missing")
	}
	url = strings.ReplaceAll(url, "$TOKEN", token)
	gzPath := strings.TrimSpace(cfg.ipinfoDownloadPath)
	csvPath := strings.TrimSpace(cfg.ipinfoSnapshotPath)
	if gzPath == "" || csvPath == "" {
		return fmt.Errorf("ipinfo download paths missing")
	}

	if err := os.MkdirAll(filepath.Dir(gzPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(csvPath), 0o755); err != nil {
		return err
	}

	timeout := cfg.ipinfoDownloadTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	downloadCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := runCurl(downloadCtx, url, gzPath); err != nil {
		return err
	}
	if err := extractGzip(gzPath, csvPath); err != nil {
		return err
	}
	return nil
}

func runCurl(ctx context.Context, url, dest string) error {
	cmd := exec.CommandContext(ctx, "curl", "-L", url, "-o", dest)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("curl failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	info, err := os.Stat(dest)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("curl produced empty file: %s", dest)
	}
	return nil
}

func extractGzip(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	reader, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer reader.Close()

	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, "ipinfo-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, reader); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmpPath, dest)
}

func nextRefreshDelay(refreshUTC string, now time.Time) time.Duration {
	hour, minute := parseRefreshHourMinute(refreshUTC)
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(now)
}

func parseRefreshHourMinute(refreshUTC string) (int, int) {
	refreshUTC = strings.TrimSpace(refreshUTC)
	if refreshUTC == "" {
		refreshUTC = "03:00"
	}
	if strings.HasPrefix(strings.ToLower(refreshUTC), "daily@") {
		refreshUTC = refreshUTC[len("daily@"):]
	}
	refreshUTC = strings.TrimSuffix(strings.ToUpper(refreshUTC), "Z")
	if parsed, err := time.Parse("15:04", refreshUTC); err == nil {
		return parsed.Hour(), parsed.Minute()
	}
	return 3, 0
}
