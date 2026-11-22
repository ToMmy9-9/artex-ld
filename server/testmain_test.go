package server

import (
	"database/sql"
	"os"
	"testing"

	"artex/db"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestMain acquires a PostgreSQL advisory lock (7337741002) for the entire
// server test suite so cross-package DELETE cleanup races with db/agent
// packages are avoided when running `go test ./...`.
func TestMain(m *testing.M) {
	dsn, _, err := db.DSN()
	if err != nil {
		os.Exit(m.Run())
	}
	if err := db.CheckTestDSN(dsn); err != nil {
		// 打到疑似生产库就整包停下。这里不能退化成 skip：
		// skip 只会让人觉得"测试都过了"，而库里的 fixture 已经放进去了。
		os.Stderr.WriteString("\n" + err.Error() + "\n\n")
		os.Exit(1)
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil || conn.Ping() != nil {
		os.Exit(m.Run())
	}
	// TestMain has no *testing.T — close the advisory-lock connection
	// when the run finishes.
	defer conn.Close()
	if _, err := conn.Exec(`SELECT pg_advisory_lock(7337741002)`); err != nil {
		os.Exit(m.Run())
	}
	defer conn.Exec(`SELECT pg_advisory_unlock(7337741002)`) //nolint:errcheck
	os.Exit(m.Run())
}
