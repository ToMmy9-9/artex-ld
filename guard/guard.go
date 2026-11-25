// guard.go：安全边界层——审计日志、拦截规则评估（PreToolUse 钩子）与失败归因。
// 主要函数/类型：
//   - AuditEntry / Guard：审计条目与安全守卫主体。
//   - New / NewWithInterceptor / newGuard：构造 Guard。
//   - Hooks / preToolUse / applyIntercept / postToolUse：钩子注册与工具调用前中后拦截。
//   - Attributions / block / record / Audit：失败归因、阻断与审计查询。
//   - interceptorAPI / systemBlockMessage：拦截器抽象与系统阻断文案。
//
// Package guard implements the safety boundary layer (docs §11): an audit log,
// user-configured intercept-rule evaluation, and Observer/G5 failure attribution.
// Every tool call passes through the PreToolUse hook before executing.
// (The RoE authorization-scope mechanism was removed; a replacement may be added
// later.) Destructive/exfil gating is no longer hard-coded here — it lives in the
// DB intercept rules (seeded as ordinary [内置] rules, so users can disable or
// delete them), evaluated via applyIntercept.
package guard

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"artex/intercept"
	"norma/hook"
)

// AuditEntry records one gated tool call.
type AuditEntry struct {
	TS      int64  `json:"ts"`
	Tool    string `json:"tool"`
	Action  string `json:"action"` // allow|block
	Reason  string `json:"reason,omitempty"`
	Command string `json:"command,omitempty"`
}

// interceptorAPI 是 Guard 使用的审批能力；接口让审计逻辑可以独立做单元测试。
type interceptorAPI interface {
	IsToolEnabled(string) bool
	Match(string, []byte) (intercept.Decision, bool)
	Judge(context.Context, string, json.RawMessage) (intercept.Decision, bool)
	Log(context.Context, int64, intercept.Decision, string, []byte, string)
	HandleAsk(context.Context, int64, intercept.Decision, string, []byte) bool
}

// Guard enforces the side-effect policy via agent-core hooks.
type Guard struct {
	mu     sync.Mutex
	audit  []AuditEntry
	attrib map[string]int // failure attribution counts (Observer / G5)
	// decisions counts platform-governance outcomes per tool and action:
	// "deny:<tool>", "ask_allowed:<tool>", "ask_denied:<tool>", "allow:<tool>".
	// These are fed to the planner via graph_overview so it can see that a
	// direction is being blocked instead of re-trying it every round.
	decisions   map[string]int
	targetBlock map[string]int // target-side WAF/403 hits per tool (from postToolUse)
	reg         *hook.Registry
	interceptor interceptorAPI // optional; nil disables user-configured rules
}

// New creates a Guard without user-configured intercept rules (used for pentest
// tasks where the Interceptor is not yet available).
func New() *Guard { return newGuard(nil) }

// NewWithInterceptor creates a Guard with user-configured intercept rules.
func NewWithInterceptor(ic *intercept.Interceptor) *Guard {
	if ic == nil {
		return newGuard(nil)
	}
	return newGuard(ic)
}

func newGuard(ic interceptorAPI) *Guard {
	g := &Guard{
		attrib:      map[string]int{},
		decisions:   map[string]int{},
		targetBlock: map[string]int{},
		interceptor: ic,
	}
	g.reg = hook.NewRegistry().
		On(hook.PreToolUse, g.preToolUse).
		On(hook.PostToolUse, g.postToolUse)
	return g
}

// Hooks returns the hook registry to attach to an agent session.
func (g *Guard) Hooks() *hook.Registry { return g.reg }

func (g *Guard) preToolUse(ctx context.Context, ev hook.Event) hook.Result {
	// Extract the shell-command surface for the audit log: Bash + the interactive-shell
	// tools (shell_open's command, shell_send's text). Destructive/exfil gating is no
	// longer hard-coded here — it now lives in the DB intercept rules, evaluated by
	// applyIntercept below. Other tools record an empty command.
	var cmd string
	switch ev.ToolName {
	case "Bash", "shell_open":
		var in struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(ev.Input, &in)
		cmd = in.Command
	case "shell_send":
		var in struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(ev.Input, &in)
		cmd = in.Text
	}
	result := g.applyIntercept(ctx, ev)
	if result.Decision != "block" {
		g.record(ev.ToolName, "allow", "", cmd)
	}
	return result
}

// applyIntercept evaluates user-configured intercept rules against the tool call.
// Both rules and the fallback judge receive the complete tool input.
func (g *Guard) applyIntercept(ctx context.Context, ev hook.Event) hook.Result {
	if g.interceptor == nil {
		return hook.Result{}
	}
	// 【二开·审核流 PR1/§2.8】K15 审核写工具闸门来源校验：非 review 会话（如 planner
	// 调 add_review_intent 送审）直接绕过，不进规则匹配/LLM 判官——否则 planner 每次
	// 送审都会弹人工确认卡（种子规则把这三件工具加进了全局启用集）。
	if intercept.SkipReviewGateFor(ctx, ev.ToolName) {
		return hook.Result{}
	}
	if !g.interceptor.IsToolEnabled(ev.ToolName) {
		return hook.Result{}
	}
	ctx = intercept.WithCall(ctx, ev.ToolName, ev.Input)
	dec, matched := g.interceptor.Match(ev.ToolName, ev.Input)
	if !matched {
		// No rule matched. Ask the LLM fallback judge (if enabled); when it is off
		// or unwired, keep current behavior and allow.
		d, judged := g.interceptor.Judge(ctx, ev.ToolName, ev.Input)
		if !judged {
			return hook.Result{}
		}
		dec = d
	}
	switch dec.Action {
	case "deny":
		// 观测:deny 命中不阻塞审批,直接记一条 denied（历史/任务拦截页可见）。
		g.interceptor.Log(ctx, intercept.ConvIDFromContext(ctx), dec, ev.ToolName, ev.Input, "denied")
		g.count("deny:" + ev.ToolName)
		return g.block(ev.ToolName, systemBlockMessage(dec.Message), "")
	case "allow":
		// Record explicit rule and model approvals so review details remain auditable.
		g.interceptor.Log(ctx, intercept.ConvIDFromContext(ctx), dec, ev.ToolName, ev.Input, "allowed")
		g.count("allow:" + ev.ToolName)
		return hook.Result{}
	case "ask":
		// If the worker context is already cancelled (task stopped / killed), block
		// immediately without creating a pending record — avoids orphaned DB entries
		// and makes execOne complete fast, reducing the race against drainSynthetic.
		if ctx.Err() != nil {
			g.count("ask_denied:" + ev.ToolName)
			return g.block(ev.ToolName, systemBlockMessage("工作已取消，平台安全管控阻止执行"), "")
		}
		convID := intercept.ConvIDFromContext(ctx)
		if !g.interceptor.HandleAsk(ctx, convID, dec, ev.ToolName, ev.Input) {
			g.count("ask_denied:" + ev.ToolName)
			return g.block(ev.ToolName, systemBlockMessage("人工审批未通过（用户拒绝或审批超时）"), "")
		}
		g.count("ask_allowed:" + ev.ToolName)
		return hook.Result{}
	}
	return hook.Result{}
}

// systemBlockMessage frames an intercept block as an ARTEX platform-governance
// decision so the agent does not mistake it for a target-side defense.
//
// The bare reasons ("禁止执行此工具" / "用户拒绝") read exactly like a WAF/403 on
// the target, so a pentest agent's instinct is to bypass them — rewrite the
// command, swap the payload, re-encode, retry. That is both futile (the platform
// blocks the class of action, not one string) and wrong (it's a policy decision,
// not an obstacle to defeat). This prefix states plainly that the block comes
// from the platform, is not the target's protection, and that the operation is
// forbidden — so the agent pivots to another approach instead of evading it.
// Audit/history rows keep the raw reason (see Interceptor.Log); only the
// model-facing tool_result carries this framing.
func systemBlockMessage(reason string) string {
	return "【artex 平台管控·非目标防御】此调用被平台拦截。" +
		"原因：" + reason + "。此操作被禁止。"
}

var reBlocked = regexp.MustCompile(`(?i)\b(403|forbidden|waf|blocked|rate.?limit|429|captcha|denied)\b`)

// postToolUse is the Observer failure-attribution hook (G5): it classifies tool
// results into blocked / error / ok so the planner can change strategy instead
// of giving up at a WAF.
func (g *Guard) postToolUse(_ context.Context, ev hook.Event) hook.Result {
	if ev.ToolName != "Bash" {
		return hook.Result{}
	}
	class := "ok"
	switch {
	case reBlocked.Match(ev.Result):
		class = "blocked"
	case ev.IsError:
		class = "error"
	}
	g.mu.Lock()
	g.attrib[class]++
	if class == "blocked" {
		g.targetBlock[ev.ToolName]++
	}
	g.mu.Unlock()
	return hook.Result{}
}

// count increments one decision counter. Internal to the guard's mutex; callers
// must not hold g.mu.
func (g *Guard) count(key string) {
	g.mu.Lock()
	if g.decisions == nil {
		g.decisions = map[string]int{}
	}
	g.decisions[key]++
	g.mu.Unlock()
}

// GuardStats is the planner-facing summary of platform governance activity for
// one task. Counts are cumulative for the task's lifetime (the same guard object
// lives as long as the task).
type GuardStats struct {
	// Denied / AskDenied / AskAllowed / Allowed are totals across all tools.
	Denied     int `json:"denied"`
	AskDenied  int `json:"ask_denied"`
	AskAllowed int `json:"ask_allowed"`
	Allowed    int `json:"allowed"`
	// TargetBlocked counts tool results that looked like a target-side WAF/403/429.
	TargetBlocked int `json:"target_blocked"`
	// ByTool is a compact per-tool breakdown, largest first, capped at 10 entries.
	ByTool []ToolDecision `json:"by_tool,omitempty"`
}

// ToolDecision is one row of the per-tool breakdown.
type ToolDecision struct {
	Tool       string `json:"tool"`
	Denied     int    `json:"denied,omitempty"`
	AskDenied  int    `json:"ask_denied,omitempty"`
	AskAllowed int    `json:"ask_allowed,omitempty"`
	Allowed    int    `json:"allowed,omitempty"`
	// TargetBlocked is the target-side WAF/403 count for this tool.
	TargetBlocked int `json:"target_blocked,omitempty"`
}

// Stats returns the task's governance summary. A nil Guard returns a zero value
// so callers need no nil check.
func (g *Guard) Stats() GuardStats {
	var out GuardStats
	if g == nil {
		return out
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	byTool := map[string]*ToolDecision{}
	row := func(tool string) *ToolDecision {
		d, ok := byTool[tool]
		if !ok {
			d = &ToolDecision{Tool: tool}
			byTool[tool] = d
		}
		return d
	}
	for key, n := range g.decisions {
		action, tool, ok := cutOnce(key, ":")
		if !ok {
			continue
		}
		switch action {
		case "deny":
			out.Denied += n
			row(tool).Denied += n
		case "ask_denied":
			out.AskDenied += n
			row(tool).AskDenied += n
		case "ask_allowed":
			out.AskAllowed += n
			row(tool).AskAllowed += n
		case "allow":
			out.Allowed += n
			row(tool).Allowed += n
		}
	}
	for tool, n := range g.targetBlock {
		out.TargetBlocked += n
		row(tool).TargetBlocked += n
	}
	for _, d := range byTool {
		out.ByTool = append(out.ByTool, *d)
	}
	sort.Slice(out.ByTool, func(i, j int) bool {
		a, b := out.ByTool[i], out.ByTool[j]
		sa := a.Denied + a.AskDenied + a.AskAllowed + a.TargetBlocked
		sb := b.Denied + b.AskDenied + b.AskAllowed + b.TargetBlocked
		if sa != sb {
			return sa > sb
		}
		return a.Tool < b.Tool
	})
	if len(out.ByTool) > 10 {
		out.ByTool = out.ByTool[:10]
	}
	return out
}

// cutOnce splits s at the first occurrence of sep.
func cutOnce(s, sep string) (before, after string, ok bool) {
	i := strings.Index(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

// Attributions returns failure-attribution counts (Observer / G5).
func (g *Guard) Attributions() map[string]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]int, len(g.attrib))
	for k, v := range g.attrib {
		out[k] = v
	}
	return out
}

func (g *Guard) block(tool, reason, cmd string) hook.Result {
	g.record(tool, "block", reason, cmd)
	return hook.Result{Decision: "block", Message: reason}
}

func (g *Guard) record(tool, action, reason, cmd string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.audit = append(g.audit, AuditEntry{TS: time.Now().Unix(), Tool: tool, Action: action, Reason: reason, Command: cmd})
	if len(g.audit) > 2000 {
		g.audit = g.audit[len(g.audit)-2000:]
	}
}

// Audit returns a snapshot of recent gated calls (most recent last).
func (g *Guard) Audit() []AuditEntry {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]AuditEntry, len(g.audit))
	copy(out, g.audit)
	return out
}

// RecordForTest increments one decision counter. Exported only so other packages'
// tests can exercise the plumbing without an interceptor; production code paths
// go through applyIntercept / postToolUse.
func (g *Guard) RecordForTest(action, tool string) {
	if g == nil {
		return
	}
	g.count(action + ":" + tool)
}
