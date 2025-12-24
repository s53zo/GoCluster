package peer

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type topologyStore struct {
	db        *sql.DB
	retention time.Duration
}

func openTopologyStore(path string, retention time.Duration) (*topologyStore, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`pragma journal_mode=WAL;`); err != nil {
		return nil, err
	}
	if err := ensurePeerNodesSchema(db); err != nil {
		return nil, err
	}
	return &topologyStore{db: db, retention: retention}, nil
}

func ensurePeerNodesSchema(db *sql.DB) error {
	schema := `
	create table if not exists peer_nodes (
		id integer primary key autoincrement,
		origin text,
		bitmap int,
		call text,
		version text,
		build text,
		ip text,
		updated_at integer
	);
	create index if not exists idx_peer_nodes_origin on peer_nodes(origin);
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	cols, err := fetchColumns(db, "peer_nodes")
	if err != nil {
		return err
	}
	need := []string{"origin", "bitmap", "call", "version", "build", "ip", "updated_at"}
	missing := false
	for _, col := range need {
		if _, ok := cols[col]; ok {
			continue
		}
		missing = true
	}
	if missing {
		if _, err := db.Exec(`drop table if exists peer_nodes;`); err != nil {
			return err
		}
		if _, err := db.Exec(schema); err != nil {
			return err
		}
	}
	return nil
}

func fetchColumns(db *sql.DB, table string) (map[string]struct{}, error) {
	rows, err := db.Query(fmt.Sprintf("pragma table_info(%s);", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := make(map[string]struct{})
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[strings.ToLower(name)] = struct{}{}
	}
	return cols, rows.Err()
}

func (t *topologyStore) applyPC92(frame *Frame, now time.Time) {
	if t == nil || frame == nil {
		return
	}
	fields := frame.payloadFields()
	// Expected payload fields (after "PC92^"):
	//   0: origin node
	//   1: timestamp
	//   2: record type (A/C/D/K)
	//   3+: node entries: <bitmap><call>:<version>[:<build>[:<ip>]]
	if len(fields) < 3 {
		return
	}
	origin := strings.TrimSpace(fields[0])
	if origin == "" {
		origin = frame.Type // fallback; should not happen
	}
	recordType := strings.TrimSpace(fields[2])
	entries := fields[3:]
	if len(entries) == 0 {
		return
	}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if isHopField(entry) {
			continue
		}
		bitmap, call, version, build, ip := parsePC92Entry(entry)
		if strings.TrimSpace(call) == "" {
			continue
		}
		updatedAt := now.Unix()
		if strings.EqualFold(recordType, "D") {
			// Delete record type: remove matching origin+call rows.
			_, _ = t.db.Exec(`delete from peer_nodes where origin = ? and call = ?`, origin, call)
			continue
		}
		t.upsertPeerNode(origin, bitmap, call, version, build, ip, updatedAt)
	}
}

func (t *topologyStore) applyLegacy(frame *Frame, now time.Time) {
	if t == nil {
		return
	}
	_, _ = t.db.Exec(`insert into peer_nodes(origin, bitmap, call, version, build, ip, updated_at) values(?,?,?,?,?,?,?)`,
		frame.Type, 0, "", "", "", "", now.Unix())
}

func (t *topologyStore) upsertPeerNode(origin string, bitmap int, call, version, build, ip string, updatedAt int64) {
	if t == nil {
		return
	}
	// Best-effort upsert: delete any existing row for origin+call, then insert fresh state.
	_, _ = t.db.Exec(`delete from peer_nodes where origin = ? and call = ?`, origin, call)
	_, _ = t.db.Exec(`insert into peer_nodes(origin, bitmap, call, version, build, ip, updated_at) values(?,?,?,?,?,?,?)`,
		origin, bitmap, call, version, build, ip, updatedAt)
}

// isHopField returns true when the token is the trailing hop marker (e.g., H27).
func isHopField(token string) bool {
	token = strings.TrimSpace(strings.ToUpper(token))
	if !strings.HasPrefix(token, "H") || len(token) < 2 {
		return false
	}
	for i := 1; i < len(token); i++ {
		if token[i] < '0' || token[i] > '9' {
			return false
		}
	}
	return true
}

func (t *topologyStore) prune(now time.Time) {
	if t == nil {
		return
	}
	cutoff := now.Add(-t.retention).Unix()
	_, _ = t.db.Exec(`delete from peer_nodes where updated_at < ?`, cutoff)
}

func (t *topologyStore) Close() error {
	if t == nil {
		return nil
	}
	return t.db.Close()
}

func parsePC92Entry(entry string) (bitmap int, call, version, build, ip string) {
	// entry format: <bitmap><call>:<version>[:<build>[:<ip>]]
	parts := strings.Split(entry, ":")
	head := parts[0]
	if len(head) > 0 {
		bitmap = int(head[0] - '0')
		if len(head) > 1 {
			call = head[1:]
		}
	}
	if len(parts) > 1 {
		version = parts[1]
	}
	if len(parts) > 2 {
		build = parts[2]
	}
	if len(parts) > 3 {
		ip = parts[3]
	}
	return
}
