package model

import (
	"time"
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
