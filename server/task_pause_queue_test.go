// task_pause_queue_test.go：锁住「暂停必须出队」这条不变量。
//
// 背景（真实缺陷）：一个因为并发额度或 LLM 不可用而挂在准入队列里的任务
// （tasks.queued=true），用户暂停它之后，如果只把 paused 置真而不摘队列，
// 就会留下 paused+queued 的组合。下游两处都拒绝这个状态：
//   - QueueTaskArchive 见 queued 直接回 ErrTaskArchiveQueued，任务永远无法归档；
//   - StartTaskRunSegment 的 WHERE 带 NOT queued，恢复运行也起不来。
// 再叠加 reconcileConcurrency 会跳过 paused 任务，它既不会被自动推进，也不会
// 被自动出队 —— 用户只能靠"配好 LLM 让它先跑起来"才能脱身。
//
// 生产路径 ApplyTaskPauseOriginReason 的 SQL 里已经带了 queued=false，但
// Manager.SetTaskPaused 走的 SetPausedRuntime 没有。两者语义必须一致，否则
// 谁调用后者谁踩坑（原先就是这个分歧让归档用例偶发失败）。
package server

import (
	"strconv"
	"testing"
)

// pausedTaskFixture 建一个任务，可选先入队，然后返回它的 id 和 manager。
func pausedTaskFixture(t *testing.T) (*Manager, int64) {
	t.Helper()
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	t.Cleanup(func() { m.Close() })

	task, err := m.CreateTask("暂停出队用例", "验证暂停会清理队列", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	tid, err := strconv.ParseInt(task.ID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		m.pg.Exec(`DELETE FROM task_archives WHERE task_id=$1`, tid)
		m.pg.DeleteTask(tid)
	})
	return m, tid
}

func queuedFlag(t *testing.T, m *Manager, tid int64) bool {
	t.Helper()
	var queued bool
	if err := m.pg.QueryRow(`SELECT queued FROM tasks WHERE id=$1`, tid).Scan(&queued); err != nil {
		t.Fatalf("读取 queued 失败：%v", err)
	}
	return queued
}

func TestSetTaskPausedClearsQueue(t *testing.T) {
	m, tid := pausedTaskFixture(t)
	if err := m.EnqueueTask(strconv.FormatInt(tid, 10), "bootstrap"); err != nil {
		t.Fatal(err)
	}
	if !queuedFlag(t, m, tid) {
		t.Fatal("前置条件不成立：入队后 queued 应为 true")
	}

	if err := m.SetTaskPaused(strconv.FormatInt(tid, 10), true); err != nil {
		t.Fatal(err)
	}
	if queuedFlag(t, m, tid) {
		t.Fatal("暂停后 queued 必须为 false，否则任务既起不来也无法归档")
	}
}

func TestPausedQueuedTaskCanBeArchived(t *testing.T) {
	m, tid := pausedTaskFixture(t)
	if err := m.EnqueueTask(strconv.FormatInt(tid, 10), "bootstrap"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTaskPaused(strconv.FormatInt(tid, 10), true); err != nil {
		t.Fatal(err)
	}
	// 这条是用户视角的验收：暂停之后点归档，不该被"还在排队"挡住。
	if _, err := m.pg.QueueTaskArchive(tid); err != nil {
		t.Fatalf("暂停后应能归档，实际：%v", err)
	}
}

func TestApplyTaskPauseOriginReasonClearsQueue(t *testing.T) {
	// 生产路径（UI 暂停走这条）必须和 SetTaskPaused 保持一致。
	// 两条都清了队列，才不会出现"某个入口暂停后归档不了"的分裂行为。
	m, tid := pausedTaskFixture(t)
	if err := m.EnqueueTask(strconv.FormatInt(tid, 10), "bootstrap"); err != nil {
		t.Fatal(err)
	}
	if err := m.ApplyTaskPauseOriginReason(strconv.FormatInt(tid, 10), "paused_by_user", false); err != nil {
		t.Fatal(err)
	}
	if queuedFlag(t, m, tid) {
		t.Fatal("生产暂停路径也必须清掉 queued")
	}
}
