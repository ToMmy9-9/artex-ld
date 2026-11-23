package db

import (
	"testing"
	"time"
)

func invocationDB(t *testing.T) (*DB, int64) {
	t.Helper()
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	// 清掉早于本测试文件的残留派发行（那些用例已被删除，但行留在共享测试库里）。
	// PendingFindingInvocations 按 created_at 升序 + LIMIT 取数，残留行会把
	// 本用例的行挤出结果集，表现为「pending selection/order mismatch []」——
	// 失败原因在测试库而不在被测代码，不清掉会让用例莫名其妙地红。
	if _, err := d.Exec(`DELETE FROM finding_agent_invocations`); err != nil {
		t.Fatalf("clear stale invocations: %v", err)
	}
	fid, err := d.AddFinding(0, 0, "invocation-test", "调用记录测试漏洞", "high", "original summary", "original evidence", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec(`DELETE FROM finding_agent_invocations WHERE finding_id=$1`, fid)
		_, _ = d.DeleteFinding(fid)
		d.Close()
	})
	return d, fid
}

func mustRecordInvocation(t *testing.T, d *DB, agentKey string, findingID, taskID int64, reason string) FindingInvocation {
	t.Helper()
	inv, inserted, err := d.RecordFindingInvocation(agentKey, findingID, taskID, reason)
	if err != nil || !inserted {
		t.Fatalf("record invocation %s/%d: inserted=%v err=%v", agentKey, findingID, inserted, err)
	}
	return inv
}

func TestFindingInvocationRecordIdempotent(t *testing.T) {
	d, fid := invocationDB(t)

	inv, inserted, err := d.RecordFindingInvocation("vuln_dispatcher", fid, 101, "首次派发")
	if err != nil || !inserted {
		t.Fatalf("first record: inserted=%v err=%v", inserted, err)
	}
	if inv.AgentKey != "vuln_dispatcher" || inv.FindingID != fid || inv.TaskID != 101 || inv.Reason != "首次派发" {
		t.Fatal("stored fields mismatch", inv)
	}
	if inv.ConversationID != nil || inv.LastRetryAt != nil || inv.CreatedAt.IsZero() {
		t.Fatal("new invocation must start pending without retry timestamps", inv)
	}

	again, inserted, err := d.RecordFindingInvocation("vuln_dispatcher", fid, 202, "重复派发")
	if err != nil || inserted {
		t.Fatalf("duplicate record must read the existing row: inserted=%v err=%v", inserted, err)
	}
	if again.TaskID != 101 || again.Reason != "首次派发" {
		t.Fatal("conflict must not overwrite task_id/reason", again)
	}
	if !again.CreatedAt.Equal(inv.CreatedAt) {
		t.Fatal("conflict must not touch created_at", again.CreatedAt, inv.CreatedAt)
	}

	// 不同 agent 可以各自记录对同一 finding 的调用。
	other, inserted, err := d.RecordFindingInvocation("trace_cleaner", fid, 101, "另一目标")
	if err != nil || !inserted {
		t.Fatalf("second agent on same finding: inserted=%v err=%v", inserted, err)
	}
	if other.FindingID != fid || other.AgentKey != "trace_cleaner" {
		t.Fatal("second agent record mismatch", other)
	}
}

func TestFindingInvocationSetConversationOnlyWhenNull(t *testing.T) {
	d, fid := invocationDB(t)
	mustRecordInvocation(t, d, "vuln_dispatcher", fid, 1, "派发")

	if err := d.SetFindingInvocationConversation("vuln_dispatcher", fid, 777); err != nil {
		t.Fatal(err)
	}
	got, inserted, err := d.RecordFindingInvocation("vuln_dispatcher", fid, 1, "再读")
	if err != nil || inserted {
		t.Fatal(err)
	}
	if got.ConversationID == nil || *got.ConversationID != 777 {
		t.Fatal("conversation backfill missing", got)
	}

	// 重复/合并投递不能覆盖第一个已知会话 ID。
	if err := d.SetFindingInvocationConversation("vuln_dispatcher", fid, 888); err != nil {
		t.Fatal(err)
	}
	got, _, err = d.RecordFindingInvocation("vuln_dispatcher", fid, 1, "三读")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConversationID == nil || *got.ConversationID != 777 {
		t.Fatal("second backfill must not overwrite the first conversation", got)
	}
}

func TestFindingInvocationClaimThrottle(t *testing.T) {
	d, fid := invocationDB(t)
	mustRecordInvocation(t, d, "agent_a", fid, 1, "fresh")

	// NULL + 不足 10 分钟：不可认领（覆盖 0 分钟与 9 分钟）。
	if ok, err := d.ClaimFindingInvocationRetry("agent_a", fid, 10*time.Minute); err != nil || ok {
		t.Fatalf("fresh invocation must not be claimable: ok=%v err=%v", ok, err)
	}
	if _, err := d.Exec(`UPDATE finding_agent_invocations SET created_at=now() - interval '9 minutes'
		WHERE agent_key='agent_a' AND finding_id=$1`, fid); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.ClaimFindingInvocationRetry("agent_a", fid, 10*time.Minute); err != nil || ok {
		t.Fatalf("9-minute-old invocation must not be claimable: ok=%v err=%v", ok, err)
	}

	// NULL + 超过 10 分钟：可认领；认领推进 last_retry_at 后立即再次认领失败。
	if _, err := d.Exec(`UPDATE finding_agent_invocations SET created_at=now() - interval '11 minutes'
		WHERE agent_key='agent_a' AND finding_id=$1`, fid); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.ClaimFindingInvocationRetry("agent_a", fid, 10*time.Minute); err != nil || !ok {
		t.Fatalf("due invocation must be claimed: ok=%v err=%v", ok, err)
	}
	if ok, err := d.ClaimFindingInvocationRetry("agent_a", fid, 10*time.Minute); err != nil || ok {
		t.Fatalf("immediate re-claim must fail: ok=%v err=%v", ok, err)
	}

	// 已回填 conversation 的记录永不被认领，即使时间早已到期。
	mustRecordInvocation(t, d, "agent_b", fid, 1, "已回填")
	if err := d.SetFindingInvocationConversation("agent_b", fid, 555); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE finding_agent_invocations SET created_at=now() - interval '2 hours'
		WHERE agent_key='agent_b' AND finding_id=$1`, fid); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.ClaimFindingInvocationRetry("agent_b", fid, 10*time.Minute); err != nil || ok {
		t.Fatalf("backfilled invocation must never be claimed: ok=%v err=%v", ok, err)
	}
}

func TestFindingInvocationPendingSelection(t *testing.T) {
	d, fid := invocationDB(t)
	old := mustRecordInvocation(t, d, "agent_old", fid, 1, "最旧")
	mid := mustRecordInvocation(t, d, "agent_mid", fid, 1, "中间")
	mustRecordInvocation(t, d, "agent_fresh", fid, 1, "最新")
	if _, err := d.Exec(`UPDATE finding_agent_invocations SET created_at=now() - interval '1 hour'
		WHERE agent_key='agent_old' AND finding_id=$1`, fid); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE finding_agent_invocations SET created_at=now() - interval '20 minutes'
		WHERE agent_key='agent_mid' AND finding_id=$1`, fid); err != nil {
		t.Fatal(err)
	}

	// 15 分钟门控：fresh（0 分钟）排除，old/mid 按 created_at 升序返回。
	pending, err := d.PendingFindingInvocations(15*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, inv := range pending {
		if inv.FindingID == fid {
			keys = append(keys, inv.AgentKey)
		}
	}
	if len(keys) != 2 || keys[0] != old.AgentKey || keys[1] != mid.AgentKey {
		t.Fatal("pending selection/order mismatch", keys)
	}

	// limit 生效。
	limited, err := d.PendingFindingInvocations(15*time.Minute, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0].AgentKey != old.AgentKey {
		t.Fatal("limit=1 must return only the oldest", limited)
	}

	// 回填 mid 后只剩 old；认领 old 推进 last_retry_at 后也不再到期（COALESCE）。
	if err := d.SetFindingInvocationConversation("agent_mid", fid, 666); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.ClaimFindingInvocationRetry("agent_old", fid, 15*time.Minute); err != nil || !ok {
		t.Fatal(err)
	}
	pending, err = d.PendingFindingInvocations(15*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, inv := range pending {
		if inv.FindingID == fid {
			t.Fatal("no invocation of this finding may stay pending", inv)
		}
	}
}

// 永久失败的派发必须能被就地放弃，否则 sweeper 每 10 分钟原样重试一次，
// 永不收敛，还会按 created_at 升序长期霸占队头把新派发饿死。
func TestFindingInvocationAbandonExcludedFromPending(t *testing.T) {
	d, fid := invocationDB(t)
	mustRecordInvocation(t, d, "agent_zombie", fid, 999, "永不成功的派发")

	// 造一条早就到期、本来会被 sweeper 拾取的行。
	if _, err := d.Exec(`UPDATE finding_agent_invocations
		SET created_at=now() - interval '2 hours', last_retry_at=NULL
		WHERE agent_key='agent_zombie' AND finding_id=$1`, fid); err != nil {
		t.Fatal(err)
	}
	pending, err := d.PendingFindingInvocations(10*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, inv := range pending {
		if inv.AgentKey == "agent_zombie" && inv.FindingID == fid {
			found = true
		}
	}
	if !found {
		t.Fatal("到期行必须先在待补发集合里，否则本用例证明不了放弃生效")
	}

	if err := d.AbandonFindingInvocation("agent_zombie", fid, "finding 与调用记录的任务归属不一致"); err != nil {
		t.Fatal(err)
	}
	pending, err = d.PendingFindingInvocations(10*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, inv := range pending {
		if inv.AgentKey == "agent_zombie" && inv.FindingID == fid {
			t.Fatal("已放弃的派发不得再出现在待补发集合里", inv)
		}
	}

	// 认领也要拒绝：否则正常调用路径还能把它重新排进队。
	ok, err := d.ClaimFindingInvocationRetry("agent_zombie", fid, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("已放弃的派发不得被认领")
	}

	// 放弃要幂等，且不得覆盖已回填（已启动）的会话归属。
	if err := d.AbandonFindingInvocation("agent_zombie", fid, "重复上报"); err != nil {
		t.Fatal(err)
	}
	var reason string
	if err := d.QueryRow(`SELECT abandon_reason FROM finding_agent_invocations
		WHERE agent_key='agent_zombie' AND finding_id=$1`, fid).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "finding 与调用记录的任务归属不一致" {
		t.Fatalf("重复放弃不得改写首次原因，得到 %q", reason)
	}
}
