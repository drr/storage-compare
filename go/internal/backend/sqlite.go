package backend

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"storage-compare/internal/model"
)

type SQLiteBackend struct {
	db   *sql.DB
	path string
}

const schema = `
PRAGMA journal_mode = WAL;
PRAGMA synchronous  = NORMAL;
PRAGMA page_size    = 4096;

CREATE TABLE IF NOT EXISTS entries (
    id           TEXT    NOT NULL,
    version_id   INTEGER NOT NULL,
    entry_type   TEXT    NOT NULL DEFAULT 'markdown-text',
    create_time  INTEGER NOT NULL,
    modify_time  INTEGER NOT NULL,
    is_latest    INTEGER NOT NULL DEFAULT 1 CHECK (is_latest IN (0,1)),
    content      TEXT,
    PRIMARY KEY (id, version_id)
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_latest_by_id  ON entries(id)          WHERE is_latest = 1;
CREATE INDEX IF NOT EXISTS idx_latest_by_day ON entries(modify_time) WHERE is_latest = 1;
`

// OpenSQLite opens (or creates) the SQLite database at the given path.
func OpenSQLite(dbPath string) (*SQLiteBackend, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &SQLiteBackend{db: db, path: dbPath}, nil
}

func (s *SQLiteBackend) Close() error {
	return s.db.Close()
}

func (s *SQLiteBackend) DB() *sql.DB {
	return s.db
}

// BulkInsert inserts a slice of entries in batches within a single transaction.
func (s *SQLiteBackend) BulkInsert(entries []*model.Entry, batchSize int) error {
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		if err := s.insertBatch(entries[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteBackend) insertBatch(entries []*model.Entry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO entries (id, version_id, entry_type, create_time, modify_time, is_latest, content)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		isLatest := 0
		if e.IsLatest {
			isLatest = 1
		}
		_, err = stmt.Exec(
			e.ID, e.VersionID, e.EntryType,
			e.CreateTime.UnixMilli(), e.ModifyTime.UnixMilli(),
			isLatest, e.Content,
		)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ReadLatest fetches the latest version of an entry by ID.
func (s *SQLiteBackend) ReadLatest(id string) (*model.Entry, error) {
	row := s.db.QueryRow(
		`SELECT id, version_id, entry_type, create_time, modify_time, is_latest, content
		 FROM entries WHERE id = ? AND is_latest = 1`, id)
	return scanEntryFromRow(row)
}

// ReadDay fetches all latest entries with modify_time within [startMs, endMs).
func (s *SQLiteBackend) ReadDay(startMs, endMs int64) ([]*model.Entry, error) {
	rows, err := s.db.Query(
		`SELECT id, version_id, entry_type, create_time, modify_time, is_latest, content
		 FROM entries WHERE modify_time >= ? AND modify_time < ? AND is_latest = 1`,
		startMs, endMs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Entry
	for rows.Next() {
		e, err := scanEntryFromRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddVersion archives the current latest and inserts a new version.
func (s *SQLiteBackend) AddVersion(e *model.Entry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE entries SET is_latest = 0 WHERE id = ? AND is_latest = 1`, e.ID)
	if err != nil {
		tx.Rollback()
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO entries (id, version_id, entry_type, create_time, modify_time, is_latest, content)
		 VALUES (?, ?, ?, ?, ?, 1, ?)`,
		e.ID, e.VersionID, e.EntryType,
		e.CreateTime.UnixMilli(), e.ModifyTime.UnixMilli(),
		e.Content,
	)
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// CreateEntry inserts a brand-new entry (version 1, is_latest=1).
func (s *SQLiteBackend) CreateEntry(e *model.Entry) error {
	_, err := s.db.Exec(
		`INSERT INTO entries (id, version_id, entry_type, create_time, modify_time, is_latest, content)
		 VALUES (?, ?, ?, ?, ?, 1, ?)`,
		e.ID, e.VersionID, e.EntryType,
		e.CreateTime.UnixMilli(), e.ModifyTime.UnixMilli(),
		e.Content,
	)
	return err
}

// MaxModifyTime returns the maximum modify_time (ms) in the database, or 0 if empty.
func (s *SQLiteBackend) MaxModifyTime() (int64, error) {
	var ms sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(modify_time) FROM entries`).Scan(&ms)
	if err != nil {
		return 0, err
	}
	return ms.Int64, nil
}

// Analyze runs ANALYZE to update query planner statistics.
func (s *SQLiteBackend) Analyze() error {
	_, err := s.db.Exec("ANALYZE")
	return err
}

// CountLatest returns the count of is_latest=1 rows.
func (s *SQLiteBackend) CountLatest() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM entries WHERE is_latest = 1`).Scan(&n)
	return n, err
}

// GetVersionCount returns the current max version_id for an entry.
func (s *SQLiteBackend) GetVersionCount(id string) (int, error) {
	var n sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(version_id) FROM entries WHERE id = ?`, id).Scan(&n)
	if err != nil {
		return 0, err
	}
	return int(n.Int64), nil
}

func msToTime(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}

func scanEntry(id *string, versionID *int, entryType *string, createMs, modifyMs *int64, isLatestInt *int, content *string) *model.Entry {
	return &model.Entry{
		ID:         *id,
		VersionID:  *versionID,
		EntryType:  *entryType,
		CreateTime: msToTime(*createMs),
		ModifyTime: msToTime(*modifyMs),
		IsLatest:   *isLatestInt == 1,
		Content:    *content,
	}
}

func scanEntryFromRow(row *sql.Row) (*model.Entry, error) {
	var (
		id         string
		versionID  int
		entryType  string
		createMs   int64
		modifyMs   int64
		isLatestInt int
		content    string
	)
	err := row.Scan(&id, &versionID, &entryType, &createMs, &modifyMs, &isLatestInt, &content)
	if err != nil {
		return nil, err
	}
	return scanEntry(&id, &versionID, &entryType, &createMs, &modifyMs, &isLatestInt, &content), nil
}

func scanEntryFromRows(rows *sql.Rows) (*model.Entry, error) {
	var (
		id          string
		versionID   int
		entryType   string
		createMs    int64
		modifyMs    int64
		isLatestInt int
		content     string
	)
	err := rows.Scan(&id, &versionID, &entryType, &createMs, &modifyMs, &isLatestInt, &content)
	if err != nil {
		return nil, err
	}
	return scanEntry(&id, &versionID, &entryType, &createMs, &modifyMs, &isLatestInt, &content), nil
}
