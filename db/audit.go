package db

import (
	"encoding/json"
	"strconv"
	"time"
)

// 审计日志数据层（登录/平台操作审计；后续可扩展为完整审计中心）。
//
// 与上游 AntiFox 同源：本仓库没有 RBAC 多用户与 rbac_users 表，故审计主体
// （actor）统一记 "ARTEX"（本仓库唯一的管理员账号），不引入用户表。

type AuditLog struct {
	ID        int64          `json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	Actor     string         `json:"actor"`
	Category  string         `json:"category"`
	Action    string         `json:"action"`
	Result    string         `json:"result"`
	IP        string         `json:"ip"`
	Message   string         `json:"message"`
	Detail    map[string]any `json:"detail"`
}

// RecordAudit writes one audit log row (best-effort; a failure is swallowed so
// it can never break the operation being audited).
func (d *DB) RecordAudit(actor, category, action, result, ip, message string, detail map[string]any) {
	if result == "" {
		result = "success"
	}
	var dj []byte
	if detail != nil {
		dj, _ = json.Marshal(detail)
	} else {
		dj = []byte("{}")
	}
	_, _ = d.Exec(`INSERT INTO audit_logs(actor,category,action,result,ip,message,detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`, actor, category, action, result, ip, message, string(dj))
}

// ListAuditLogs pages audit logs newest-first (filterable by category/actor/
// action/result). Returns the rows and the total matching count.
func (d *DB) ListAuditLogs(category, actor, action, result string, limit, offset int) ([]AuditLog, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	where := "WHERE 1=1"
	args := []any{}
	add := func(col, val string) {
		if val == "" {
			return
		}
		args = append(args, val)
		where += " AND " + col + "=$" + strconv.Itoa(len(args))
	}
	add("category", category)
	add("actor", actor)
	add("action", action)
	add("result", result)

	var total int
	if err := d.QueryRow(`SELECT count(*) FROM audit_logs `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := d.Query(`SELECT id,created_at,actor,category,action,result,ip,message,detail
		FROM audit_logs `+where+` ORDER BY id DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []AuditLog{}
	for rows.Next() {
		var l AuditLog
		var detail []byte
		if err := rows.Scan(&l.ID, &l.CreatedAt, &l.Actor, &l.Category, &l.Action, &l.Result, &l.IP, &l.Message, &detail); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(detail, &l.Detail)
		out = append(out, l)
	}
	return out, total, rows.Err()
}
