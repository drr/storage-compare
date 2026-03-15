package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"storage-compare/internal/model"
)

type FSBackend struct {
	root string
}

// OpenFS creates a new filesystem backend rooted at root.
func OpenFS(root string) (*FSBackend, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	return &FSBackend{root: root}, nil
}

func (f *FSBackend) Root() string { return f.root }

// BulkWrite writes a slice of entries to the filesystem.
func (f *FSBackend) BulkWrite(entries []*model.Entry) error {
	for _, e := range entries {
		if err := f.Write(e); err != nil {
			return err
		}
	}
	return nil
}

// Write writes an entry as the latest version. If an existing latest file is
// present, it is archived with a version suffix first (version update protocol).
func (f *FSBackend) Write(e *model.Entry) error {
	dir := e.DayDir(f.root)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	latestPath := e.LatestPath(f.root)

	// Check if a current latest exists that needs archiving.
	if _, err := os.Stat(latestPath); err == nil {
		// Determine current version count.
		n, err := f.versionCount(e, dir)
		if err != nil {
			return err
		}
		archivePath := filepath.Join(dir, e.VersionFilename(n))
		if err := os.Rename(latestPath, archivePath); err != nil {
			return err
		}
	}

	data, err := e.FrontmatterYAML()
	if err != nil {
		return err
	}
	return os.WriteFile(latestPath, data, 0644)
}

// ReadLatest reads and parses the latest version of an entry by its creation date and ID.
func (f *FSBackend) ReadLatest(createDateDir, id string) (*model.Entry, error) {
	// We need to find the file. createDateDir is like "2024/2024-03/2024-03-15"
	dir := filepath.Join(f.root, createDateDir)
	// Find the file matching YYYY-MM-DD-{id}.md (no version suffix)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, de := range entries {
		name := de.Name()
		if strings.Contains(name, id) && strings.HasSuffix(name, ".md") && !isVersionedFile(name) {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
			return model.ParseFrontmatter(data)
		}
	}
	return nil, fmt.Errorf("entry %s not found in %s", id, dir)
}

// ReadLatestByPath reads and parses the latest version of an entry given its full file path.
func (f *FSBackend) ReadLatestByPath(path string) (*model.Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return model.ParseFrontmatter(data)
}

// ReadDay reads all latest entries in a day directory.
// dayPath is relative to f.root, e.g. "2024/2024-03/2024-03-15"
func (f *FSBackend) ReadDay(dayPath string) ([]*model.Entry, error) {
	dir := filepath.Join(f.root, dayPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*model.Entry
	for _, de := range entries {
		name := de.Name()
		if !strings.HasSuffix(name, ".md") || isVersionedFile(name) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		e, err := model.ParseFrontmatter(data)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// versionCount returns the current highest version number present in dir for entry e.
// If only the bare .md exists (no -vN files), returns 1.
func (f *FSBackend) versionCount(e *model.Entry, dir string) (int, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return 1, err
	}
	prefix := fmt.Sprintf("%s-%s-v", e.CreateTime.Format("2006-01-02"), e.ID)
	max := 1
	for _, de := range des {
		name := de.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".md") {
			var v int
			inner := name[len(prefix) : len(name)-3]
			if n, _ := fmt.Sscanf(inner, "%d", &v); n == 1 && v >= max {
				max = v + 1
			}
		}
	}
	return max, nil
}

// isVersionedFile returns true if the filename has a -vN suffix before .md
func isVersionedFile(name string) bool {
	// Strip .md suffix and check if the remainder ends with -vN
	base := strings.TrimSuffix(name, ".md")
	if base == name {
		return false
	}
	// Find last "-v" occurrence
	idx := strings.LastIndex(base, "-v")
	if idx < 0 {
		return false
	}
	suffix := base[idx+2:]
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(suffix) > 0
}

// ListDays returns all day directory paths (relative to root) that exist in the FS backend.
func (f *FSBackend) ListDays() ([]string, error) {
	var days []string
	// Walk year/month/day
	years, err := os.ReadDir(f.root)
	if err != nil {
		return nil, err
	}
	for _, year := range years {
		if !year.IsDir() {
			continue
		}
		months, err := os.ReadDir(filepath.Join(f.root, year.Name()))
		if err != nil {
			continue
		}
		for _, month := range months {
			if !month.IsDir() {
				continue
			}
			dayDirs, err := os.ReadDir(filepath.Join(f.root, year.Name(), month.Name()))
			if err != nil {
				continue
			}
			for _, day := range dayDirs {
				if day.IsDir() {
					days = append(days, filepath.Join(year.Name(), month.Name(), day.Name()))
				}
			}
		}
	}
	sort.Strings(days)
	return days, nil
}
