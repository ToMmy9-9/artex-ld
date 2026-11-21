package server

// 客户名称从【HTTP 查询参数】一路走到【报告封面 meta】的端到端契约。
//
// report_client_param_test.go 只验证了「参数能被解析进 GenInput」；但真正让用户
// 看到效果的是 docx 封面 —— python 侧读的是 meta['client_name']。这中间还有
// BuildDoc 的归一化（空值会回退成 "客户"/"客戶"/"Client" 占位文案），
// 所以这里补上从 query 到 doc.Meta.ClientName 的整条链路。

import (
	"testing"

	"artex/reportx"
)

// TestClientNameReachesDocMeta 客户名必须原样进入报告 meta，不能被占位文案覆盖。
func TestClientNameReachesDocMeta(t *testing.T) {
	s, pg := newTestServer(t)

	const want = "某某银行股份有限公司"
	in := s.buildReportInput(reportTask(pg, "内网 10.0.0.0/24"),
		reportQueryInput(map[string]string{"client_name": want}))

	doc := reportx.BuildDoc(t.Context(), pg, in)
	if doc.Meta.ClientName != want {
		t.Fatalf("封面的客户名称 = %q，期望 %q（修复前这里会是占位文案「客户」）",
			doc.Meta.ClientName, want)
	}
}

// TestClientNameFallsBackToPlaceholder 没传客户名时仍应回退到占位文案，
// 避免封面出现空行。
func TestClientNameFallsBackToPlaceholder(t *testing.T) {
	s, pg := newTestServer(t)

	in := s.buildReportInput(reportTask(pg, ""), reportQueryInput(nil))
	doc := reportx.BuildDoc(t.Context(), pg, in)

	if doc.Meta.ClientName == "" {
		t.Fatal("未提供客户名时应回退到占位文案，而不是留空")
	}
}
