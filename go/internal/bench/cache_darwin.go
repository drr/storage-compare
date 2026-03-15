//go:build darwin

package bench

// PurgeCache invokes sudo purge to evict the OS buffer cache on macOS.
// This requires that the calling process has sudo privileges (or purge is setuid).
// In practice, this is called via scripts/purge_cache.sh before the benchmark binary runs.
// The function is a no-op at the Go level; cache purge is done externally.
func PurgeCache() error {
	return nil
}
