// toolassets_notify.go：外部工具可用时的主动通知。
//
// 背景：工具被 keeper 认领后，agent 的工具集**每轮装配都会重新读取**，所以
// 新工具在 agent 的下一个动作就能看到（这条路径已验证）。但"下一个动作"
// 对一个正在跑几十步的 worker 来说可能太晚——它可能整轮都在用旧手段。
//
// 这里补的是"主动告知"：keeper 认领完成时，往**正在运行的任务**的探索图上
// 挂一条 hint，并唤醒 planner。让正在跑的 worker/planner 立刻知道：
//
//	外部工具 <name> 已可用（分类/风险/调用方式/已禁用的能力）
//
// 通知与工具本身解耦：hint 只是提示，真正的可用性由 tools 表的绑定 +
// run_tool 的契约校验保证。
package server

import (
	"context"
	"fmt"
	"log"
	"strings"

	"artex/db"
)

// notifyToolAvailable announces a newly claimed tool to the running tasks.
//
// It is best-effort: a failure to notify must never fail the claim itself
// (the tool IS usable — the agent just learns about it a moment later).
func (s *Server) notifyToolAvailable(asset *db.ToolAsset) {
	if asset == nil || s.m.pg == nil {
		return
	}
	text := toolAvailableNotice(asset)
	tasks := s.runningTasks()
	if len(tasks) == 0 {
		return
	}
	notified := 0
	for _, t := range tasks {
		if t.Task == nil {
			continue
		}
		exp := s.m.pg.Exploration(t.Task.ExpID)
		if exp == nil {
			continue
		}
		// AddNode(KindHint, payload, 0, "active", "human", nil) — same shape the
		// in-task add_hint tool writes, so planner/worker read it the same way.
		if _, err := exp.AddNode(db.KindHint, map[string]any{"text": text}, 0, "active", "system", nil); err != nil {
			log.Printf("[tool-assets] 通知任务 %s 失败: %v", t.Name, err)
			continue
		}
		// Wake the planner so it can dispatch intent that uses the new tool.
		t.Task.NotifyHint([]string{text})
		notified++
	}
	if notified > 0 {
		log.Printf("[tool-assets] 已通知 %d 个运行中任务：外部工具 %s 可用", notified, asset.Name)
	}
}

// runningTaskRef is one running task's identity for notification.
type runningTaskRef struct {
	Task *Task
	Name string
}

// runningTasks lists the tasks currently in a running state.
//
// Only tasks that are actually executing get notified: a queued or paused task
// will pick the tool up on its next run anyway (tool sets are re-assembled
// every run), so notifying it would only be noise.
func (s *Server) runningTasks() []runningTaskRef {
	if s.m.pg == nil {
		return nil
	}
	rows, err := s.m.pg.Query(`SELECT id, name FROM tasks
		WHERE status = 'running' AND deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []runningTaskRef
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		t, ok := s.m.Task(fmt.Sprintf("%d", id))
		if !ok || t == nil {
			continue
		}
		out = append(out, runningTaskRef{Task: t, Name: name})
	}
	return out
}

// toolAvailableNotice renders the notice text for a newly claimed tool.
//
// It carries what an agent needs to decide whether and how to use the tool:
// what it is, how risky it is, how to call it, and — crucially — which
// dangerous capabilities the contract already removed.
func toolAvailableNotice(asset *db.ToolAsset) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【外部工具已可用】%s", asset.Name)
	if asset.Category != "" {
		fmt.Fprintf(&b, "（%s）", asset.Category)
	}
	if asset.RiskLevel != "" {
		fmt.Fprintf(&b, " 风险等级 %s", asset.RiskLevel)
	}
	b.WriteString("\n")
	if asset.Summary != "" {
		b.WriteString(asset.Summary + "\n")
	}
	if asset.UsageExample != "" {
		b.WriteString("调用示例: " + asset.UsageExample + "\n")
	}
	// The disabled capabilities are the part that matters most for a
	// decision: they tell the agent what the tool will NOT do.
	if c, err := db.ParseContract(asset.Contract); err == nil && len(c.DenyArgs) > 0 {
		b.WriteString("已被契约禁用（不可用）: " + strings.Join(c.DenyArgs, " ") + "\n")
	}
	if lim, err := db.ParseContract(asset.Limits); err == nil && lim.ScopeEnforcement != db.ScopeEnforced {
		if lim.ScopeEnforcement == db.ScopeAdvisory {
			b.WriteString("注意: 该工具不经过平台流量代理，平台无法拦截它访问越界目标，调用时必须自行严格限定在授权范围内。\n")
		} else {
			b.WriteString("注意: 该工具不作用于网络目标，无授权范围概念。\n")
		}
	}
	b.WriteString("请用 run_tool 以受管方式调用（平台会校验参数与目标范围），不要直接用 Bash 拼接命令行。")
	return b.String()
}

var _ = context.Background
