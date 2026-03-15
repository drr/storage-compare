package bench

import (
	"math/rand"
	"time"

	"github.com/google/uuid"

	"storage-compare/internal/backend"
	"storage-compare/internal/model"
	"storage-compare/internal/wordgen"
)

// SQLiteReadRandomOp returns an Op that picks a random entry and reads its latest version.
func SQLiteReadRandomOp(db *backend.SQLiteBackend, idx []IndexEntry, rng *rand.Rand) Op {
	return func() (time.Duration, bool) {
		e := idx[rng.Intn(len(idx))]
		start := time.Now()
		_, err := db.ReadLatest(e.ID)
		return time.Since(start), err == nil
	}
}

// SQLiteReadDayOp returns an Op that picks a random day and reads all latest entries in it.
func SQLiteReadDayOp(db *backend.SQLiteBackend, dayPool map[string][]IndexEntry, rng *rand.Rand) Op {
	days := make([]string, 0, len(dayPool))
	for d := range dayPool {
		days = append(days, d)
	}
	return func() (time.Duration, bool) {
		day := days[rng.Intn(len(days))]
		t, _ := time.Parse("2006-01-02", day)
		startMs := t.UTC().UnixMilli()
		endMs := t.UTC().Add(24 * time.Hour).UnixMilli()
		start := time.Now()
		_, err := db.ReadDay(startMs, endMs)
		return time.Since(start), err == nil
	}
}

// SQLiteCreateEntryOp returns an Op that inserts a new entry.
func SQLiteCreateEntryOp(db *backend.SQLiteBackend, rng *rand.Rand) Op {
	now := time.Now().UTC()
	return func() (time.Duration, bool) {
		e := &model.Entry{
			ID:         uuid.New().String(),
			VersionID:  1,
			EntryType:  "markdown-text",
			CreateTime: now,
			ModifyTime: now,
			IsLatest:   true,
			Content:    wordgen.Generate(rng),
		}
		start := time.Now()
		err := db.CreateEntry(e)
		return time.Since(start), err == nil
	}
}

// SQLiteCreateVersionOp returns an Op that picks a random entry from a local pool,
// reads its current state, and adds a new version. Returns ok=false when the pool
// is exhausted (signals the runner to stop).
func SQLiteCreateVersionOp(db *backend.SQLiteBackend, idx []IndexEntry, rng *rand.Rand) Op {
	pool := make([]IndexEntry, len(idx))
	copy(pool, idx)
	now := time.Now().UTC()
	return func() (time.Duration, bool) {
		if len(pool) == 0 {
			return 0, false
		}
		pick := rng.Intn(len(pool))
		ie := pool[pick]
		pool[pick] = pool[len(pool)-1]
		pool = pool[:len(pool)-1]

		vCount, err := db.GetVersionCount(ie.ID)
		if err != nil || vCount == 0 {
			return 0, false
		}
		existing, err := db.ReadLatest(ie.ID)
		if err != nil {
			return 0, false
		}
		e := &model.Entry{
			ID:         ie.ID,
			VersionID:  vCount + 1,
			EntryType:  "markdown-text",
			CreateTime: existing.CreateTime,
			ModifyTime: now,
			IsLatest:   true,
			Content:    wordgen.Generate(rng),
		}
		start := time.Now()
		err = db.AddVersion(e)
		return time.Since(start), err == nil
	}
}
