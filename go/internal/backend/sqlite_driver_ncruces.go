//go:build ncruces

package backend

import (
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

const sqliteDriverName = "sqlite3"

func sqliteDSN(path string) string { return path }
