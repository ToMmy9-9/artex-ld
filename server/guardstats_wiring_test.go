// guardstats_wiring_test.go：钉住「护栏统计回流 planner」在 server 侧的接线。
//
// guard 早就在累计 allow/deny/ask 与目标侧 WAF 命中，但没有任何代码读取它，
// 于是 planner 看不到"这个方向连续被平台打回"，每轮重复派同样的意图。这里验证
// 生产用的 resolver 表达式本身：有 Guard 时给出真实计数，无 Guard 时给 nil
// （nil 会让 graph_overview 省略整块，而不是输出一堆 0 误导模型）。
package server

import (
	"testing"

	"artex/guard"
)

func TestGuardStatsResolverReturnsTaskCounters(t *testing.T) {
	g := guard.New()
	g.RecordForTest("deny", "Bash")
	g.RecordForTest("deny", "Bash")
	g.RecordForTest("ask_allowed", "shell_send")

	task := &Task{Guard: g}
	got := guardStatsResolver(task)()
	stats, ok := got.(guard.GuardStats)
	if !ok {
		t.Fatalf("resolver returned %T, want guard.GuardStats", got)
	}
	if stats.Denied != 2 || stats.AskAllowed != 1 {
		t.Fatalf("stats = %+v, want denied=2 ask_allowed=1", stats)
	}
}

func TestGuardStatsResolverNilTaskOrGuard(t *testing.T) {
	if v := guardStatsResolver(nil)(); v != nil {
		t.Fatalf("nil task must resolve to nil, got %v", v)
	}
	if v := guardStatsResolver(&Task{})(); v != nil {
		t.Fatalf("task without a Guard must resolve to nil, got %v", v)
	}
}
