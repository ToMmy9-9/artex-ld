// constraints_api.go：任务约束（Constraint）的增删改查 HTTP API 与 DTO 转换。
// 主要函数/类型：
//   - listConstraints / addConstraint / editConstraint / deleteConstraint：约束列表/新增/编辑/删除。
//   - constraintInjectPlanner / constraintInjectWorker：约束是否注入 planner / worker prompt 的开关。
//   - normalizeConstraintKind / constraintDTOs：约束类型归一 / DTO 转换。
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"artex/db"
)

// 总览「约束管理」的人工 CRUD 接口 + 注入范围开关的解析。操作约束(allow/deny)与 agent 侧的
// set_constraints 工具写同一张 task_constraints 表;这里是人类在 UI 上直接增删改。约束仅是
// 提示上下文——增删改后【不】通知 planner,下一轮规划自然读库生效(按产品决策)。每个变更 handler
// 都走 beginTaskOperation/decInflight,避免与任务删除竞态(与目标/意图 CRUD 一致)。

// 注入范围开关的 settings key,默认都开(GetBool 第二参数 = true)。
const (
	settingConstraintsInjectPlanner = "constraints_inject_planner"
	settingConstraintsInjectWorker  = "constraints_inject_worker"
	// settingLogLevel 【P-LOG·M0】日志级别热切换键：debug/info/warn/error。
	settingLogLevel = "log_level"

	// settingInterceptBlockMessage 是「平台拦截」提醒的可配置模板。
	// 空 = 内置默认模板（与历史文案等价）。变量：{tool} {rule} {reason} {command} {action}。
	settingInterceptBlockMessage = "intercept_block_message"

	// settingInterceptTrafficEvidence 控制拦截提醒是否附带真实流量证据（默认开）。
	settingInterceptTrafficEvidence = "intercept_traffic_evidence"
)

// constraintInjectPlanner / constraintInjectWorker 报告是否把操作约束注入对应 agent 的
// 系统提示(默认开)。作为 resolver 传给 planner/worker,每轮读 → 改开关即时生效。
func (s *Server) constraintInjectPlanner() bool {
	return s.m.pg.GetBool(settingConstraintsInjectPlanner, true)
}

func (s *Server) constraintInjectWorker() bool {
	return s.m.pg.GetBool(settingConstraintsInjectWorker, true)
}

// listConstraints 返回本任务的全部操作约束(allow 在前、deny 在后)。
func (s *Server) listConstraints(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "任务不存在")
		return
	}
	rows, err := t.Store.ListConstraints()
	if err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"constraints": constraintDTOs(rows)})
}

// addConstraint 人工新增一条操作约束(kind=allow|deny)。不通知 planner。
func (s *Server) addConstraint(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "任务不存在")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, "任务正在删除,无法新增约束")
		return
	}
	defer s.engine.decInflight(t.ID)

	var body struct {
		Text string `json:"text"`
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求体不是合法的 JSON")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeErr(w, 400, "约束内容不能为空")
		return
	}
	kind := normalizeConstraintKind(body.Kind)
	if kind == "" {
		writeErr(w, 400, "kind 必须是 allow 或 deny")
		return
	}
	id, err := t.Store.AddConstraint(kind, text, "human")
	if err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ConstraintDTO{ID: strconv.FormatInt(id, 10), Kind: kind, Text: text, Origin: "human"})
}

// editConstraint 人工修改一条约束(kind + text)。不通知 planner。
func (s *Server) editConstraint(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "任务不存在")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, "任务正在删除,无法修改约束")
		return
	}
	defer s.engine.decInflight(t.ID)

	cid, err := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil || cid <= 0 {
		writeErr(w, 400, "约束 id 无效")
		return
	}
	var body struct {
		Text string `json:"text"`
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "请求体不是合法的 JSON")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeErr(w, 400, "约束内容不能为空")
		return
	}
	kind := normalizeConstraintKind(body.Kind)
	if kind == "" {
		writeErr(w, 400, "kind 必须是 allow 或 deny")
		return
	}
	if err := t.Store.UpdateConstraint(cid, kind, text); err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ConstraintDTO{ID: strconv.FormatInt(cid, 10), Kind: kind, Text: text})
}

// deleteConstraint 人工删除一条约束。不通知 planner。
func (s *Server) deleteConstraint(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "任务不存在")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, "任务正在删除,无法删除约束")
		return
	}
	defer s.engine.decInflight(t.ID)

	cid, err := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil || cid <= 0 {
		writeErr(w, 400, "约束 id 无效")
		return
	}
	if err := t.Store.DeleteConstraint(cid); err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// normalizeConstraintKind lowercases + validates the kind; "" on invalid.
func normalizeConstraintKind(k string) string {
	k = strings.TrimSpace(strings.ToLower(k))
	if k == "allow" || k == "deny" {
		return k
	}
	return ""
}

// constraintDTOs converts db rows to the frontend shape.
func constraintDTOs(in []db.Constraint) []ConstraintDTO {
	out := make([]ConstraintDTO, 0, len(in))
	for _, c := range in {
		out = append(out, ConstraintDTO{
			ID:     strconv.FormatInt(c.ID, 10),
			Kind:   c.Kind,
			Text:   c.Text,
			Origin: c.Origin,
			TS:     rfc3339(c.CreatedAt),
		})
	}
	return out
}

// interceptBlockMessage 返回当前生效的平台拦截提醒模板。空串 = 用内置默认
// (guard.DefaultBlockMessageTemplate)。每轮现读,改设置不必重启。
func (s *Server) interceptBlockMessage() string {
	v, _, _ := s.m.pg.GetSetting(settingInterceptBlockMessage)
	return strings.TrimSpace(v)
}

// interceptTrafficEvidenceEnabled 报告拦截提醒是否附带真实流量证据(默认开)。
func (s *Server) interceptTrafficEvidenceEnabled() bool {
	return s.m.pg.GetBool(settingInterceptTrafficEvidence, true)
}
