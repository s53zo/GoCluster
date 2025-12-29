package archive

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dxcluster/config"
	"dxcluster/spot"

	_ "modernc.org/sqlite"
)

// Writer persists spots to SQLite asynchronously with per-mode retention.
// It is designed to be removable: the hot path never blocks on the writer,
// and backpressure results in dropped archive writes (logged via counters).
type Writer struct {
	cfg       config.ArchiveConfig
	db        *sql.DB
	queue     chan *spot.Spot
	stop      chan struct{}
	dropCount uint64
}

// Purpose: Initialize archive storage and return a writer instance.
// Key aspects: Creates SQLite DB and schema; sets queue size defaults.
// Upstream: main.go archive setup.
// Downstream: ensureSchema, SQLite open/pragmas.
func NewWriter(cfg config.ArchiveConfig) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, fmt.Errorf("archive: mkdir: %w", err)
	}
	db, err := openArchiveDB(cfg)
	if err != nil {
		return nil, err
	}
	qsize := cfg.QueueSize
	if qsize <= 0 {
		qsize = 10000
	}
	return &Writer{
		cfg:   cfg,
		db:    db,
		queue: make(chan *spot.Spot, qsize),
		stop:  make(chan struct{}),
	}, nil
}

// Purpose: Open the archive DB with durability settings and optional corruption recovery.
// Key aspects: Applies pragmas, validates integrity when enabled, and recreates on corruption.
// Upstream: NewWriter.
// Downstream: applyArchivePragmas, checkArchiveIntegrity, ensureSchema.
func openArchiveDB(cfg config.ArchiveConfig) (*sql.DB, error) {
	const maxAttempts = 2
	for attempt := 0; attempt < maxAttempts; attempt++ {
		db, err := sql.Open("sqlite", cfg.DBPath)
		if err != nil {
			return nil, fmt.Errorf("archive: open db: %w", err)
		}
		// Serialize all archive writes through a single connection to avoid SQLITE_BUSY
		// contention between inserts and cleanup deletes, and to ensure PRAGMA settings
		// (busy_timeout, journal_mode) are consistently applied.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		if err := applyArchivePragmas(db, cfg); err != nil {
			_ = db.Close()
			if cfg.AutoDeleteCorruptDB && isSQLiteCorrupted(err) {
				if err := DropDB(cfg.DBPath); err != nil {
					return nil, fmt.Errorf("archive: delete corrupt db: %w", err)
				}
				log.Printf("archive: pragmas failed (%v), deleted %s", err, cfg.DBPath)
				continue
			}
			return nil, fmt.Errorf("archive: pragmas: %w", err)
		}
		if cfg.AutoDeleteCorruptDB {
			healthy, detail, err := checkArchiveIntegrity(db)
			if err != nil && !isSQLiteCorrupted(err) {
				_ = db.Close()
				return nil, fmt.Errorf("archive: integrity check failed: %w", err)
			}
			if err != nil || !healthy {
				_ = db.Close()
				if err := DropDB(cfg.DBPath); err != nil {
					return nil, fmt.Errorf("archive: delete corrupt db: %w", err)
				}
				if detail == "" && err != nil {
					detail = err.Error()
				}
				if detail == "" {
					detail = "unknown corruption"
				}
				log.Printf("archive: integrity check failed (%s), deleted %s", detail, cfg.DBPath)
				continue
			}
		}
		if err := ensureSchema(db); err != nil {
			if cfg.AutoDeleteCorruptDB && isSQLiteCorrupted(err) {
				_ = db.Close()
				if err := DropDB(cfg.DBPath); err != nil {
					return nil, fmt.Errorf("archive: delete corrupt db: %w", err)
				}
				log.Printf("archive: schema failed (%v), deleted %s", err, cfg.DBPath)
				continue
			}
			_ = db.Close()
			return nil, err
		}
		return db, nil
	}
	return nil, fmt.Errorf("archive: failed to initialize after removing corrupt db")
}

// Purpose: Apply SQLite durability and busy-timeout pragmas for the archive writer.
// Key aspects: WAL mode, configurable synchronous, and bounded busy timeout.
// Upstream: openArchiveDB.
// Downstream: sqlite pragmas.
func applyArchivePragmas(db *sql.DB, cfg config.ArchiveConfig) error {
	syncMode, err := archiveSynchronousPragma(cfg)
	if err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf("pragma journal_mode=WAL; pragma synchronous=%s; pragma busy_timeout=%d", syncMode, cfg.BusyTimeoutMS))
	return err
}

// Purpose: Normalize archive synchronous setting to a SQLite pragma value.
// Key aspects: Defaults to OFF to favor throughput when the archive is disposable.
// Upstream: applyArchivePragmas.
// Downstream: sqlite pragma string.
func archiveSynchronousPragma(cfg config.ArchiveConfig) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Synchronous))
	if mode == "" {
		mode = "off"
	}
	switch mode {
	case "off", "normal", "full", "extra":
		return strings.ToUpper(mode), nil
	default:
		return "", fmt.Errorf("archive: invalid synchronous mode %q", cfg.Synchronous)
	}
}

// Purpose: Run a fast integrity check for obvious SQLite corruption.
// Key aspects: quick_check returns "ok" when healthy; any other row signals corruption.
// Upstream: openArchiveDB.
// Downstream: sqlite pragma query.
func checkArchiveIntegrity(db *sql.DB) (bool, string, error) {
	rows, err := db.Query("pragma quick_check")
	if err != nil {
		return false, "", err
	}
	defer rows.Close()

	seen := false
	for rows.Next() {
		seen = true
		var result string
		if err := rows.Scan(&result); err != nil {
			return false, "", err
		}
		if strings.ToLower(strings.TrimSpace(result)) != "ok" {
			return false, result, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, "", err
	}
	if !seen {
		return false, "", fmt.Errorf("archive: integrity check returned no rows")
	}
	return true, "", nil
}

// Purpose: Identify common SQLite corruption errors.
// Key aspects: String match keeps dependencies minimal and matches modernc/sqlite errors.
// Upstream: openArchiveDB.
// Downstream: None.
func isSQLiteCorrupted(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database disk image is malformed") ||
		strings.Contains(msg, "file is encrypted or is not a database") ||
		strings.Contains(msg, "file is not a database")
}

// Purpose: Start background loops for inserts and retention cleanup.
// Key aspects: Runs goroutines; writer remains non-blocking for callers.
// Upstream: main.go startup.
// Downstream: insertLoop goroutine, cleanupLoop goroutine.
func (w *Writer) Start() {
	// Goroutine: batched insert loop drains queue without blocking ingest.
	go w.insertLoop()
	// Goroutine: periodic cleanup loop enforces retention windows.
	go w.cleanupLoop()
}

// Purpose: Stop the writer and close the underlying DB.
// Key aspects: Signals loops to exit; closes DB without waiting for full drain.
// Upstream: main.go shutdown.
// Downstream: close(w.stop), db.Close.
func (w *Writer) Stop() {
	close(w.stop)
	_ = w.db.Close()
}

// Purpose: Try to enqueue a spot for archival without blocking.
// Key aspects: Drops silently when the queue is full to protect hot path.
// Upstream: main.go spot ingest/broadcast.
// Downstream: writer queue channel.
func (w *Writer) Enqueue(s *spot.Spot) {
	if w == nil || s == nil {
		return
	}
	select {
	case w.queue <- s:
	default:
		// Drop silently to avoid interfering with the hot path.
	}
}

// Purpose: Batch and insert queued spots into SQLite.
// Key aspects: Uses a size/time batch; flushes on stop signal.
// Upstream: Start goroutine.
// Downstream: flush, time.Timer.
func (w *Writer) insertLoop() {
	batch := make([]*spot.Spot, 0, w.cfg.BatchSize)
	timer := time.NewTimer(time.Duration(w.cfg.BatchIntervalMS) * time.Millisecond)
	defer timer.Stop()

	for {
		select {
		case <-w.stop:
			w.flush(batch)
			return
		case s := <-w.queue:
			batch = append(batch, s)
			if len(batch) >= w.cfg.BatchSize {
				w.flush(batch)
				batch = batch[:0]
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(time.Duration(w.cfg.BatchIntervalMS) * time.Millisecond)
			}
		case <-timer.C:
			if len(batch) > 0 {
				w.flush(batch)
				batch = batch[:0]
			}
			timer.Reset(time.Duration(w.cfg.BatchIntervalMS) * time.Millisecond)
		}
	}
}

// Purpose: Flush a batch of spots into SQLite in a single transaction.
// Key aspects: Best-effort logging on errors; per-spot inserts within tx.
// Upstream: insertLoop.
// Downstream: sql.Tx, stmt.Exec.
func (w *Writer) flush(batch []*spot.Spot) {
	if len(batch) == 0 {
		return
	}
	tx, err := w.db.Begin()
	if err != nil {
		log.Printf("archive: begin tx: %v", err)
		return
	}
	stmt, err := tx.Prepare(`insert into spots(ts, dx, de, freq, mode, report, has_report, comment, source, source_node, ttl, is_beacon, dx_grid, de_grid, confidence, band) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		log.Printf("archive: prepare: %v", err)
		_ = tx.Rollback()
		return
	}
	now := time.Now().UTC()
	for _, s := range batch {
		if s == nil {
			continue
		}
		if _, err := stmt.Exec(
			s.Time.UTC().Unix(),
			s.DXCall,
			s.DECall,
			s.Frequency,
			s.Mode,
			s.Report,
			boolToInt(s.HasReport),
			s.Comment,
			string(s.SourceType),
			s.SourceNode,
			s.TTL,
			boolToInt(s.IsBeacon),
			s.DXMetadata.Grid,
			s.DEMetadata.Grid,
			s.Confidence,
			s.Band,
		); err != nil {
			log.Printf("archive: insert failed: %v", err)
		}
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		log.Printf("archive: commit: %v", err)
	}
	_ = now
}

// Purpose: Periodically enforce retention policy by deleting old rows.
// Key aspects: Uses a ticker; exits on stop signal.
// Upstream: Start goroutine.
// Downstream: cleanupOnce, time.Ticker.
func (w *Writer) cleanupLoop() {
	interval := time.Duration(w.cfg.CleanupIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			w.cleanupOnce()
		}
	}
}

// Purpose: Run one retention cleanup pass.
// Key aspects: Applies separate retention windows and deletes in small batches
// to keep write locks short under high ingest rates.
// Upstream: cleanupLoop.
// Downstream: sql.Exec deletes.
func (w *Writer) cleanupOnce() {
	now := time.Now().UTC().Unix()
	cutoffFT := now - int64(w.cfg.RetentionFTSeconds)
	cutoffDefault := now - int64(w.cfg.RetentionDefaultSeconds)

	batchSize := w.cfg.CleanupBatchSize
	if batchSize <= 0 {
		batchSize = 2000
	}
	yield := time.Duration(w.cfg.CleanupBatchYieldMS) * time.Millisecond
	if w.cfg.CleanupBatchYieldMS < 0 {
		yield = 0
	}

	deleteBatch := func(label, query string, cutoff int64) {
		for {
			// Use rowid-limited deletes to keep each transaction short and avoid long-lived locks.
			res, err := w.db.Exec(query, cutoff, batchSize)
			if err != nil {
				log.Printf("archive: cleanup %s: %v", label, err)
				return
			}
			affected, err := res.RowsAffected()
			if err != nil {
				log.Printf("archive: cleanup %s: rows affected: %v", label, err)
				return
			}
			if affected < int64(batchSize) {
				return
			}
			if yield > 0 {
				time.Sleep(yield)
			}
		}
	}

	deleteBatch("FT", `delete from spots where rowid in (
		select rowid from spots where mode in ('FT8','FT4') and ts < ? limit ?
	)`, cutoffFT)
	deleteBatch("default", `delete from spots where rowid in (
		select rowid from spots where mode not in ('FT8','FT4') and ts < ? limit ?
	)`, cutoffDefault)
}

// Purpose: Ensure the archive schema and indexes exist.
// Key aspects: Executes a single multi-statement schema block.
// Upstream: NewWriter.
// Downstream: db.Exec.
func ensureSchema(db *sql.DB) error {
	schema := `
	create table if not exists spots (
		id integer primary key autoincrement,
		ts integer,
		dx text,
		de text,
		freq real,
		mode text,
		report integer,
		has_report integer,
		comment text,
		source text,
		source_node text,
		ttl integer,
		is_beacon integer,
		dx_grid text,
		de_grid text,
		confidence text,
		band text
	);
	create index if not exists idx_spots_ts on spots(ts);
	create index if not exists idx_spots_mode_ts on spots(mode, ts);
	create index if not exists idx_spots_dx_ts on spots(dx, ts);
	create index if not exists idx_spots_de_ts on spots(de, ts);
	`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("archive: schema: %w", err)
	}
	return nil
}

// Purpose: Convert a bool to a SQLite-friendly integer.
// Key aspects: True->1, false->0.
// Upstream: flush, Recent row mapping.
// Downstream: None.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Purpose: Delete the archive DB files (test helper).
// Key aspects: Removes main DB plus WAL/SHM/journal sidecars when present.
// Upstream: Tests or maintenance tools.
// Downstream: os.Remove.
func DropDB(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("archive: empty path")
	}
	targets := []string{
		path,
		path + "-wal",
		path + "-shm",
		path + "-journal",
	}
	var firstErr error
	for _, target := range targets {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Purpose: Return the most recent N archived spots, newest-first.
// Key aspects: Read-only query; reconstructs Spot objects from rows.
// Upstream: Telnet SHOW/DX handlers when archive is enabled.
// Downstream: db.Query, Spot normalization helpers.
func (w *Writer) Recent(limit int) ([]*spot.Spot, error) {
	if w == nil || w.db == nil {
		return nil, fmt.Errorf("archive: writer is nil")
	}
	if limit <= 0 {
		return []*spot.Spot{}, nil
	}
	rows, err := w.db.Query(`select ts, dx, de, freq, mode, report, has_report, comment, source, source_node, ttl, is_beacon, dx_grid, de_grid, confidence, band from spots order by ts desc limit ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("archive: query recent: %w", err)
	}
	defer rows.Close()

	results := make([]*spot.Spot, 0, limit)
	for rows.Next() {
		var (
			ts         int64
			dx         string
			de         string
			freq       float64
			mode       string
			report     int
			hasReport  int
			comment    string
			source     string
			sourceNode string
			ttl        int
			isBeacon   int
			dxGrid     string
			deGrid     string
			conf       string
			band       string
		)
		if err := rows.Scan(&ts, &dx, &de, &freq, &mode, &report, &hasReport, &comment, &source, &sourceNode, &ttl, &isBeacon, &dxGrid, &deGrid, &conf, &band); err != nil {
			return nil, fmt.Errorf("archive: scan recent: %w", err)
		}
		s := &spot.Spot{
			DXCall:     dx,
			DECall:     de,
			Frequency:  freq,
			Mode:       mode,
			Report:     report,
			Time:       time.Unix(ts, 0).UTC(),
			Comment:    comment,
			SourceType: spot.SourceType(source),
			SourceNode: sourceNode,
			TTL:        uint8(clampToByte(ttl)),
			IsBeacon:   isBeacon > 0,
			HasReport:  hasReport > 0,
			Confidence: conf,
			Band:       band,
		}
		s.DXMetadata.Grid = dxGrid
		s.DEMetadata.Grid = deGrid
		s.EnsureNormalized()
		s.RefreshBeaconFlag()
		results = append(results, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("archive: iterate recent: %w", err)
	}
	return results, nil
}

// Purpose: Clamp an integer into the 0..255 range.
// Key aspects: Used to rebuild uint8 fields from DB rows.
// Upstream: Recent.
// Downstream: None.
func clampToByte(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
