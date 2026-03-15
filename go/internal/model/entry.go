package model

import (
	"fmt"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Entry struct {
	ID         string
	VersionID  int
	EntryType  string
	CreateTime time.Time
	ModifyTime time.Time
	IsLatest   bool
	Content    string
}

// DayDir returns the directory path for this entry based on its creation date.
func (e *Entry) DayDir(fsRoot string) string {
	d := e.CreateTime
	return filepath.Join(fsRoot, d.Format("2006"), d.Format("2006-01"), d.Format("2006-01-02"))
}

// LatestFilename returns the filename for the latest version (no version suffix).
func (e *Entry) LatestFilename() string {
	return fmt.Sprintf("%s-%s.md", e.CreateTime.Format("2006-01-02"), e.ID)
}

// VersionFilename returns the filename for a specific archived version.
func (e *Entry) VersionFilename(v int) string {
	return fmt.Sprintf("%s-%s-v%d.md", e.CreateTime.Format("2006-01-02"), e.ID, v)
}

// LatestPath returns the full path for the latest version file.
func (e *Entry) LatestPath(fsRoot string) string {
	return filepath.Join(e.DayDir(fsRoot), e.LatestFilename())
}

type frontmatter struct {
	ID         string `yaml:"id"`
	VersionID  int    `yaml:"version_id"`
	EntryType  string `yaml:"entry_type"`
	CreateTime string `yaml:"create_time"`
	ModifyTime string `yaml:"modify_time"`
}

// FrontmatterYAML returns the YAML frontmatter + content for filesystem storage.
func (e *Entry) FrontmatterYAML() ([]byte, error) {
	fm := frontmatter{
		ID:         e.ID,
		VersionID:  e.VersionID,
		EntryType:  e.EntryType,
		CreateTime: e.CreateTime.UTC().Format(time.RFC3339),
		ModifyTime: e.ModifyTime.UTC().Format(time.RFC3339),
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return nil, err
	}
	out := []byte("---\n")
	out = append(out, fmBytes...)
	out = append(out, []byte("---\n\n")...)
	out = append(out, []byte(e.Content)...)
	return out, nil
}

// ParseFrontmatter parses a file's YAML frontmatter and content into an Entry.
func ParseFrontmatter(data []byte) (*Entry, error) {
	// Find the second "---"
	if len(data) < 3 || string(data[:3]) != "---" {
		return nil, fmt.Errorf("missing frontmatter delimiter")
	}
	rest := data[3:]
	// skip newline after first ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}
	end := -1
	for i := 0; i < len(rest)-3; i++ {
		if rest[i] == '-' && rest[i+1] == '-' && rest[i+2] == '-' {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("missing closing frontmatter delimiter")
	}
	fmData := rest[:end]
	var fm frontmatter
	if err := yaml.Unmarshal(fmData, &fm); err != nil {
		return nil, err
	}
	ct, err := time.Parse(time.RFC3339, fm.CreateTime)
	if err != nil {
		return nil, err
	}
	mt, err := time.Parse(time.RFC3339, fm.ModifyTime)
	if err != nil {
		return nil, err
	}
	content := ""
	body := rest[end+3:]
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}
	content = string(body)
	return &Entry{
		ID:         fm.ID,
		VersionID:  fm.VersionID,
		EntryType:  fm.EntryType,
		CreateTime: ct,
		ModifyTime: mt,
		IsLatest:   true,
		Content:    content,
	}, nil
}
