package evidence

import (
	"context"
	"os"
	"testing"

	"artex/db"
)

// Initialize an explicitly configured fresh database before taking the same
// suite lock as db, agent and server. Hold it on one pinned connection.
func TestMain(m *testing.M) {
	if os.Getenv("ARTEX_PG_DSN") == "" {
		os.Exit(m.Run())
	}
	if err := db.CheckTestDSN(os.Getenv("ARTEX_PG_DSN")); err != nil {
		// 打到疑似生产库就整包停下。这里不能退化成 skip：
		// skip 只会让人觉得"测试都过了"，而库里的 fixture 已经放进去了。
		os.Stderr.WriteString("\n" + err.Error() + "\n\n")
		os.Exit(1)
	}
	os.Exit(runEvidenceSuite(m))
}
func runEvidenceSuite(m *testing.M) int {
	pg, err := db.Open(os.Getenv("ARTEX_PG_DSN"))
	if err != nil {
		panic(err)
	}
	defer pg.Close()
	conn, err := pg.Conn(context.Background())
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	if _, err = conn.ExecContext(context.Background(), `SELECT pg_advisory_lock(7337741002)`); err != nil {
		panic(err)
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(7337741002)`)
	return m.Run()
}
