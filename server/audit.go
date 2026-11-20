package server

import (
	"net/http"
	"strconv"
	"strings"

	"artex/db"
)

// 审计日志 API（/api/audit/logs）。
//
// 与上游 AntiFox 同源：登录/平台操作写 audit_logs 表，这里分页读回。
// 本仓库没有 RBAC 多用户，故 actor 统一为 "ARTEX"。

// GET /api/audit/logs?category=&actor=&action=&result=&limit=&offset=
func (s *Server) auditLogs(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	q := r.URL.Query()
	category := strings.TrimSpace(q.Get("category"))
	actor := strings.TrimSpace(q.Get("actor"))
	action := strings.TrimSpace(q.Get("action"))
	result := strings.TrimSpace(q.Get("result"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	logs, total, err := pg.ListAuditLogs(category, actor, action, result, limit, offset)
	if err != nil {
		writeErr(w, 500, "获取审计日志失败")
		return
	}
	if logs == nil {
		logs = []db.AuditLog{}
	}
	writeJSON(w, 200, map[string]any{"logs": logs, "total": total})
}
