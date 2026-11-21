package server

// ═══ [同类 bug 排查 · 裁判模型池兜底] ═══
// 与 reviewProvider 的池兜底是同一个根因：llm_judge_profile_id 默认 0 表示
// 「跟随全局默认」，实现只查 ActiveProfile()。而库里完全可以没有任何 is_default
// （实测就是 2 条 profile、0 条 is_default），于是返回 nil → 直接报「未配置可用的
// 裁判模型」→ 所有工具审批全线失败，可池里明明躺着可用模型。
// 二开日期：2026-11-21

import (
	"testing"

	"artex/db"
)

// TestResolveJudgeProfileIDFallsBackToPool 核心回归：配置为 0、无 is_default、
// 池里有可用档时，必须解析出一个真实 id，而不是 0。
func TestResolveJudgeProfileIDFallsBackToPool(t *testing.T) {
	s, pg := newTestServer(t)

	// 快照原默认并在结束时还原（SetActiveProfile/清空都会动它）。
	snapshotActiveLLMProfile(t, pg)

	// 造一个可用档（有 key、非 jev），并把它顶到池首。
	pid, err := pg.SaveProfile(&db.LLMProfile{
		Name:    "judge-fallback-test",
		Format:  "anthropic",
		BaseURL: "http://127.0.0.1:1",
		Model:   "judge-fallback-model",
		APIKey:  "test-key-judge",
	})
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	t.Cleanup(func() { _, _ = pg.Exec(`DELETE FROM llm_profiles WHERE id=$1`, pid) })
	if _, err := pg.Exec(`UPDATE llm_profiles SET priority=999 WHERE id=$1`, pid); err != nil {
		t.Fatalf("set priority: %v", err)
	}

	// 清掉所有 is_default —— 复现「库里一条默认都没有」的真实状态。
	if _, err := pg.Exec(`UPDATE llm_profiles SET is_default=false`); err != nil {
		t.Fatalf("clear defaults: %v", err)
	}
	if act, _ := pg.ActiveProfile(); act != nil {
		t.Fatalf("前置条件不成立：仍有 is_default 档 id=%d", act.ID)
	}

	got := s.resolveJudgeProfileID(0)
	if got == 0 {
		t.Fatal("无 is_default 但池里有可用档时，resolveJudgeProfileID 必须回落到池，" +
			"实际返回 0（这会让所有工具审批报「未配置可用的裁判模型」）")
	}
	if p, err := pg.ProfileByID(got); err != nil || p == nil {
		t.Fatalf("回落到的 id=%d 查不到对应档位: %v", got, err)
	}
}

// TestResolveJudgeProfileIDPrefersExplicit 显式配置优先于一切回落。
func TestResolveJudgeProfileIDPrefersExplicit(t *testing.T) {
	s, pg := newTestServer(t)

	pid, err := pg.SaveProfile(&db.LLMProfile{
		Name: "judge-explicit-test", Format: "anthropic", Model: "m", APIKey: "k",
	})
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	t.Cleanup(func() { _, _ = pg.Exec(`DELETE FROM llm_profiles WHERE id=$1`, pid) })

	if got := s.resolveJudgeProfileID(pid); got != pid {
		t.Fatalf("显式配置的 id=%d 未被尊重，实际返回 %d", pid, got)
	}
}

// TestResolveJudgeProfileIDUsesActive 有 is_default 时用它，不必落到池里。
func TestResolveJudgeProfileIDUsesActive(t *testing.T) {
	s, pg := newTestServer(t)

	snapshotActiveLLMProfile(t, pg)
	pid, err := pg.SaveProfile(&db.LLMProfile{
		Name: "judge-active-test", Format: "anthropic", Model: "m", APIKey: "k",
	})
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	t.Cleanup(func() { _, _ = pg.Exec(`DELETE FROM llm_profiles WHERE id=$1`, pid) })
	if err := pg.SetActiveProfile(pid); err != nil {
		t.Fatalf("SetActiveProfile: %v", err)
	}

	if got := s.resolveJudgeProfileID(0); got != pid {
		t.Fatalf("有 is_default 时应返回 id=%d，实际 %d", pid, got)
	}
}

// TestResolveJudgeProfileIDEmptyPool 池也空时返回 0，由调用方报出可读错误。
func TestResolveJudgeProfileIDEmptyPool(t *testing.T) {
	s, pg := newTestServer(t)

	snapshotActiveLLMProfile(t, pg)
	// 把现有档全部排除出池并清空默认，模拟「确实一个可用模型都没有」。
	rows, err := pg.Query(`SELECT id FROM llm_profiles`)
	if err != nil {
		t.Fatalf("query profiles: %v", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatalf("scan: %v", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = pg.Exec(`UPDATE llm_profiles SET pool_exclude=false WHERE id=$1`, id)
		}
	})
	if _, err := pg.Exec(`UPDATE llm_profiles SET is_default=false, pool_exclude=true`); err != nil {
		t.Fatalf("quarantine profiles: %v", err)
	}

	if got := s.resolveJudgeProfileID(0); got != 0 {
		t.Fatalf("无任何可用档时应返回 0，实际 %d", got)
	}
}
