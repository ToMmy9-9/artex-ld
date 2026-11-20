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
	// 并发上限是全局设置：开启且 LLM 不可用时 reconcileConcurrency 会把任务持久化
	// 入队，排队态会让归档给出另一种 409 文案。本用例与并发无关，因此**在建任务之前**
	// 就关掉它（恢复放在 defer 里：本文件的 defer m.Close() 先于 t.Cleanup 执行，
	// 恢复必须在 DB 还活着时做）。
	defer disableConcurrencyForTest(t, m)()
	recorder := request(http.MethodPost, "/api/tasks/"+task.ID+"/archive", "{}")
	if recorder.Code != http.StatusConflict {
		t.Fatalf("running archive status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	// 置为暂停：暂停态才允许归档。
	// 必须走 HTTP 控制端点（applyTaskControl）而不是直写 DB —— 直写 DB 只会改
	// 数据行，内存态生命周期镜像仍是「未暂停」，后台调度器据此把任务重新入队，
	// 于是归档又落回「queued task must be paused」。
	recorder = request(http.MethodPost, "/api/tasks/"+task.ID+"/control", `{"action":"pause"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("pause task status=%d body=%s", recorder.Code, recorder.Body.String())
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
