package server

// docx 依赖缺失时的入口行为契约。
//
// 修复前的行为：POST /api/report/generate 无条件返回 202「生成中」，几秒后
// job.status 变成 failed，error 是整段 Python traceback：
//
//	python 渲染失败: exit status 1: Traceback (most recent call last):
//	  File ".../report_builder.py", line 19, in <module>
//	    from docx import Document
//	ModuleNotFoundError: No module named 'docx'
//
// 对使用者而言这是「点了没反应然后莫名其妙失败」，既不说明原因也给不出下一步。
// reportx.PythonAvailable() 早就实现了这套探测，却从未被调用（死代码）。
//
// 现在的要求：docx 依赖不可用时，入口直接拒绝并给出可执行提示；而 md 格式
// 不依赖 python，必须照常放行——不能因为缺依赖把能用的功能一起关掉。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGenerateReportRejectsDocxWhenPythonMissing 缺 python-docx 时 docx 请求应被
// 入口拒绝（5xx 且提示可操作），而不是先 202 再后台失败。
func TestGenerateReportRejectsDocxWhenPythonMissing(t *testing.T) {
	s, pg := newTestServer(t)
	// 直接钉死探测结果：本地是否真装了 python-docx 由 CI 环境决定，测试不该依赖它。
	s.docxProbe.Do(func() {}) // 先消费掉 once，避免它真的去 fork 探测
	s.docxOK = false

	m := &Manager{pg: pg, tasks: map[string]*Task{"77": {ID: "77", Name: "docx-依赖测试", Store: pg.Exploration(-1)}}}
	s.m = m
	s.reportJobs = map[string]*reportJob{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/report/generate?task=77", nil)
	s.generateReport(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("缺依赖时 docx 生成应返回 501，实际 %d（body=%s）", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "python-docx") {
		t.Fatalf("错误提示应指明缺失的依赖 python-docx，实际：%s", body)
	}
	// 不能把 Python traceback 甩给用户
	if strings.Contains(body, "Traceback") || strings.Contains(body, "ModuleNotFoundError") {
		t.Fatalf("不应把 python traceback 暴露给用户：%s", body)
	}
	// 也不能留下没跑起来的 job，否则 /status 会永远停在 generating
	s.reportMu.Lock()
	_, left := s.reportJobs["77"]
	s.reportMu.Unlock()
	if left {
		t.Fatal("被拒绝的生成请求不应在 reportJobs 里留下 job")
	}
}

// TestGenerateReportAllowsMarkdownWhenPythonMissing md 不依赖 python，
// 缺 docx 依赖时必须照常放行（202）。
func TestGenerateReportAllowsMarkdownWhenPythonMissing(t *testing.T) {
	s, pg := newTestServer(t)
	s.docxProbe.Do(func() {})
	s.docxOK = false

	m := &Manager{pg: pg, tasks: map[string]*Task{"78": {ID: "78", Name: "md-放行测试", Store: pg.Exploration(-1)}}}
	s.m = m
	s.reportJobs = map[string]*reportJob{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/report/generate?task=78&format=md", nil)
	s.generateReport(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("md 格式不依赖 python，应返回 202，实际 %d（body=%s）", rec.Code, rec.Body.String())
	}
}

// TestGenerateReportAllowsDocxWhenPythonPresent 依赖齐备时必须正常放行，
// 别让新加的守卫误伤正常环境。
func TestGenerateReportAllowsDocxWhenPythonPresent(t *testing.T) {
	s, pg := newTestServer(t)
	s.docxProbe.Do(func() {})
	s.docxOK = true

	m := &Manager{pg: pg, tasks: map[string]*Task{"79": {ID: "79", Name: "docx-放行测试", Store: pg.Exploration(-1)}}}
	s.m = m
	s.reportJobs = map[string]*reportJob{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/report/generate?task=79", nil)
	s.generateReport(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("依赖齐备时应返回 202，实际 %d（body=%s）", rec.Code, rec.Body.String())
	}
}
