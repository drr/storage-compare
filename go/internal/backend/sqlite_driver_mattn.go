//go:build !modernc && !ncruces

package backend

import _ "github.com/mattn/go-sqlite3"

const sqliteDriverName = "sqlite3"

func sqliteDSN(path string) string { return path }
