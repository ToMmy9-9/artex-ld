package server

import (
	"testing"

	"artex/agent"
)

// TestPlannerFailStateThreshold 验证「连续 planner 失败到阈值才判失败」的行为。
//
// 背景：planner 一直出错（LLM 配置坏、上游持续 5xx）时，任务会永远停在 running，
// 而 running 的任务占着并发槽 —— 开启并发上限后几个卡死的任务就能堵死整个队列。
// 偶发失败则不应判死（网络抖动很常见），所以必须有阈值 + 成功后清零。
func TestPlannerFailStateThreshold(t *testing.T) {
	var p plannerFailState
	for i := 1; i < plannerFailureThreshold; i++ {
		if p.record() {
			t.Fatalf("第 %d 次失败就判死了，阈值应为 %d", i, plannerFailureThreshold)
		}
	}
	if !p.record() {
		t.Fatalf("第 %d 次失败应达到阈值", plannerFailureThreshold)
	}
	if p.streak != plannerFailureThreshold {
		t.Fatalf("streak=%d, want %d", p.streak, plannerFailureThreshold)
	}
}

// TestPlannerFailStateResetOnSuccess 验证一轮成功规划会把失败计数清零，
// 免得「失败 4 轮 + 成功 1 轮 + 失败 4 轮」被累计成 8 轮而误判。
func TestPlannerFailStateResetOnSuccess(t *testing.T) {
	var p plannerFailState
	for i := 0; i < plannerFailureThreshold-1; i++ {
		p.record()
	}
	p.reset()
	if p.streak != 0 {
		t.Fatalf("reset 后 streak=%d, want 0", p.streak)
	}
	// 清零后再失败，计数应从头开始。
	for i := 0; i < plannerFailureThreshold-1; i++ {
		if p.record() {
			t.Fatalf("reset 后第 %d 次失败就判死了", i+1)
		}
	}
	if !p.record() {
		t.Fatal("reset 后累计到阈值应判死")
	}
}

// TestAbortPlannerFailedCause 验证新增的中止原因能被识别，且语义与 goal_met 区分开
// （goal_met 的意图标记为 stopped，planner_failed 的应标记为 failed）。
func TestAbortPlannerFailedCause(t *testing.T) {
	if agent.AbortPlannerFailed == nil {
		t.Fatal("AbortPlannerFailed 未定义")
	}
	code, short, text := agent.AbortPlannerFailed.Code, agent.AbortPlannerFailed.Short, agent.AbortPlannerFailed.Text
	if code != "planner_failed" {
		t.Fatalf("code=%q, want planner_failed", code)
	}
	if short == "" || text == "" {
		t.Fatalf("short=%q text=%q, 均不应为空", short, text)
	}
	if code == agent.AbortGoalMet.Code {
		t.Fatal("planner_failed 不应与 goal_met 同码")
	}
}
