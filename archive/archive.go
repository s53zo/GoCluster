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

// NewWriter initializes the SQLite database and returns a writer; call Start to begin processing.
func NewWriter(cfg config.ArchiveConfig) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, fmt.Errorf("archive: mkdir: %w", err)
	}
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("archive: open db: %w", err)
	}
	if _, err := db.Exec(`pragma journal_mode=WAL; pragma synchronous=NORMAL; pragma busy_timeout=` + fmt.Sprintf("%d", cfg.BusyTimeoutMS)); err != nil {
		return nil, fmt.Errorf("archive: pragmas: %w", err)
	}
	if err := ensureSchema(db); err != nil {
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

// Start launches the insert and cleanup loops.
func (w *Writer) Start() {
	go w.insertLoop()
	go w.cleanupLoop()
}

// Stop closes the writer; best-effort flush.
func (w *Writer) Stop() {
	close(w.stop)
	_ = w.db.Close()
}

// Enqueue attempts to queue a spot for archival without blocking; drops on full queue.
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

func (w *Writer) cleanupOnce() {
	now := time.Now().UTC().Unix()
	cutoffFT := now - int64(w.cfg.RetentionFTSeconds)
	cutoffDefault := now - int64(w.cfg.RetentionDefaultSeconds)

	// FT modes
	if _, err := w.db.Exec(`delete from spots where mode in ('FT8','FT4') and ts < ?`, cutoffFT); err != nil {
		log.Printf("archive: cleanup FT: %v", err)
	}
	// All others
	if _, err := w.db.Exec(`delete from spots where mode not in ('FT8','FT4') and ts < ?`, cutoffDefault); err != nil {
		log.Printf("archive: cleanup default: %v", err)
	}
}

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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// DropDB is a helper to reset the archive during testing.
func DropDB(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("archive: empty path")
	}
	return os.Remove(path)
}
