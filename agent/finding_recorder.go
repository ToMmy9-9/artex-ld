// finding_recorder.go：finding 上报侧接线——recorder 接口、注入方法与流量证据指引文案。
// 主要函数/类型：
//   - FindingRecorder：report_finding 落库时的上报/登记回调接口。
//   - findingTrafficGuidance：report_finding 流量证据可选绑定的模型指引文案。
//   - SetFindingRecorder（ToolSet / Worker / Planner / MainAgent）：向各 agent 注入 recorder。
package agent

import (
	"context"
	"encoding/json"

	"artex/db"
)

// FindingRecorder is injected by the host; agents never synthesize or copy
// evidence bodies themselves. Its implementation owns the atomic write.
type FindingRecorder interface {
	Record(context.Context, db.RecordFindingInput, []db.TrafficRef) (*db.RecordedFinding, error)
}

// Tool-use guidance is appended without replacing the user's editable prompt.
// It does not require capture or claim that unavailable traffic tools exist.
const findingTrafficGuidance = "\n\n**漏洞流量证据（可选）**：调用 report_finding 上报漏洞时，如有已查看并确认支持漏洞结论的 HTTP 请求/响应，可用 traffic_refs 按复现顺序绑定真实 ID；域名和时间只作候选筛选，不推定关联。TCP 等非 HTTP 漏洞、未采集或无确切匹配时省略或传 []，在 evidence 保留命令输出、日志等其他可验证证据，建议说明未绑定原因。不要猜测 ID，也不要仅为补包重复探测。"

func (t *ToolSet) SetFindingRecorder(r FindingRecorder)   { t.findingRecorder = r }
func (w *Worker) SetFindingRecorder(r FindingRecorder)    { w.findingRecorder = r }
func (p *Planner) SetFindingRecorder(r FindingRecorder)   { p.findingRecorder = r }
func (m *MainAgent) SetFindingRecorder(r FindingRecorder) { m.findingRecorder = r }

// ── 【AntiFox 证据驱动交付】复现步骤 → 真实流量包 解析回调 ──
//
// resolveTraffic 把 report_finding 传入的结构化复现步骤映射到「当时真实录制的
// MITM 报文」，让每一步一开始就带上可跳转的真实流量链接（而不是事后由 AI 复述）。

// SetTrafficResolver wires the reproduction→traffic resolver on a Worker.
func (w *Worker) SetTrafficResolver(fn func(taskID int64, repro json.RawMessage) json.RawMessage) {
	w.resolveTraffic = fn
}

// SetTrafficResolver wires the reproduction→traffic resolver on a Planner.
func (p *Planner) SetTrafficResolver(fn func(taskID int64, repro json.RawMessage) json.RawMessage) {
	p.resolveTraffic = fn
}

// SetTrafficResolver wires the reproduction→traffic resolver on a MainAgent.
func (m *MainAgent) SetTrafficResolver(fn func(taskID int64, repro json.RawMessage) json.RawMessage) {
	m.resolveTraffic = fn
}
