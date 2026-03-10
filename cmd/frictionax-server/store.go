package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sageox/frictionax"

	_ "modernc.org/sqlite"
)

// Store provides SQLite-backed storage for friction events and catalog data.
type Store struct {
	db *sql.DB
}

// NewStore opens a SQLite database at dbPath and initializes the schema.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return &Store{db: db}, nil
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS friction_events (
		id TEXT PRIMARY KEY,
		ts TEXT NOT NULL,
		kind TEXT NOT NULL,
		command TEXT DEFAULT '',
		subcommand TEXT DEFAULT '',
		actor TEXT NOT NULL,
		agent_type TEXT DEFAULT '',
		path_bucket TEXT DEFAULT '',
		input TEXT DEFAULT '',
		error_msg TEXT DEFAULT '',
		suggestion TEXT DEFAULT '',
		created_at TEXT DEFAULT (datetime('now'))
	);

	CREATE INDEX IF NOT EXISTS idx_events_kind ON friction_events(kind);
	CREATE INDEX IF NOT EXISTS idx_events_ts ON friction_events(ts);
	CREATE INDEX IF NOT EXISTS idx_events_actor ON friction_events(actor);

	CREATE TABLE IF NOT EXISTS catalog (
		version TEXT PRIMARY KEY,
		data TEXT NOT NULL,
		created_at TEXT DEFAULT (datetime('now'))
	);
	`
	_, err := db.Exec(schema)
	return err
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// newID generates a time-sortable unique ID.
func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// InsertEvents stores friction events, returning the count of inserted events.
func (s *Store) InsertEvents(events []frictionax.FrictionEvent) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO friction_events (id, ts, kind, command, subcommand, actor, agent_type, path_bucket, input, error_msg, suggestion)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, e := range events {
		_, err := stmt.Exec(
			newID(),
			e.Timestamp,
			string(e.Kind),
			e.Command,
			e.Subcommand,
			e.Actor,
			e.AgentType,
			e.PathBucket,
			e.Input,
			e.ErrorMsg,
			e.Suggestion,
		)
		if err != nil {
			return inserted, fmt.Errorf("insert event: %w", err)
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return inserted, nil
}

// EventCount returns the total number of stored friction events.
func (s *Store) EventCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM friction_events").Scan(&count)
	return count, err
}

// SummaryResult holds aggregated friction data.
type SummaryResult struct {
	TotalEvents int               `json:"total_events"`
	ByKind      map[string]int    `json:"by_kind"`
	ByActor     map[string]int    `json:"by_actor"`
	TopInputs   []TopInput        `json:"top_inputs"`
	Since       string            `json:"since"`
}

// TopInput represents a frequently seen friction input.
type TopInput struct {
	Input string `json:"input"`
	Count int    `json:"count"`
	Kind  string `json:"kind"`
}

// Summary returns aggregated friction data, optionally filtered by kind and time range.
func (s *Store) Summary(kind string, since time.Time, limit int) (*SummaryResult, error) {
	if limit <= 0 {
		limit = 10
	}

	result := &SummaryResult{
		ByKind:  make(map[string]int),
		ByActor: make(map[string]int),
		Since:   since.UTC().Format(time.RFC3339),
	}

	sinceStr := since.UTC().Format(time.RFC3339)

	// total events matching filters
	var totalQuery string
	var totalArgs []interface{}
	if kind != "" {
		totalQuery = "SELECT COUNT(*) FROM friction_events WHERE ts >= ? AND kind = ?"
		totalArgs = []interface{}{sinceStr, kind}
	} else {
		totalQuery = "SELECT COUNT(*) FROM friction_events WHERE ts >= ?"
		totalArgs = []interface{}{sinceStr}
	}
	if err := s.db.QueryRow(totalQuery, totalArgs...).Scan(&result.TotalEvents); err != nil {
		return nil, fmt.Errorf("count total: %w", err)
	}

	// breakdown by kind
	kindRows, err := s.db.Query("SELECT kind, COUNT(*) FROM friction_events WHERE ts >= ? GROUP BY kind ORDER BY COUNT(*) DESC", sinceStr)
	if err != nil {
		return nil, fmt.Errorf("query by kind: %w", err)
	}
	defer kindRows.Close()
	for kindRows.Next() {
		var k string
		var c int
		if err := kindRows.Scan(&k, &c); err != nil {
			return nil, fmt.Errorf("scan kind row: %w", err)
		}
		result.ByKind[k] = c
	}

	// breakdown by actor
	actorRows, err := s.db.Query("SELECT actor, COUNT(*) FROM friction_events WHERE ts >= ? GROUP BY actor ORDER BY COUNT(*) DESC", sinceStr)
	if err != nil {
		return nil, fmt.Errorf("query by actor: %w", err)
	}
	defer actorRows.Close()
	for actorRows.Next() {
		var a string
		var c int
		if err := actorRows.Scan(&a, &c); err != nil {
			return nil, fmt.Errorf("scan actor row: %w", err)
		}
		result.ByActor[a] = c
	}

	// top inputs
	var topQuery string
	var topArgs []interface{}
	if kind != "" {
		topQuery = "SELECT input, COUNT(*) as cnt, kind FROM friction_events WHERE ts >= ? AND kind = ? AND input != '' GROUP BY input, kind ORDER BY cnt DESC LIMIT ?"
		topArgs = []interface{}{sinceStr, kind, limit}
	} else {
		topQuery = "SELECT input, COUNT(*) as cnt, kind FROM friction_events WHERE ts >= ? AND input != '' GROUP BY input, kind ORDER BY cnt DESC LIMIT ?"
		topArgs = []interface{}{sinceStr, limit}
	}
	topRows, err := s.db.Query(topQuery, topArgs...)
	if err != nil {
		return nil, fmt.Errorf("query top inputs: %w", err)
	}
	defer topRows.Close()
	for topRows.Next() {
		var ti TopInput
		if err := topRows.Scan(&ti.Input, &ti.Count, &ti.Kind); err != nil {
			return nil, fmt.Errorf("scan top input: %w", err)
		}
		result.TopInputs = append(result.TopInputs, ti)
	}

	if result.TopInputs == nil {
		result.TopInputs = []TopInput{}
	}

	return result, nil
}

// GetCatalog returns the most recently stored catalog, or nil if none exists.
func (s *Store) GetCatalog() (*frictionax.CatalogData, error) {
	var dataStr string
	err := s.db.QueryRow("SELECT data FROM catalog ORDER BY created_at DESC LIMIT 1").Scan(&dataStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query catalog: %w", err)
	}

	var catalog frictionax.CatalogData
	if err := json.Unmarshal([]byte(dataStr), &catalog); err != nil {
		return nil, fmt.Errorf("unmarshal catalog: %w", err)
	}
	return &catalog, nil
}

// SetCatalog stores a new catalog version.
func (s *Store) SetCatalog(catalog *frictionax.CatalogData) error {
	data, err := json.Marshal(catalog)
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}

	_, err = s.db.Exec(
		"INSERT OR REPLACE INTO catalog (version, data) VALUES (?, ?)",
		catalog.Version, string(data),
	)
	return err
}

// Patterns returns aggregated friction patterns with actor breakdown.
// Patterns are grouped by input, kind, and error_msg.
func (s *Store) Patterns(minCount, limit int) ([]frictionax.PatternDetail, error) {
	if limit <= 0 {
		limit = 100
	}
	if minCount < 1 {
		minCount = 1
	}

	query := `
		SELECT
			input AS pattern,
			kind,
			error_msg,
			COUNT(*) AS total_count,
			SUM(CASE WHEN actor = 'human' THEN 1 ELSE 0 END) AS human_count,
			SUM(CASE WHEN actor = 'agent' THEN 1 ELSE 0 END) AS agent_count,
			COUNT(DISTINCT CASE WHEN agent_type != '' THEN agent_type ELSE NULL END) AS agent_types,
			COALESCE(MAX(created_at), '') AS latest_version,
			COALESCE(MIN(ts), '') AS first_seen,
			COALESCE(MAX(ts), '') AS last_seen
		FROM friction_events
		WHERE input IS NOT NULL AND input != ''
		GROUP BY input, kind, error_msg
		HAVING COUNT(*) >= ?
		ORDER BY total_count DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, minCount, limit)
	if err != nil {
		return nil, fmt.Errorf("query patterns: %w", err)
	}
	defer rows.Close()

	var patterns []frictionax.PatternDetail
	for rows.Next() {
		var p frictionax.PatternDetail
		if err := rows.Scan(
			&p.Pattern, &p.Kind, &p.ErrorMsg,
			&p.TotalCount, &p.HumanCount, &p.AgentCount, &p.AgentTypes,
			&p.LatestVersion, &p.FirstSeen, &p.LastSeen,
		); err != nil {
			return nil, fmt.Errorf("scan pattern: %w", err)
		}
		patterns = append(patterns, p)
	}

	if patterns == nil {
		patterns = []frictionax.PatternDetail{}
	}

	return patterns, nil
}
