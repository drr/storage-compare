//go:build !darwin

package bench

// PurgeCache is a no-op on non-Darwin platforms.
func PurgeCache() error {
	return nil
}
