// route_contract_test.go：路由契约回归 —— 防止「handler 存在但路由没注册」再次发生。
//
// 背景：合并 AntiFox 时只移植了它的 handler 文件（server/knowledge.go 等），
// 却漏掉了注册这些 handler 的路由行（原本在 AntiFox 的 server.go 里）。结果
// 知识库的「编辑/删除单条文档」「从转录补录漏洞」「漏洞↔原始流量包关联」
// 在运行时全部 404 —— 而 go build 与 go vet 都发现不了，单元测试也没覆盖到。
//
// 这里用「拿一个真实 Server，把请求打进去，看响应是不是 Go 默认的
// 404 page not found」来判定路由是否注册：ServeMux 未注册的 pattern 会返回
// 默认的纯文本 404，已注册但资源不存在则返回 JSON 错误体，两者可区分。
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// muxRouteContract 是「必须已注册」的路由清单。每条都用一个明显不存在的
// 资源 id 去打：路由存在 → 返回 JSON 错误体；路由没注册 → 返回 Go 默认
// 的 "404 page not found" 纯文本。
var muxRouteContract = []struct {
	method string
	path   string
	why    string
}{
	// ── AntiFox 知识库：单条文档的读/改/删（前端「编辑」「删除」按钮）──
	{"GET", "/api/knowledge/{id}", "知识库单条读取"},
	{"PUT", "/api/knowledge/{id}", "知识库单条更新"},
	{"DELETE", "/api/knowledge/{id}", "知识库单条删除"},

	// ── AntiFox 漏洞补录：从任务会话转录回填已确认漏洞 ──
	{"POST", "/api/tasks/{id}/extract-findings", "从转录补录漏洞"},

	// ── AntiFox 原始流量包级关联（与 artex-ld 的快照式 /traffic 并存）──
	{"GET", "/api/exploration/findings/{id}/traffic-exchanges", "列出关联的原始流量包"},
	{"POST", "/api/exploration/findings/{id}/traffic-exchanges", "关联原始流量包"},
	{"DELETE", "/api/exploration/findings/{id}/traffic-exchanges/{exchange_id}", "解除原始流量包关联"},

	// ── AntiFox 证据驱动交付：富化 / 证据截图 ──
	{"GET", "/api/exploration/findings/{id}/detail", "漏洞详情（富化+复现过程）"},
	{"POST", "/api/exploration/findings/{id}/enrich", "触发 AI 富化"},
	{"GET", "/api/exploration/findings/{id}/enrich/status", "查询富化状态"},
	{"PUT", "/api/exploration/findings/{id}", "更新漏洞状态/名称"},
	{"POST", "/api/exploration/findings/{id}/evidence", "上传证据截图"},
	{"GET", "/api/exploration/findings/{id}/evidence", "列出证据截图"},
	{"GET", "/api/exploration/findings/{id}/evidence/{file}", "读取证据截图文件"},
	{"PUT", "/api/exploration/findings/{id}/evidence/{eid}", "更新证据图注"},
	{"DELETE", "/api/exploration/findings/{id}/evidence/{eid}", "删除证据截图"},

	// ── 发现列表：全选全部页所需的筛选命中 id（字面量段，必须压过 {id} 通配）──
	{"GET", "/api/exploration/findings/ids", "全选全部页的筛选命中 id"},

	// ── AntiFox 交付级报告 ──
	{"POST", "/api/report/generate", "生成交付级报告"},
	{"GET", "/api/report/status", "报告生成状态"},
	{"GET", "/api/report-settings", "读取报告出具方设置"},
	{"PUT", "/api/report-settings", "保存报告出具方设置"},
	{"GET", "/api/reports", "已生成报告清单"},
	{"GET", "/api/report-templates", "报告模版清单"},

	// ── AntiFox 知识库其余端点 ──
	{"GET", "/api/knowledge", "知识文档清单"},
	{"POST", "/api/knowledge", "新建知识文档"},
	{"POST", "/api/knowledge/upload", "上传知识文档"},
	{"GET", "/api/knowledge/categories", "知识分类枚举"},
	{"POST", "/api/knowledge/search", "知识检索"},
	{"GET", "/api/knowledge/logs", "检索日志"},
	{"GET", "/api/knowledge/config", "Embedder 配置"},
	{"PUT", "/api/knowledge/config", "保存 Embedder 配置"},
	{"POST", "/api/knowledge/config/test", "测试 Embedder 连通"},
	{"POST", "/api/knowledge/reindex", "重建索引"},

	// ── artex-ld 自有的快照式流量证据绑定（与上面的 exchange 级并存）──
	{"GET", "/api/exploration/findings/{id}/traffic", "列出快照式流量证据"},
	{"POST", "/api/exploration/findings/{id}/traffic", "绑定快照式流量证据"},
	{"PATCH", "/api/exploration/findings/{id}/traffic/{binding_id}", "编辑流量证据绑定"},
	{"DELETE", "/api/exploration/findings/{id}/traffic/{binding_id}", "解除流量证据绑定"},

	// ── RuoJi6 侧：调度 / 案例 / 情报 / 代理池 / 重放 ──
	{"GET", "/api/schedules", "计划任务清单"},
	{"POST", "/api/schedules", "新建计划任务"},
	{"GET", "/api/exploration/finding-case-suggestions", "漏洞归并建议"},
	{"GET", "/api/exploration/findings/{id}/evidence", "证据截图（与上面同路径，确认只注册一次）"},
	{"GET", "/api/settings/proxy-pool", "代理池清单"},
	{"POST", "/api/repeater/send", "请求重放"},
	{"GET", "/api/system/metrics", "系统指标"},
	{"GET", "/api/update/check", "检查更新"},

	// ── 报告 logo 静态资源 ──
	// 这条此前既没进契约、handler 又直接用 http.ServeFile，缺文件时吐的是 Go
	// 内置的英文 "404 page not found"，两条防线一起漏。
	{"GET", "/api/report-assets/{file}", "报告模板 logo 读取"},
}

func TestMuxRouteContract(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	td := t.TempDir()
	s := New(ctx, m, td, td, td)

	h := s.Handler()
	for _, tc := range muxRouteContract {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			// 用明显不存在的 id：路由存在 → 应用层错误（JSON）；未注册 → Go 默认纯文本 404。
			path := strings.NewReplacer(
				"{id}", "999999999",
				"{eid}", "999999999",
				"{exchange_id}", "no-such-exchange",
				"{binding_id}", "999999999",
				"{file}", "no-such-file",
			).Replace(tc.path)
			req := httptest.NewRequest(tc.method, path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			body := rec.Body.String()
			if rec.Code == http.StatusNotFound && strings.Contains(body, "page not found") {
				t.Fatalf("路由未注册（%s %s）：%s —— %s", tc.method, tc.path, tc.why, body)
			}
		})
	}
}
