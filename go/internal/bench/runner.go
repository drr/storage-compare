package bench

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// IndexEntry mirrors the JSON structure of data/index.json.
type IndexEntry struct {
	ID           string `json:"id"`
	Day          string `json:"day"`
	DayPath      string `json:"day_path"`
	VersionCount int    `json:"version_count"`
}

// LoadIndex reads data/index.json.
func LoadIndex(path string) ([]IndexEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var idx []IndexEntry
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return idx, nil
}

// DayPool builds a map of day -> []IndexEntry for days with >= minEntries entries.
func DayPool(idx []IndexEntry, minEntries int) map[string][]IndexEntry {
	m := make(map[string][]IndexEntry)
	for _, e := range idx {
		m[e.Day] = append(m[e.Day], e)
	}
	out := make(map[string][]IndexEntry)
	for day, es := range m {
		if len(es) >= minEntries {
			out[day] = es
		}
	}
	return out
}

// Op is a single invocation of a benchmark operation.
// Returns (latency, ok). ok=false means this call should be skipped
// (e.g. pool exhausted for create_version); the runner will not count it.
type Op func() (time.Duration, bool)

// RunAdaptive runs op in rounds of batchSize until the 95% CI for the median
// is narrower than precision (relative to the median), or maxN total valid
// samples are collected. batchSize is never decreased below its initial value.
// Returns the collected timings and whether convergence was achieved.
func RunAdaptive(op Op, batchSize int, precision float64, maxN int) ([]time.Duration, bool) {
	timings := make([]time.Duration, 0, batchSize)
	collectBatch := func(n int) {
		consecutiveFails := 0
		for len(timings) < cap(timings) && consecutiveFails < 20 {
			d, ok := op()
			if ok {
				timings = append(timings, d)
				consecutiveFails = 0
			} else {
				consecutiveFails++
			}
		}
		_ = n
	}

	// Use a simpler approach: collect in rounds, check convergence between rounds.
	collected := 0
	for collected < maxN {
		need := min(batchSize, maxN-collected)
		// Grow cap if needed
		if cap(timings)-len(timings) < need {
			next := make([]time.Duration, len(timings), len(timings)+need)
			copy(next, timings)
			timings = next
		}
		_ = collectBatch

		consecutiveFails := 0
		for i := 0; i < need && consecutiveFails < 20; {
			d, ok := op()
			if ok {
				timings = append(timings, d)
				collected++
				consecutiveFails = 0
				i++
			} else {
				consecutiveFails++
			}
		}

		if consecutiveFails >= 20 {
			break // op is exhausted (e.g. pool drained)
		}

		if medianConverged(timings, precision) {
			return timings, true
		}
	}

	return timings, medianConverged(timings, precision)
}

// medianConverged returns true if the 95% CI for the median (computed via
// order statistics) is narrower than precision relative to the median.
// Requires at least 30 samples; returns false below that threshold.
func medianConverged(timings []time.Duration, precision float64) bool {
	n := len(timings)
	if n < 30 {
		return false
	}
	sorted := sortedDurations(timings)
	medIdx := n / 2
	median := sorted[medIdx]
	if median == 0 {
		return true
	}
	// 95% CI for median via order statistics: span ±ceil(0.98*sqrt(n)) from median index.
	// Derived from the binomial: P(X_lo ≤ median ≤ X_hi) ≥ 0.95 where
	// lo = floor(n/2 - 0.98*sqrt(n)), hi = ceil(n/2 + 0.98*sqrt(n)).
	k := int(math.Ceil(0.98 * math.Sqrt(float64(n))))
	lo := medIdx - k
	hi := medIdx + k
	if lo < 0 {
		lo = 0
	}
	if hi >= n {
		hi = n - 1
	}
	relWidth := float64(sorted[hi]-sorted[lo]) / float64(median)
	return relWidth < precision
}

// MedianCIWidth returns the relative 95% CI width for the median of a sorted slice.
// Returns 1.0 if there are fewer than 30 samples.
func MedianCIWidth(sorted []time.Duration) float64 {
	n := len(sorted)
	if n < 30 {
		return 1.0
	}
	medIdx := n / 2
	median := sorted[medIdx]
	if median == 0 {
		return 0.0
	}
	k := int(math.Ceil(0.98 * math.Sqrt(float64(n))))
	lo := medIdx - k
	hi := medIdx + k
	if lo < 0 {
		lo = 0
	}
	if hi >= n {
		hi = n - 1
	}
	return float64(sorted[hi]-sorted[lo]) / float64(median)
}

func sortedDurations(timings []time.Duration) []time.Duration {
	s := make([]time.Duration, len(timings))
	copy(s, timings)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s
}

// Result holds per-operation timing data.
type Result struct {
	Backend   string
	Operation string
	Timings   []time.Duration
	Converged bool
}

func (r *Result) Stats() Stats {
	return ComputeStats(r.Timings)
}

// Stats holds computed latency statistics.
type Stats struct {
	N        int
	Min      time.Duration
	Median   time.Duration
	P95      time.Duration
	P99      time.Duration
	Max      time.Duration
	OpsSec   float64
	MedianCI float64 // relative 95% CI width for the median
}

// ComputeStats computes min/median/p95/p99/max/ops-per-sec from a slice of durations.
func ComputeStats(timings []time.Duration) Stats {
	if len(timings) == 0 {
		return Stats{}
	}
	sorted := sortedDurations(timings)
	n := len(sorted)
	median := sorted[n/2]
	p95 := sorted[int(math.Ceil(float64(n)*0.95))-1]
	p99 := sorted[int(math.Ceil(float64(n)*0.99))-1]
	opsSec := 0.0
	if median > 0 {
		opsSec = float64(time.Second) / float64(median)
	}
	return Stats{
		N:        n,
		Min:      sorted[0],
		Median:   median,
		P95:      p95,
		P99:      p99,
		Max:      sorted[n-1],
		OpsSec:   opsSec,
		MedianCI: MedianCIWidth(sorted),
	}
}

func fmtDur(d time.Duration) string {
	ms := float64(d) / float64(time.Millisecond)
	return fmt.Sprintf("%.2fms", ms)
}

func fmtCI(ci float64, converged bool) string {
	pct := ci * 100
	if !converged {
		return fmt.Sprintf("%4.1f%%!", pct)
	}
	return fmt.Sprintf("%5.1f%%", pct)
}

// PrintTable prints the results in the ASCII table format.
// The medCI column shows the relative 95% CI width for the median;
// a trailing '!' means the precision target was not reached.
func PrintTable(results []*Result, population int) {
	fmt.Printf("Runtime: go  |  Population: %d  |  Date: %s\n\n",
		population, time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("%-10s | %-15s | %5s | %6s | %7s | %7s | %7s | %7s | %7s | %7s\n",
		"Backend", "Operation", "N", "medCI", "Min", "Median", "P95", "P99", "Max", "ops/sec")
	fmt.Println(strings.Repeat("-", 100))
	for _, r := range results {
		s := r.Stats()
		fmt.Printf("%-10s | %-15s | %5d | %6s | %7s | %7s | %7s | %7s | %7s | %7.0f\n",
			r.Backend, r.Operation, s.N,
			fmtCI(s.MedianCI, r.Converged),
			fmtDur(s.Min), fmtDur(s.Median), fmtDur(s.P95), fmtDur(s.P99), fmtDur(s.Max),
			s.OpsSec,
		)
	}
}

// SaveCSV writes per-operation nanosecond timings to a CSV file.
func SaveCSV(resultsDir, lang, backend, operation string, timings []time.Duration) error {
	dir := filepath.Join(resultsDir, lang)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s_%s_timings.csv", backend, operation))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, d := range timings {
		fmt.Fprintf(f, "%d\n", d.Nanoseconds())
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
