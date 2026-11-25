package guard

import "testing"

// TestGuardStatsCountsDecisions 校验 platform-governance 计数按工具归类，
// 并计入目标侧 WAF 命中（这一类来自 postToolUse，不是拦截规则）。
func TestGuardStatsCountsDecisions(t *testing.T) {
	g := newGuard(nil)
	g.count("deny:Bash")
	g.count("deny:Bash")
	g.count("ask_allowed:Bash")
	g.count("ask_denied:shell_send")
	g.count("allow:list_assets")

	g.mu.Lock()
	g.targetBlock["Bash"] += 3
	g.mu.Unlock()

	st := g.Stats()
	if st.Denied != 2 || st.AskAllowed != 1 || st.AskDenied != 1 || st.Allowed != 1 {
		t.Fatalf("totals = %+v, want denied=2 ask_allowed=1 ask_denied=1 allowed=1", st)
	}
	if st.TargetBlocked != 3 {
		t.Fatalf("target_blocked = %d, want 3", st.TargetBlocked)
	}
	if len(st.ByTool) != 3 {
		t.Fatalf("by_tool rows = %d, want 3 (%+v)", len(st.ByTool), st.ByTool)
	}
	// Bash 的合计最高（2 deny + 3 target_blocked），必须排第一，planner 才先看到它。
	if st.ByTool[0].Tool != "Bash" {
		t.Fatalf("by_tool[0] = %q, want Bash (highest total first)", st.ByTool[0].Tool)
	}
	if st.ByTool[0].Denied != 2 || st.ByTool[0].TargetBlocked != 3 {
		t.Fatalf("Bash row = %+v, want denied=2 target_blocked=3", st.ByTool[0])
	}
}

// TestGuardStatsNilSafe nil Guard 必须返回零值，调用方无需判空。
func TestGuardStatsNilSafe(t *testing.T) {
	var g *Guard
	st := g.Stats()
	if st.Denied != 0 || st.ByTool != nil {
		t.Fatalf("nil guard stats = %+v, want zero", st)
	}
}

// TestGuardStatsCapsToolRows 分工具明细最多 10 条，避免把上下文撑爆。
func TestGuardStatsCapsToolRows(t *testing.T) {
	g := newGuard(nil)
	for _, tool := range []string{
		"t01", "t02", "t03", "t04", "t05", "t06", "t07", "t08", "t09", "t10", "t11", "t12",
	} {
		g.count("deny:" + tool)
	}
	st := g.Stats()
	if len(st.ByTool) != 10 {
		t.Fatalf("by_tool rows = %d, want 10", len(st.ByTool))
	}
	if st.Denied != 12 {
		t.Fatalf("denied total = %d, want 12 (totals must not be capped)", st.Denied)
	}
}
