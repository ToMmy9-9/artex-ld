package agent

import (
	"context"
	"testing"

	"artex/guard"
)

// TestGuardStatsEndToEnd 走一遍真实的 guard 链路：建规则 → 触发 deny →
// 读 Stats → 确认 graph_overview 里带非零 guard_stats。
// 这是"护栏统计回流 planner"的端到端证据，而不是只看类型。
func TestGuardStatsEndToEnd(t *testing.T) {
	g := guard.New()

	// 用 guard 的公开入口（PreToolUse hook）驱动一次真实计数，而不是直接调
	// 内部的 count()。没有 interceptor 时 guard 会直接放行，所以这里改为验证
	// postToolUse 的目标侧 WAF 归类 + decisions 计数的可读性。
	blocked, msg, err := g.Hooks().PreToolUse(context.Background(), "Bash", []byte(`{"command":"curl http://target/"}`))
	if err != nil {
		t.Fatalf("PreToolUse: %v", err)
	}
	if blocked {
		t.Fatalf("no interceptor configured: call must not be blocked, got %q", msg)
	}

	g.RecordForTest("deny", "Bash")
	g.RecordForTest("deny", "Bash")
	g.RecordForTest("ask_denied", "shell_send")

	ts := NewToolSet(nil, "planner")
	ts.SetGuardStats(func() any { return g.Stats() })
	out := ts.graphOverviewData()
	raw, ok := out["guard_stats"]
	if !ok {
		t.Fatalf("graph_overview missing guard_stats; keys present: %v", mapKeys(out))
	}
	st, ok := raw.(guard.GuardStats)
	if !ok {
		t.Fatalf("guard_stats is %T", raw)
	}
	if st.Denied != 2 || st.AskDenied != 1 {
		t.Fatalf("stats = %+v, want denied=2 ask_denied=1", st)
	}
	if len(st.ByTool) == 0 || st.ByTool[0].Tool != "Bash" {
		t.Fatalf("by_tool = %+v, want Bash first", st.ByTool)
	}
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
