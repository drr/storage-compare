package backend

import (
	"fmt"
	"os"
	"path/filepath"
)

// ftsSchema adds the FTS5 virtual table and an AFTER INSERT trigger to keep it
// in sync with the entries table. The trigger fires inside the same transaction
// as any INSERT INTO entries, so FTS population is atomic.
const ftsSchema = `
CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
    entry_id  UNINDEXED,
    content,
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS entries_fts_ai
AFTER INSERT ON entries BEGIN
    INSERT INTO entries_fts(entry_id, content)
    VALUES (new.id, new.content);
END;
`

// SQLiteFTSBackend wraps SQLiteBackend and adds FTS5 full-text search.
// All write methods (BulkInsert, CreateEntry, AddVersion) are inherited
// unchanged — the trigger keeps entries_fts in sync automatically.
type SQLiteFTSBackend struct {
	*SQLiteBackend
}

// OpenSQLiteFTS opens (or creates) an SQLite database with FTS5 support at
// the given path. It applies the base schema first, then the FTS schema on top.
func OpenSQLiteFTS(dbPath string) (*SQLiteFTSBackend, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}
	base, err := OpenSQLite(dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := base.db.Exec(ftsSchema); err != nil {
		base.Close()
		return nil, fmt.Errorf("fts schema: %w", err)
	}
	return &SQLiteFTSBackend{SQLiteBackend: base}, nil
}

// SearchFTS performs a full-text MATCH query and returns up to limit entry IDs,
// ordered by FTS rank (best match first).
func (s *SQLiteFTSBackend) SearchFTS(query string, limit int) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT entry_id FROM entries_fts WHERE entries_fts MATCH ? ORDER BY rank LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CountFTS returns the total number of rows in entries_fts.
func (s *SQLiteFTSBackend) CountFTS() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM entries_fts`).Scan(&n)
	return n, err
}
