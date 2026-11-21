package server

// ═══ [审核流 · reviewProvider 回落链测试] ═══
// 覆盖 reviewProvider 的四级回落：任务链 → agent 绑定 → is_default → LLM 池。
// 最关键是最后一级：库里只有优先级档、没有任何 is_default 时（实测踩中：
// 2 条 profile、is_default=0 条），旧实现会返回 nil，审核轮永久静默跳过 ——
// 表现为「开了自动处置，意图却一直堆着」。
// 二开日期：2026-11-21

import (
	"testing"

	"artex/db"
)

// reviewProviderTestEnv 准备干净的 profile / agent 绑定现场，测试后原样还原。
type reviewProviderTestEnv struct {
	server    *Server
	profileID int64
	task      *Task
}

// setupReviewProviderEnv 造一个可用 profile，并清空所有「可能干扰回落链」的状态：
// 任务链为空、review agent 未绑 LLM、无 is_default。
func setupReviewProviderEnv(t *testing.T) *reviewProviderTestEnv {
	t.Helper()
	s, pg := newTestServer(t)

	// 快照 review agent 的绑定，测试后还原（否则污染真实配置）。
	a, err := pg.GetAgentByKey("review")
	if err != nil {
		t.Fatalf("GetAgentByKey(review): %v", err)
	}
	if a == nil {
		t.Skip("review agent 未初始化")
	}
	origBind := a.LLMProfileID
	origEnabled := a.Enabled
	t.Cleanup(func() {
		_ = pg.SetAgentLLMProfile("review", origBind)
		_, _ = pg.Exec(`UPDATE agents SET enabled=$1 WHERE key='review'`, origEnabled)
	})

	// 快照并清空 is_default（回落链的第三级）。cleanup 会还原原默认，
	// 不能只清掉不清回来 —— 那会把运维的默认档永久关掉。
	snapshotActiveLLMProfile(t, pg)
	if _, err := pg.Exec(`UPDATE llm_profiles SET is_default=false`); err != nil {
		t.Fatalf("clear defaults: %v", err)
	}

	// 造一个可用档（有 key、非 jev、pool_exclude=false）。
	pid, err := pg.SaveProfile(&db.LLMProfile{
		Name:    "review-provider-fallback-test",
		Format:  "anthropic",
		BaseURL: "http://127.0.0.1:1", // 不会真发请求，只验证 provider 解析
		Model:   "fallback-test-model",
		APIKey:  "test-key-fallback",
	})
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	t.Cleanup(func() { _, _ = pg.Exec(`DELETE FROM llm_profiles WHERE id=$1`, pid) })

	// 关掉所有其它可用档的 pool 参与，确保回落一定命中我们这条（保持判定确定）。
	// 用优先级把本档顶到最高，比逐条改 pool_exclude 更少副作用。
	if _, err := pg.Exec(`UPDATE llm_profiles SET priority=999 WHERE id=$1`, pid); err != nil {
		t.Fatalf("set priority: %v", err)
	}

	if _, err := pg.Exec(`UPDATE agents SET enabled=true WHERE key='review'`); err != nil {
		t.Fatalf("enable review agent: %v", err)
	}
	if err := pg.SetAgentLLMProfile("review", nil); err != nil {
		t.Fatalf("unbind review agent llm: %v", err)
	}

	return &reviewProviderTestEnv{
		server:    s,
		profileID: pid,
		task:      &Task{ID: "0"}, // 任务链为空
	}
}

// TestReviewProviderFallsBackToPool 核心回归：无任务链、无 agent 绑定、无 is_default
// 时，仍必须从 LLM 池里拿到一个可用 provider。
func TestReviewProviderFallsBackToPool(t *testing.T) {
	env := setupReviewProviderEnv(t)

	rr := &ReviewRunner{s: env.server}
	prov := rr.reviewProvider(env.task)
	if prov == nil {
		t.Fatalf("无 is_default 但池里有可用档时，reviewProvider 必须回落到池，实际返回 nil" +
			"（这正是「开了自动处置却一直不审」的根因）")
	}
}

// TestReviewProviderPrefersTaskChain 任务链优先于池：任务显式绑了档位就该用它。
func TestReviewProviderPrefersTaskChain(t *testing.T) {
	env := setupReviewProviderEnv(t)

	// 再造一条档位作为「任务链指定」，优先级压到最低，确保不是靠池顺序赢的。
	chainID, err := env.server.m.pg.SaveProfile(&db.LLMProfile{
		Name:    "review-provider-chain-test",
		Format:  "anthropic",
		BaseURL: "http://127.0.0.1:2",
		Model:   "chain-test-model",
		APIKey:  "test-key-chain",
	})
	if err != nil {
		t.Fatalf("SaveProfile chain: %v", err)
	}
	t.Cleanup(func() { _, _ = env.server.m.pg.Exec(`DELETE FROM llm_profiles WHERE id=$1`, chainID) })
	if _, err := env.server.m.pg.Exec(`UPDATE llm_profiles SET priority=-999 WHERE id=$1`, chainID); err != nil {
		t.Fatalf("lower chain priority: %v", err)
	}

	task := &Task{ID: "0", LLMProfileIDs: []int64{chainID}}
	rr := &ReviewRunner{s: env.server}
	if rr.reviewProvider(task) == nil {
		t.Fatalf("任务链里的可用档应被优先采用")
	}
}

// TestReviewProviderNilWhenAgentDisabled 关闭 review agent 就应彻底不审（明确返回 nil）。
func TestReviewProviderNilWhenAgentDisabled(t *testing.T) {
	env := setupReviewProviderEnv(t)
	if _, err := env.server.m.pg.Exec(`UPDATE agents SET enabled=false WHERE key='review'`); err != nil {
		t.Fatalf("disable review agent: %v", err)
	}
	rr := &ReviewRunner{s: env.server}
	if rr.reviewProvider(env.task) != nil {
		t.Fatalf("review agent 被禁用时必须返回 nil，不得绕过开关继续审")
	}
}
