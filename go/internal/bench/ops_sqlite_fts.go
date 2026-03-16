package bench

import (
	"math/rand"
	"time"

	"storage-compare/internal/backend"
	"storage-compare/internal/wordgen"
)

// SQLiteFTSSearchOp returns an Op that runs an FTS MATCH query against
// entries_fts. It picks a uniformly random phrase from wordgen.TagPhrases each
// invocation and returns (elapsed, numResults, ok). numResults enables
// per-result normalization in the output table.
func SQLiteFTSSearchOp(db *backend.SQLiteFTSBackend, rng *rand.Rand) Op {
	phrases := wordgen.TagPhrases
	return func() (time.Duration, int, bool) {
		phrase := phrases[rng.Intn(len(phrases))]
		t0 := time.Now()
		ids, err := db.SearchFTS(phrase, 100)
		elapsed := time.Since(t0)
		if err != nil {
			return 0, 1, false
		}
		count := len(ids)
		if count == 0 {
			count = 1
		}
		return elapsed, count, true
	}
}
