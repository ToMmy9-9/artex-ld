// finding_invocation_abandon_test.go：补发 sweeper 必须区分永久失败与瞬时故障。
//
// 背景（实测缺陷）：findings.task_id 是 ON DELETE SET NULL。任务一删，finding 的
// task_id 变 NULL，enqueueFindingInvocation 的归属校验必然失败。早期代码把所有
// 错误一视同仁地只打日志，而 ClaimFindingInvocationRetry 早已推进 last_retry_at，
// 于是这些行每 10 分钟原样失败一次、永不收敛；更糟的是 PendingFindingInvocations
// 按 created_at 升序取前 N 条，这些老僵尸长期霸占队头，把真正需要补发的新派发饿死。
// 线上实测 100 条这样的行，一行都跑不掉。
package server

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// 归属已失效（finding.task_id 为 NULL，等价于任务被删）的派发必须被就地放弃，
// 之后再也不会出现在待补发集合里，也不会被认领重放。
func TestRetryFindingInvocationsAbandonsPermanentFailure(t *testing.T) {
	m, s := fiFixture(t)
	task := fiCreateTask(t, m, "fi 永久失败任务")
	taskID := mustTaskID(t, task.ID)
	findingID, _ := fiCreateFinding(t, m, task, "fi 归属会失效的 finding")
	fiCreateAgent(t, m, "fitest_perm", "")

	if _, _, err := m.pg.RecordFindingInvocation("fitest_perm", findingID, taskID, "任务删除前的派发"); err != nil {
		t.Fatal(err)
	}
	// 模拟任务删除：findings.task_id → NULL（与 ON DELETE SET NULL 一致）。
	if _, err := m.pg.Exec(`UPDATE findings SET task_id=NULL WHERE id=$1`, findingID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.pg.Exec(`UPDATE finding_agent_invocations SET created_at=now()-interval '11 minutes'
		WHERE agent_key='fitest_perm' AND finding_id=$1`, findingID); err != nil {
		t.Fatal(err)
	}

	sc := newScheduler(s)
	sc.retryFindingInvocations(time.Now())

	// 不能入队：归属校验必须拒绝。
	if got := fiQueueLen(s, "fitest_perm"); got != 0 {
		t.Fatalf("永久失败的派发不得入队，实际 %d 条", got)
	}
	// 必须被标记放弃。
	var abandoned *time.Time
	var reason string
	if err := m.pg.QueryRow(`SELECT abandoned_at, abandon_reason FROM finding_agent_invocations
		WHERE agent_key='fitest_perm' AND finding_id=$1`, findingID).Scan(&abandoned, &reason); err != nil {
		t.Fatal(err)
	}
	if abandoned == nil {
		t.Fatal("永久失败必须写 abandoned_at，否则下一轮会原样重试")
	}
	if reason == "" {
		t.Fatal("放弃原因不能为空，排障时需要看到为什么放弃")
	}

	// 再次 sweep（越过 10 分钟节流）也不能再拾取它。
	sc.lastFindingInvocationSweep = time.Time{}
	sc.retryFindingInvocations(time.Now().Add(time.Hour))
	if got := fiQueueLen(s, "fitest_perm"); got != 0 {
		t.Fatalf("已放弃的派发在后续轮次仍被重试，实际入队 %d 条", got)
	}
	if ok, err := m.pg.ClaimFindingInvocationRetry("fitest_perm", findingID, time.Millisecond); err != nil || ok {
		t.Fatalf("已放弃的派发不得被认领: ok=%v err=%v", ok, err)
	}
}

// 对照：一切正常的派发不得被放弃，否则会永久丢掉这次派发。
func TestRetryFindingInvocationsDoesNotAbandonSuccessfulDispatch(t *testing.T) {
	m, s := fiFixture(t)
	task := fiCreateTask(t, m, "fi 正常派发任务")
	taskID := mustTaskID(t, task.ID)
	findingID, _ := fiCreateFinding(t, m, task, "fi 正常 finding")
	fiCreateAgent(t, m, "fitest_ok", "")

	if _, _, err := m.pg.RecordFindingInvocation("fitest_ok", findingID, taskID, "正常派发"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.pg.Exec(`UPDATE finding_agent_invocations SET created_at=now()-interval '11 minutes'
		WHERE agent_key='fitest_ok' AND finding_id=$1`, findingID); err != nil {
		t.Fatal(err)
	}

	sc := newScheduler(s)
	sc.retryFindingInvocations(time.Now())

	if got := fiQueueLen(s, "fitest_ok"); got != 1 {
		t.Fatalf("正常派发应入队 1 条，实际 %d", got)
	}
	var abandoned *time.Time
	if err := m.pg.QueryRow(`SELECT abandoned_at FROM finding_agent_invocations
		WHERE agent_key='fitest_ok' AND finding_id=$1`, findingID).Scan(&abandoned); err != nil {
		t.Fatal(err)
	}
	if abandoned != nil {
		t.Fatal("成功入队的派发不得被标记放弃")
	}
}

// 哨兵值本身要稳定：包装后的永久错误必须能被 errors.Is 识别，瞬时错误不能。
func TestFindingInvocationPermanentSentinel(t *testing.T) {
	wrapped := fmt.Errorf("外层上下文: %w", permanentInvocationErr("任务不存在"))
	if !errors.Is(wrapped, errFindingInvocationPermanent) {
		t.Fatal("包装后的永久错误必须可被 errors.Is 识别，否则 sweeper 会把它当瞬时故障无限重试")
	}
	transient := errors.New("读取 Finding 失败: 连接超时")
	if errors.Is(transient, errFindingInvocationPermanent) {
		t.Fatal("瞬时故障不得被误判为永久失败，否则一次抖动就永久丢掉派发")
	}
	// 裸错误构造也必须带上哨兵。
	if !errors.Is(permanentInvocationErr("目标 Agent 已禁用"), errFindingInvocationPermanent) {
		t.Fatal("permanentInvocationErr 必须包装哨兵")
	}
}
