//go:build modernc

package backend

import _ "modernc.org/sqlite"

const sqliteDriverName = "sqlite"

func sqliteDSN(path string) string { return path }
