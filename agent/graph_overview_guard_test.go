package agent

import (
	"testing"

	"artex/db"
	"artex/guard"
)

// TestGraphOverviewIncludesGuardStats 校验 guard_stats 真的进了 graph_overview
// 的输出——这是"护栏统计回流 planner"的接线点，断了 planner 就又看不到被拦次数。
func TestGraphOverviewIncludesGuardStats(t *testing.T) {
	g := guard.New()
	g.RecordForTest("deny", "Bash")
	g.RecordForTest("deny", "Bash")

	ts := NewToolSet(nil, "planner")
	ts.SetGuardStats(func() any { return g.Stats() })
	out := ts.graphOverviewData()
	gs, ok := out["guard_stats"]
	if !ok {
		t.Fatalf("graph_overview has no guard_stats key; keys=%v", keysOf(out))
	}
	stats, ok := gs.(guard.GuardStats)
	if !ok {
		t.Fatalf("guard_stats type = %T", gs)
	}
	if stats.Denied != 2 {
		t.Fatalf("denied = %d, want 2", stats.Denied)
	}
}

// TestGraphOverviewOmitsGuardStatsWhenUnwired 未接线时不应出现空块，
// 否则每个老调用方都会多一个恒为 0 的噪声字段。
func TestGraphOverviewOmitsGuardStatsWhenUnwired(t *testing.T) {
	ts := NewToolSet(nil, "planner")
	if _, ok := ts.graphOverviewData()["guard_stats"]; ok {
		t.Fatal("guard_stats present without a resolver")
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

var _ = db.KindFact
