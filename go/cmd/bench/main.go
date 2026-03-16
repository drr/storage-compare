package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"storage-compare/internal/backend"
	"storage-compare/internal/bench"
)

func main() {
	dataDir := flag.String("data-dir", "./data", "root data dir")
	resultsDir := flag.String("results-dir", "./results", "results output dir")
	readRandom := flag.Int("read-random", 1000, "minimum read_random operations")
	readDay := flag.Int("read-day", 200, "minimum read_day operations")
	createEntry := flag.Int("create-entry", 500, "minimum create_entry operations")
	createVersion := flag.Int("create-version", 200, "minimum create_version operations")
	ftsSearch := flag.Int("fts-search", 200, "minimum fts_search operations")
	precision := flag.Float64("precision", 0.05, "target relative 95%% CI width for the median (e.g. 0.05 = 5%%)")
	maxFactor := flag.Int("max-factor", 10, "max samples = min-N * max-factor")
	seed := flag.Int64("seed", time.Now().UnixNano(), "RNG seed")
	fts := flag.Bool("fts", false, "also benchmark SQLite-FTS database at data/sqlite-fts/notes.db")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))

	indexPath := filepath.Join(*dataDir, "index.json")
	idx, err := bench.LoadIndex(indexPath)
	if err != nil {
		log.Fatalf("load index: %v", err)
	}
	if len(idx) == 0 {
		log.Fatal("index.json is empty — run generate first")
	}
	fmt.Printf("Loaded %d entries from index.\n", len(idx))
	fmt.Printf("Precision target: %.0f%% relative CI for median | max-factor: %dx\n\n", *precision*100, *maxFactor)

	dayPool := bench.DayPool(idx, 10)
	if len(dayPool) == 0 {
		log.Fatal("no days with >=10 entries — generate more data")
	}

	dbPath := filepath.Join(*dataDir, "sqlite", "notes.db")
	ftsDPath := filepath.Join(*dataDir, "sqlite-fts", "notes.db")

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

	var fsDB *backend.FSBackend
	if !*fts {
		fsRoot := filepath.Join(*dataDir, "fs")
		fsDB, err = backend.OpenFS(fsRoot)
		if err != nil {
			log.Fatalf("open fs: %v", err)
		}
	}

	if err := os.MkdirAll(*resultsDir, 0755); err != nil {
		log.Fatalf("create results dir: %v", err)
	}

	var results []*bench.Result

	run := func(backendName, opName string, minN int, op bench.Op) {
		timings, perUnit, converged := bench.RunAdaptive(op, minN, *precision, minN**maxFactor)
		r := &bench.Result{Backend: backendName, Operation: opName, Timings: timings, PerUnit: perUnit, Converged: converged}
		results = append(results, r)
		if err := bench.SaveCSV(*resultsDir, "go", backendName, opName, timings); err != nil {
			log.Printf("save csv %s/%s: %v", backendName, opName, err)
		}
	}

	// Run order: reads first (before writes change the cache picture), then writes.
	fmt.Println("=== SQLite ===")
	run("sqlite", "read_random", *readRandom, bench.SQLiteReadRandomOp(sqliteDB, idx, rng))
	run("sqlite", "read_day", *readDay, bench.SQLiteReadDayOp(sqliteDB, dayPool, rng))
	run("sqlite", "create_entry", *createEntry, bench.SQLiteCreateEntryOp(sqliteDB, rng))
	run("sqlite", "create_version", *createVersion, bench.SQLiteCreateVersionOp(sqliteDB, idx, rng))

	if fsDB != nil {
		fmt.Println("=== Filesystem ===")
		run("filesystem", "read_random", *readRandom, bench.FSReadRandomOp(fsDB, idx, rng))
		run("filesystem", "read_day", *readDay, bench.FSReadDayOp(fsDB, dayPool, rng))
		run("filesystem", "create_entry", *createEntry, bench.FSCreateEntryOp(fsDB, rng))
		run("filesystem", "create_version", *createVersion, bench.FSCreateVersionOp(fsDB, idx, rng))
	}

	if ftsDB != nil {
		fmt.Println("=== SQLite-FTS ===")
		run("sqlite-fts", "read_random", *readRandom, bench.SQLiteReadRandomOp(ftsDB.SQLiteBackend, idx, rng))
		run("sqlite-fts", "read_day", *readDay, bench.SQLiteReadDayOp(ftsDB.SQLiteBackend, dayPool, rng))
		run("sqlite-fts", "create_entry", *createEntry, bench.SQLiteCreateEntryOp(ftsDB.SQLiteBackend, rng))
		run("sqlite-fts", "create_version", *createVersion, bench.SQLiteCreateVersionOp(ftsDB.SQLiteBackend, idx, rng))
		run("sqlite-fts", "fts_search", *ftsSearch, bench.SQLiteFTSSearchOp(ftsDB, rng))
	}

	fmt.Println()
	bench.PrintTable(results, len(idx))
}
