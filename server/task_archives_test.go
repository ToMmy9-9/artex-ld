package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestTaskArchiveAPIQueueListAndLimits(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) - skipping", err)
	}
	defer m.Close()
	task, err := m.CreateTask("archive api", "verify 202", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := strconv.ParseInt(task.ID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = m.pg.Exec(`DELETE FROM task_archives WHERE task_id=$1`, taskID)
		_ = m.pg.DeleteTask(taskID)
	}()

	// Start with an already-cancelled context so the background archive worker
	// exits before these route-contract assertions enqueue work.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dataDir := t.TempDir()
	s := New(ctx, m, dataDir, dataDir, dataDir)
	s.archiveWG.Wait()
	token, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		s.Handler().ServeHTTP(recorder, req)
		return recorder
	}

	// 先把任务置为暂停再断言「运行中归档被拒」：归档要求任务已暂停，
	// 而任务是否处于 queued 取决于全局并发上限设置（其它测试可能改过），
	// 直接断言 running→409 会被排队态干扰。先确保它不在排队里。
	// 归档要求「已暂停或终态」，运行中的任务必须被拒（409）。
	// 任务是否处于 queued 取决于全局并发上限设置：开启且 LLM 不可用时，
	// reconcileConcurrency 会把任务持久化入队（给出另一种 409 文案）。
	// 本用例只关心「未暂停 → 拒归档」，所以先关掉并发上限并清掉排队态。
	// 并发上限是全局设置：本用例临时关掉它。本文件的 defer m.Close() 先于
	// t.Cleanup 执行，所以恢复必须在 defer 链里做（DB 还活着的时候）。
	defer disableConcurrencyForTest(t, m)()
	if err := m.pg.SetQueued(taskID, false); err != nil {
		t.Fatal(err)
	}
	if err := m.pg.SetPaused(taskID, false); err != nil {
		t.Fatal(err)
	}
	recorder := request(http.MethodPost, "/api/tasks/"+task.ID+"/archive", "{}")
	if recorder.Code != http.StatusConflict {
		t.Fatalf("running archive status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	// 之后置为暂停：暂停态才允许归档。
	if err := m.pg.SetPaused(taskID, true); err != nil {
		t.Fatal(err)
	}
	recorder = request(http.MethodPost, "/api/tasks/"+task.ID+"/archive", "{}")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("archive queue status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var queued struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &queued); err != nil {
		t.Fatal(err)
	}
	if queued.ID <= 0 || queued.State != "archive_queued" {
		t.Fatalf("unexpected queue response: %+v", queued)
	}

	recorder = request(http.MethodGet, "/api/task-archives?page=1&size=10&q="+task.ID, "")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), fmt.Sprintf(`"task_id":%d`, taskID)) {
		t.Fatalf("archive list status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	ids := make([]string, 101)
	for index := range ids {
		ids[index] = strconv.Itoa(index + 1)
	}
	body, _ := json.Marshal(map[string]any{"task_ids": ids})
	recorder = request(http.MethodPost, "/api/tasks/archive/batch", string(body))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "100") {
		t.Fatalf("oversized batch status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
