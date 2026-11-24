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
	"time"

	"artex/db"
)

// waitReportJobDone 等后台报告生成 goroutine 结束。
//
// 为什么必须等：generateReport 是异步的（返回 202 后 go func 继续跑），而
// newTestServer 用 t.TempDir() 给 dataDir —— Go 会在测试结束时删它。若后台
// goroutine 还在往 data/reports 写文件，清理就会报
// "unlinkat ...: The directory is not empty"，**测试断言其实通过了**，
// 却被 Go 判成 FAIL（实测失败率约 60%）。
//
// 轮询 job.Status 而不是 sleep 固定时长：既确定又不会拖慢正常路径。
func waitReportJobDone(t *testing.T, s *Server, taskID string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		s.reportMu.Lock()
		j := s.reportJobs[taskID]
		status := ""
		if j != nil {
			status = j.Status
		}
		s.reportMu.Unlock()
		if status != "" && status != "generating" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("报告生成任务 %s 在 30s 内未结束", taskID)
}

// cleanReportRows 删掉本测试产生的报告行。
//
// 报告生成成功后会往 reports 表写一条记录，而测试库是所有测试共用的 ——
// 不清理的话每跑一次就多一条垃圾（实测已积了 10 条 task 78/79 的残留）。
func cleanReportRows(t *testing.T, pg *db.DB, taskIDs ...int64) {
	t.Helper()
	for _, id := range taskIDs {
		_, _ = pg.Exec(`DELETE FROM reports WHERE task_id = $1`, id)
	}
}

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
	// 等后台生成结束 —— 否则它还在往 data/reports 写文件时 TempDir 清理会失败。
	waitReportJobDone(t, s, "78")
	cleanReportRows(t, pg, 78)
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
	waitReportJobDone(t, s, "79")
	cleanReportRows(t, pg, 79)
}
