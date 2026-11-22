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
	"strconv"
	"strings"
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
	// 未识别的错误保持原文 —— 那才是需要照着原文排查的真实异常，
	// 翻成泛化中文反而会丢掉线索。
	err := errors.New("connection reset by peer")
	if got := archiveErrorMessage(err); got != err.Error() {
		t.Fatalf("未知错误应保持原文，实际：%q", got)
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
