package server

// 报告封面「客户名称」的查询参数契约。
//
// 这条链路三方名字不一致，是典型的静默丢参数缺陷：
//   - 前端 web/src/lib/api/misc.ts 传的是 ?client_name=...
//   - 后端 buildReportInput 只读 ?client=...，于是 client_name 被静默丢弃
//   - 而下游 reportx 的 meta 字段和 python 封面模板读的都是 client_name
//
// 结果：用户在报告对话框里填的客户名称永远进不了封面，封面只显示
// clientDefault(loc) 的占位文案（"客户"/"客戶"/"Client"），且整个过程
// 没有任何报错——最难被发现的那类缺陷。
//
// 这里把「前端怎么传，后端就得怎么收」钉死，避免以后再退回去。

import (
	"net/url"
	"strings"
	"testing"

	"artex/db"
)

// reportQueryInput 复现前端拼 query 的方式（web/src/lib/api/misc.ts reportGenerate）。
func reportQueryInput(kv map[string]string) map[string][]string {
	q := url.Values{}
	for k, v := range kv {
		q.Set(k, v)
	}
	return q
}

// reportTask 造一个带 Store 的最小任务：buildReportInput 会到 Store 里列 finding
// （KindFinding），Store 为 nil 会直接 panic。用一个不存在的 exploration id 让
// 结果为空集，测试就只关注客户名字段的解析，不被 finding 数据干扰。
func reportTask(pg *db.DB, desc string) *Task {
	return &Task{ID: "1", Name: "契约测试任务", Description: desc, Store: pg.Exploration(-1)}
}

// TestBuildReportInputAcceptsClientName 断言前端实际发送的 client_name 能被解析。
func TestBuildReportInputAcceptsClientName(t *testing.T) {
	s, pg := newTestServer(t)

	const want = "某某银行股份有限公司"
	in := s.buildReportInput(reportTask(pg, "内网 10.0.0.0/24"), reportQueryInput(map[string]string{"client_name": want}))

	if in.ClientName != want {
		t.Fatalf("client_name 未被解析：期望 %q，实际 %q（前端传 client_name，后端只认 client，参数被静默丢弃）",
			want, in.ClientName)
	}
}

// TestBuildReportInputAcceptsLegacyClient 兼容既有用法：手工调 API 时 ?client= 仍然有效。
func TestBuildReportInputAcceptsLegacyClient(t *testing.T) {
	s, pg := newTestServer(t)

	const want = "旧参数客户"
	in := s.buildReportInput(reportTask(pg, ""), reportQueryInput(map[string]string{"client": want}))

	if in.ClientName != want {
		t.Fatalf("?client= 兼容性被破坏：期望 %q，实际 %q", want, in.ClientName)
	}
}

// TestBuildReportInputClientNameWinsWhenBoth 两个参数同时出现时以 client_name 为准
// （前端正式契约优先于历史参数），避免用户看到的行为取决于参数顺序。
func TestBuildReportInputClientNameWinsWhenBoth(t *testing.T) {
	s, pg := newTestServer(t)

	in := s.buildReportInput(reportTask(pg, ""), reportQueryInput(map[string]string{
		"client_name": "新契约客户",
		"client":      "旧契约客户",
	}))

	if in.ClientName != "新契约客户" {
		t.Fatalf("client_name 应为最高优先级：实际 %q", in.ClientName)
	}
}

// TestBuildReportInputClientNameTrimmed 前后空白必须裁掉，否则封面会出现
// "   某某银行   " 这种带空格的客户名。
func TestBuildReportInputClientNameTrimmed(t *testing.T) {
	s, pg := newTestServer(t)

	in := s.buildReportInput(reportTask(pg, ""), reportQueryInput(map[string]string{
		"client_name": "  留白客户  ",
	}))

	if strings.TrimSpace(in.ClientName) != in.ClientName || in.ClientName != "留白客户" {
		t.Fatalf("客户名未裁剪空白：%q", in.ClientName)
	}
}
