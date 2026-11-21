// 真实死锁（SQLSTATE 40P01）的复现与恢复验证。
//
// 故障现场（后端日志）：
//
//	[conv 1822] append activity failed: 错误: 检测到死锁 (SQLSTATE 40P01)
//
// 成因：conversation_activities.conversation_id 有指向 conversations(id) 的外键，
// INSERT 必须对该行取 ROW SHARE 锁；TouchConversation / UpdateConversation 对同一行
// 取排他锁。两个会话交叉写入对方的父行时形成环路，PG 判定死锁并中止其中一方。
//
// 分两个测试：
//   1) 在真实 PG 上把 40P01 确定性地复现出来，证明这条路径确实会撞死锁；
//   2) 用这个【服务器真实产生的】40P01 错误驱动 execRetryDeadlock，验证它确实
//      能救回被中止的那次写入，而不是把数据丢掉。
package db

import (
	"database/sql"
	"strconv"
	"sync"
	"testing"
	"time"
)

// newConv 造一个临时会话，测试结束自动清理。
func newConv(t *testing.T, d *DB, title string) int64 {
	t.Helper()
	c, err := d.CreateConversation("test-agent", title, nil)
	if err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM conversations WHERE id=$1`, c.ID) })
	return c.ID
}

// insertActivity 在给定事务里插一条活动（走外键，会取父行 ROW SHARE 锁）。
func insertActivity(tx *sql.Tx, convID int64, summary string) error {
	_, err := tx.Exec(`
INSERT INTO conversation_activities(conversation_id, worker, kind, summary)
VALUES ($1,'test','text',$2)`, convID, summary)
	return err
}

// realDeadlockError 在真实 PG 上确定性地制造一次 40P01 并返回该错误。
//
//	连接1: BEGIN; SELECT convA FOR UPDATE;  ──等待──> INSERT convB
//	连接2: BEGIN; SELECT convB FOR UPDATE;  INSERT convA
//
// 两边各自持有对方要申请的父行排他锁，互相申请即构成环路。用 300ms 延时确认
// 连接1 已真正进入锁等待，避免因调度顺序偶发不触发（那样测试会静默失效）。
func realDeadlockError(t *testing.T, d *DB) (errDeadlock error) {
	t.Helper()
	convA, convB := newConv(t, d, "死锁-A"), newConv(t, d, "死锁-B")

	c1, err := d.Conn(t.Context())
	if err != nil {
		t.Fatalf("取连接1失败: %v", err)
	}
	defer c1.Close()
	c2, err := d.Conn(t.Context())
	if err != nil {
		t.Fatalf("取连接2失败: %v", err)
	}
	defer c2.Close()

	tx1, err := c1.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("连接1开事务失败: %v", err)
	}
	defer tx1.Rollback()
	tx2, err := c2.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("连接2开事务失败: %v", err)
	}
	defer tx2.Rollback()

	if _, err := tx1.Exec(`SELECT 1 FROM conversations WHERE id=$1 FOR UPDATE`, convA); err != nil {
		t.Fatalf("连接1锁 convA 失败: %v", err)
	}
	if _, err := tx2.Exec(`SELECT 1 FROM conversations WHERE id=$1 FOR UPDATE`, convB); err != nil {
		t.Fatalf("连接2锁 convB 失败: %v", err)
	}

	// 连接1 去写 convB：被连接2 的排他锁挡住，进入等待
	waiter := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		waiter <- insertActivity(tx1, convB, "连接1写B")
	}()

	time.Sleep(300 * time.Millisecond)
	select {
	case e := <-waiter:
		t.Fatalf("连接1 未进入锁等待（提前返回 %v），死锁场景未建立", e)
	default:
	}

	// 连接2 去写 convA：此刻形成环路，PG 中止其中一方
	err2 := insertActivity(tx2, convA, "连接2写A")

	wg.Wait()
	select {
	case err1 := <-waiter:
		for _, e := range []error{err1, err2} {
			if e != nil && isPostgresDeadlock(e) {
				return e
			}
		}
		t.Fatalf("该场景必须触发 40P01：err1=%v err2=%v", err1, err2)
	case <-time.After(15 * time.Second):
		t.Fatal("连接1 一直未返回：死锁没有被检测到")
	}
	return nil
}

// TestRealDeadlockOnCrossConversationInsert 复现故障现场：交叉写对方父行必撞 40P01。
func TestRealDeadlockOnCrossConversationInsert(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { d.Close() })

	if err := realDeadlockError(t, d); err == nil {
		t.Fatal("未拿到死锁错误")
	}
}

// TestExecRetryDeadlockRecoversFromServerDeadlock 用服务器真实产生的 40P01 驱动重试：
// 第一次失败、第二次成功，最终必须返回 nil 且活动真的落库。
//
// 这验证的是修复的核心承诺：死锁不再等于丢数据。
func TestExecRetryDeadlockRecoversFromServerDeadlock(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { d.Close() })

	serverDeadlock := realDeadlockError(t, d)
	if serverDeadlock == nil || !isPostgresDeadlock(serverDeadlock) {
		t.Fatalf("未取得真实的 40P01：%v", serverDeadlock)
	}

	conv := newConv(t, d, "重试写入")
	attempts := 0

	// 第一次尝试返回服务器真实产生的死锁错误（模拟被 PG 中止），之后正常写入。
	err = execRetryDeadlock(func() error {
		attempts++
		if attempts == 1 {
			return serverDeadlock
		}
		_, e := d.AppendConvActivity(conv, Activity{Kind: "text", Summary: "死锁后重试写入"})
		return e
	})
	if err != nil {
		t.Fatalf("重试后应成功，实际 %v（尝试 %d 次）", err, attempts)
	}
	if attempts != 2 {
		t.Fatalf("应在第 2 次尝试成功，实际 %d 次", attempts)
	}

	list, _, err := d.ConvActivityList(conv, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range list {
		if a.Summary == "死锁后重试写入" {
			found = true
		}
	}
	if !found {
		t.Fatal("重试后活动仍未落库——数据仍然丢了")
	}
	_ = strconv.Itoa(0)
}
