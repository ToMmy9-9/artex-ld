// task_archive_error_test.go：归档失败时用户看到的必须是能照着做的中文。
//
// 背景（真实体验缺陷）：writeArchiveError 与批量归档都直接把 err.Error() 透传，
// 前端 toast 拼成 "归档失败：" + 原文，于是中文界面上出现
//   "归档失败：queued task must be paused before archiving"
// 用户看不懂，也不知道下一步该点哪里。这些错误都是可预期的前置条件，
// 必须翻成"为什么不行 + 怎么做"。
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"

	pgdb "artex/db"
)

func TestArchiveErrorMessageCoversEverySentinel(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{pgdb.ErrTaskArchiveNotFound, "归档不存在或已被删除"},
		{pgdb.ErrTaskArchiveQueued, "任务还在并发队列里排队，请先暂停它再归档"},
		{pgdb.ErrTaskArchiveIneligible, "任务需要先暂停、或已经结束，才能归档"},
		{pgdb.ErrTaskArchiveDeleteBlocked, "这个归档仍被其它归档引用，无法删除"},
		{pgdb.ErrTaskArchiveFormatMismatch, "不支持的归档格式"},
		{pgdb.ErrTaskArchiveState, "归档当前状态不允许该操作，请刷新后重试"},
	}
	for _, c := range cases {
		got := archiveErrorMessage(c.err)
		if got != c.want {
			t.Errorf("%v -> %q，期望 %q", c.err, got, c.want)
		}
		// 中文文案里不该再残留英文原文，否则用户还是看不懂。
		if strings.Contains(got, "task") || strings.Contains(got, "archive") {
			t.Errorf("%v 的文案仍含英文内部措辞：%q", c.err, got)
		}
	}
}

func TestArchiveErrorMessageMentionsDownstreamForDependent(t *testing.T) {
	// 这个错误在生产里是用 fmt.Errorf("%w: task %d") 包过的，
	// 必须仍能识别成 Dependent，并把用户导向"先处理下游"。
	err := fmt.Errorf("%w: task 42", pgdb.ErrTaskArchiveDependent)
	got := archiveErrorMessage(err)
	if !strings.Contains(got, "继承") || !strings.Contains(got, "下游") {
		t.Fatalf("依赖错误应提示下游任务，实际：%q", got)
	}
}

func TestArchiveErrorMessageFallsBackToOriginalForUnknown(t *testing.T) {
	// 纯映射函数对未知错误保持原文 —— 它只用于日志/内部拼装；
	// 有 HTTP 出口的地方走 archiveRequestMessage / writeArchiveError，不会把原文给用户。
	err := errors.New("connection reset by peer")
	if got := archiveErrorMessage(err); got != err.Error() {
		t.Fatalf("未知错误应保持原文，实际：%q", got)
	}
}

func TestArchiveErrorSentinelsAreAllRecognized(t *testing.T) {
	// 哨兵清单与翻译 switch 必须同步：漏一条就意味着那条错误又会以英文原文出现。
	// 这里逐个校验，新增哨兵时先红。
	for _, sentinel := range archiveErrorSentinels {
		if !isKnownArchiveError(sentinel) {
			t.Errorf("%v 未被 isKnownArchiveError 识别", sentinel)
		}
		if got := archiveErrorMessage(sentinel); got == sentinel.Error() {
			t.Errorf("%v 没有中文文案", sentinel)
		}
	}
	// 反向：无关错误不能被误认成归档前置条件，否则会被硬塞一句不相干的中文。
	if isKnownArchiveError(errors.New("connection reset by peer")) {
		t.Fatal("无关错误被误判为已知归档错误")
	}
}

func TestArchiveFailureMessageTranslatesPackageDamage(t *testing.T) {
	// 归档包坏了是用户唯一能自己动手处理的一类：必须说清后果（不能继续还原）。
	cases := []struct {
		raw  error
		want string
	}{
		{errors.New("task archive checksum mismatch"), "校验"},
		{errors.New("task archive exceeds extraction safety limits"), "体积"},
		{fmt.Errorf("unsafe task archive path %q", "../../etc/passwd"), "不安全"},
		{fmt.Errorf("unsupported task archive entry type %d", 7), "不支持"},
		{pgdb.ErrTaskArchiveFormatMismatch, "不支持的归档格式"},
	}
	for _, c := range cases {
		got := archiveFailureMessage(c.raw)
		if !strings.Contains(got, c.want) {
			t.Errorf("%v -> %q，应含 %q", c.raw, got, c.want)
		}
		if strings.Contains(got, "checksum") || strings.Contains(got, "archive") {
			t.Errorf("%v 的失败原因仍含英文原文：%q", c.raw, got)
		}
	}
}

func TestArchiveFailureMessageMapsDiskAndPermission(t *testing.T) {
	disk := &os.PathError{Op: "write", Path: `D:\data\tasks\12\a.zip`, Err: syscall.ENOSPC}
	if got := archiveFailureMessage(disk); !strings.Contains(got, "磁盘空间不足") {
		t.Errorf("磁盘满应提示清理磁盘，实际：%q", got)
	}
	perm := &os.PathError{Op: "mkdir", Path: `D:\data\tasks\12`, Err: syscall.EACCES}
	if got := archiveFailureMessage(perm); !strings.Contains(got, "权限") {
		t.Errorf("权限不足应提示检查目录权限，实际：%q", got)
	}
}

func TestArchiveFailureMessageKeepsChineseFlowReasons(t *testing.T) {
	// 流程自己写的中文原因（如"任务仍有运行中的 Agent"）本身就是给用户看的，
	// 不能被兜底文案覆盖掉。
	raw := errors.New("任务仍有运行中的 Agent，请先暂停后重试归档")
	if got := archiveFailureMessage(raw); got != raw.Error() {
		t.Fatalf("中文原因被改写：%q", got)
	}
}

func TestArchiveFailureMessageHidesInternalDetail(t *testing.T) {
	// 这两类原文修复前会直接渲染在归档列表里：本地绝对路径、数据库 schema。
	for _, raw := range []error{
		errors.New(`mkdir D:\data\tasks\12\evidence: Access is denied`),
		errors.New(`pq: relation "task_archives" does not exist`),
		errors.New("write tcp 127.0.0.1:5432: connection reset by peer"),
	} {
		got := archiveFailureMessage(raw)
		if got != "归档过程中出现内部错误，操作未完成，详情见服务日志" {
			t.Errorf("%v -> %q，应给统一兜底文案", raw, got)
		}
		for _, leak := range []string{"D:\\", "pq:", "tcp", "task_archives", "Access is denied"} {
			if strings.Contains(got, leak) {
				t.Errorf("%v 的文案泄漏内部细节 %q：%q", raw, leak, got)
			}
		}
	}
}

func TestWriteArchiveErrorRedactsUnknownCause(t *testing.T) {
	// 未识别错误 = 内部故障，按全仓 5xx 约定只回固定文案，详情进日志。
	rec := httptest.NewRecorder()
	writeArchiveError(rec, errors.New(`pq: relation "task_archives" does not exist`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", rec.Code)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%s", rec.Body.String())
	}
	if body.Error != errDataSourceUnavailable {
		t.Fatalf("应回固定文案，实际：%q", body.Error)
	}
	if strings.Contains(body.Error, "pq") || strings.Contains(body.Error, "task_archives") {
		t.Fatalf("5xx 响应泄漏了数据库细节：%q", body.Error)
	}
}

func TestWriteArchiveErrorKeepsActionableStatus(t *testing.T) {
	// 可预期前置条件仍是 409/404 + 中文，不能被 5xx 兜底吞掉。
	cases := []struct {
		err  error
		code int
	}{
		{pgdb.ErrTaskArchiveQueued, http.StatusConflict},
		{pgdb.ErrTaskArchiveDependent, http.StatusConflict},
		{pgdb.ErrTaskArchiveFormatMismatch, http.StatusConflict},
		{pgdb.ErrTaskArchiveNotFound, http.StatusNotFound},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeArchiveError(rec, c.err)
		if rec.Code != c.code {
			t.Errorf("%v status=%d want %d", c.err, rec.Code, c.code)
		}
		var body struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("响应不是 JSON：%s", rec.Body.String())
		}
		if !hasCJK(body.Error) {
			t.Errorf("%v 的响应不是中文：%q", c.err, body.Error)
		}
	}
}

func TestBatchRestoreReportsChineseReason(t *testing.T) {
	// 端到端：批量还原一个不存在的归档，逐条原因必须是中文。
	// 修复前这里写的是 err.Error()，toast 里出现 "task archive state does not allow this operation"。
	m, _ := pausedTaskFixture(t)
	s := &Server{m: m}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/task-archives/restore/batch",
		strings.NewReader(`{"archive_ids":[999999999]}`))
	req.Header.Set("Content-Type", "application/json")

	s.queueTaskArchiveActionBatch(rec, req, "restore")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d want 202（body=%s）", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []archiveBatchItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%s", rec.Body.String())
	}
	if len(body.Items) != 1 {
		t.Fatalf("items=%d want 1", len(body.Items))
	}
	item := body.Items[0]
	if item.OK {
		t.Fatal("不存在的归档不该被认为排队成功")
	}
	if !hasCJK(item.Error) {
		t.Fatalf("逐条原因必须是中文，实际：%q", item.Error)
	}
	for _, leak := range []string{"task archive", "does not allow", "sql"} {
		if strings.Contains(item.Error, leak) {
			t.Fatalf("逐条原因泄漏英文原文 %q：%q", leak, item.Error)
		}
	}
}

func TestQueueTaskArchiveReturnsChineseMessage(t *testing.T) {
	// 端到端：用户对一个还排在并发队列里的任务点归档，
	// 接口必须回 409 + 能看懂的中文，而不是英文内部错误。
	m, tid := pausedTaskFixture(t)
	if err := m.EnqueueTask(strconv.FormatInt(tid, 10), "bootstrap"); err != nil {
		t.Fatal(err)
	}
	s := &Server{m: m}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/tasks/x/archive", nil)
	req.SetPathValue("id", strconv.FormatInt(tid, 10))

	s.queueTaskArchive(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409（body=%s）", rec.Code, rec.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%s", rec.Body.String())
	}
	if !strings.Contains(body.Error, "排队") || !strings.Contains(body.Error, "暂停") {
		t.Fatalf("应提示「先暂停再归档」，实际：%q", body.Error)
	}
	if strings.Contains(body.Error, "queued") {
		t.Fatalf("不应把英文原文透给用户：%q", body.Error)
	}
}
