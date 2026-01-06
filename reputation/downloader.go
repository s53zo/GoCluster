package reputation

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
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
	if err := g.LoadStore(); err != nil {
		return err
	}
	log.Printf("IPinfo store loaded from %s", g.cfg.ipinfoPebblePath)
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
	importTimeout := cfg.ipinfoImportTimeout
	if importTimeout <= 0 {
		importTimeout = 10 * time.Minute
	}
	importCtx, importCancel := context.WithTimeout(ctx, importTimeout)
	defer importCancel()

	if err := extractGzip(importCtx, gzPath, csvPath); err != nil {
		return err
	}
	dbPath, err := buildIPInfoPebble(importCtx, csvPath, cfg.ipinfoPebblePath, cfg.ipinfoPebbleCompact)
	if err != nil {
		return err
	}
	if err := updateIPInfoPebbleCurrent(cfg.ipinfoPebblePath, dbPath); err != nil {
		return err
	}
	if cfg.ipinfoDeleteCSV {
		if err := os.Remove(csvPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if !cfg.ipinfoKeepGzip {
		if err := os.Remove(gzPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if cfg.ipinfoPebbleCleanup {
		if err := cleanupIPInfoPebbleDirs(cfg.ipinfoPebblePath, dbPath); err != nil {
			log.Printf("Warning: ipinfo pebble cleanup failed: %v", err)
		}
	}
	log.Printf("IPinfo pebble store updated at %s", dbPath)
	return nil
}

func runCurl(ctx context.Context, url, dest string) error {
	cmd := exec.CommandContext(ctx, "curl", "-L", url, "-o", dest)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	start := time.Now()
	if err := cmd.Run(); err != nil {
		elapsed := time.Since(start)
		return fmt.Errorf("curl failed after %s: %w: %s", elapsed, err, strings.TrimSpace(stderr.String()))
	}
	elapsed := time.Since(start)
	info, err := os.Stat(dest)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("curl produced empty file: %s", dest)
	}
	log.Printf("IPinfo download %s completed in %s (%d bytes)", dest, elapsed, info.Size())
	return nil
}

func extractGzip(ctx context.Context, src, dest string) error {
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

	buf := make([]byte, 128*1024)
	for {
		if err := ctx.Err(); err != nil {
			tmp.Close()
			return err
		}
		n, err := reader.Read(buf)
		if n > 0 {
			if _, werr := tmp.Write(buf[:n]); werr != nil {
				tmp.Close()
				return werr
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			tmp.Close()
			return err
		}
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
