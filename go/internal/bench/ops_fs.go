package bench

import (
	"math/rand"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"storage-compare/internal/backend"
	"storage-compare/internal/model"
	"storage-compare/internal/wordgen"
)

// FSReadRandomOp returns an Op that opens and parses a random entry's latest .md file.
func FSReadRandomOp(fs *backend.FSBackend, idx []IndexEntry, rng *rand.Rand) Op {
	return func() (time.Duration, int, bool) {
		ie := idx[rng.Intn(len(idx))]
		path := filepath.Join(fs.Root(), ie.DayPath, ie.Day+"-"+ie.ID+".md")
		start := time.Now()
		_, err := fs.ReadLatestByPath(path)
		return time.Since(start), 1, err == nil
	}
}

// FSReadDayOp returns an Op that reads all latest entries in a random day directory.
// The count returned is the number of entries read, enabling per-entry normalization.
func FSReadDayOp(fs *backend.FSBackend, dayPool map[string][]IndexEntry, rng *rand.Rand) Op {
	days := make([]string, 0, len(dayPool))
	dayPathMap := make(map[string]string)
	for d, es := range dayPool {
		days = append(days, d)
		if len(es) > 0 {
			dayPathMap[d] = es[0].DayPath
		}
	}
	return func() (time.Duration, int, bool) {
		day := days[rng.Intn(len(days))]
		start := time.Now()
		entries, err := fs.ReadDay(dayPathMap[day])
		elapsed := time.Since(start)
		count := len(entries)
		if count < 1 {
			count = 1
		}
		return elapsed, count, err == nil
	}
}

// FSCreateEntryOp returns an Op that writes a new entry .md file.
func FSCreateEntryOp(fs *backend.FSBackend, rng *rand.Rand) Op {
	now := time.Now().UTC()
	return func() (time.Duration, int, bool) {
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
		err := fs.Write(e)
		return time.Since(start), 1, err == nil
	}
}

// FSCreateVersionOp returns an Op that picks a random entry from a local pool,
// archives its current file, and writes a new version. Returns ok=false when
// the pool is exhausted.
func FSCreateVersionOp(fs *backend.FSBackend, idx []IndexEntry, rng *rand.Rand) Op {
	pool := make([]IndexEntry, len(idx))
	copy(pool, idx)
	now := time.Now().UTC()
	return func() (time.Duration, int, bool) {
		if len(pool) == 0 {
			return 0, 1, false
		}
		pick := rng.Intn(len(pool))
		ie := pool[pick]
		pool[pick] = pool[len(pool)-1]
		pool = pool[:len(pool)-1]

		path := filepath.Join(fs.Root(), ie.DayPath, ie.Day+"-"+ie.ID+".md")
		existing, err := fs.ReadLatestByPath(path)
		if err != nil {
			return 0, 1, false
		}
		e := &model.Entry{
			ID:         ie.ID,
			VersionID:  ie.VersionCount + 1,
			EntryType:  "markdown-text",
			CreateTime: existing.CreateTime,
			ModifyTime: now,
			IsLatest:   true,
			Content:    wordgen.Generate(rng),
		}
		start := time.Now()
		err = fs.Write(e)
		return time.Since(start), 1, err == nil
	}
}
