package main

import (
	"encoding/json"
	"flag"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"storage-compare/internal/backend"
	"storage-compare/internal/model"
	"storage-compare/internal/wordgen"
)

type IndexEntry struct {
	ID           string `json:"id"`
	Day          string `json:"day"`   // YYYY-MM-DD (creation date)
	DayPath      string `json:"day_path"` // e.g. "2024/2024-03/2024-03-15"
	VersionCount int    `json:"version_count"`
}

func main() {
	count := flag.Int("count", 10000, "entries to generate")
	dataDir := flag.String("data-dir", "./data", "root data dir")
	seed := flag.Int64("seed", time.Now().UnixNano(), "RNG seed")
	batchSize := flag.Int("batch-size", 500, "SQLite insert batch size")
	appendMode := flag.Bool("append", false, "add to existing population without truncating")
	fts := flag.Bool("fts", false, "also write to SQLite-FTS database at data/sqlite-fts/notes.db")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))

	dbPath := filepath.Join(*dataDir, "sqlite", "notes.db")
	ftsDPath := filepath.Join(*dataDir, "sqlite-fts", "notes.db")
	indexPath := filepath.Join(*dataDir, "index.json")

	// Open backends
	sqliteDB, err := backend.OpenSQLite(dbPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer sqliteDB.Close()

	var ftsDB *backend.SQLiteFTSBackend
	if *fts {
		ftsDB, err = backend.OpenSQLiteFTS(ftsDPath)
		if err != nil {
			log.Fatalf("open sqlite-fts: %v", err)
		}
		defer ftsDB.Close()
	}

	// Determine time range
	var startTime time.Time
	if *appendMode {
		maxMs, err := sqliteDB.MaxModifyTime()
		if err != nil || maxMs == 0 {
			// Fall back to 2 years ago
			startTime = time.Now().UTC().Add(-2 * 365 * 24 * time.Hour)
		} else {
			startTime = time.UnixMilli(maxMs).UTC()
		}
	} else {
		startTime = time.Now().UTC().Add(-2 * 365 * 24 * time.Hour)
	}

	now := time.Now().UTC()
	totalSeconds := now.Sub(startTime).Seconds()
	if totalSeconds <= 0 {
		totalSeconds = float64(2 * 365 * 24 * 3600)
	}

	log.Printf("Generating %d entries (seed=%d, append=%v)", *count, *seed, *appendMode)

	// Generate all entries
	entries := make([]*model.Entry, 0, *count)
	for i := 0; i < *count; i++ {
		id := uuid.New().String()
		var content string
		// When FTS is enabled, 10% of entries get a searchable phrase embedded.
		if *fts && rng.Intn(10) == 0 {
			phrase := wordgen.TagPhrases[rng.Intn(len(wordgen.TagPhrases))]
			content = wordgen.GenerateWithPhrase(rng, phrase)
		} else {
			content = wordgen.Generate(rng)
		}

		// Uniform random timestamp in range
		offsetSecs := rng.Float64() * totalSeconds
		createDay := startTime.Add(time.Duration(offsetSecs) * time.Second)
		// Clamp to same day 08:00–18:00
		y, m, d := createDay.Date()
		baseTime := time.Date(y, m, d, 8, 0, 0, 0, time.UTC)
		jitter := time.Duration(rng.Int63n(int64(10 * time.Hour)))
		createTime := baseTime.Add(jitter)
		modifyTime := createTime

		e := &model.Entry{
			ID:         id,
			VersionID:  1,
			EntryType:  "markdown-text",
			CreateTime: createTime,
			ModifyTime: modifyTime,
			IsLatest:   true,
			Content:    content,
		}
		entries = append(entries, e)
	}

	// Write to backends in parallel
	var wg sync.WaitGroup
	var sqliteErr, ftsErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("Writing %d entries to SQLite...", len(entries))
		sqliteErr = sqliteDB.BulkInsert(entries, *batchSize)
		if sqliteErr == nil {
			sqliteErr = sqliteDB.Analyze()
		}
	}()

	if ftsDB != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Printf("Writing %d entries to SQLite-FTS...", len(entries))
			ftsErr = ftsDB.BulkInsert(entries, *batchSize)
			if ftsErr == nil {
				ftsErr = ftsDB.Analyze()
			}
		}()
	}

	wg.Wait()

	if sqliteErr != nil {
		log.Fatalf("sqlite write: %v", sqliteErr)
	}
	if ftsErr != nil {
		log.Fatalf("sqlite-fts write: %v", ftsErr)
	}

	// Update index.json
	var index []IndexEntry
	if *appendMode {
		data, err := os.ReadFile(indexPath)
		if err == nil {
			json.Unmarshal(data, &index)
		}
	}

	for _, e := range entries {
		dayStr := e.CreateTime.Format("2006-01-02")
		dayPath := filepath.Join(
			e.CreateTime.Format("2006"),
			e.CreateTime.Format("2006-01"),
			dayStr,
		)
		index = append(index, IndexEntry{
			ID:           e.ID,
			Day:          dayStr,
			DayPath:      dayPath,
			VersionCount: 1,
		})
	}

	if err := os.MkdirAll(filepath.Dir(indexPath), 0755); err != nil {
		log.Fatalf("mkdir index dir: %v", err)
	}
	indexData, err := json.Marshal(index)
	if err != nil {
		log.Fatalf("marshal index: %v", err)
	}
	if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
		log.Fatalf("write index: %v", err)
	}

	// Verify
	count2, err := sqliteDB.CountLatest()
	if err != nil {
		log.Printf("count verify: %v", err)
	} else {
		log.Printf("SQLite latest count: %d", count2)
	}

	if ftsDB != nil {
		ftsCount, err := ftsDB.CountFTS()
		if err != nil {
			log.Printf("fts count verify: %v", err)
		} else {
			log.Printf("SQLite-FTS indexed rows: %d", ftsCount)
		}
	}

	log.Printf("Done. index.json has %d entries.", len(index))
}
