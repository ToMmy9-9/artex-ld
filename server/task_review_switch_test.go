package server

// ═══ [同类 bug 排查 · 反向僵尸开关] ═══
// 已经修过「关闭审核时联动关闭自动处置」，但只堵了一个方向。反过来的组合
// （审核关着、却把自动处置打开）同样造出僵尸开关：面板显示「自动审核中」，
// 而 sweep 只拾取 review_enabled=true 的任务，审核轮永远不会跑。
// 实测对 review_enabled=false 的任务 PATCH {"review_auto":true} 曾能成功落库。
// 二开日期：2026-11-21

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"artex/db"
)

// TestTaskCannotEnableAutoWithoutReview 审核关闭时开启自动处置必须被拒绝。
func TestTaskCannotEnableAutoWithoutReview(t *testing.T) {
	f := newSideHTTPFixture(t)

	task, err := f.m.CreateTask("auto needs review", "goal", nil, 0, 0)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	id := task.ID
	t.Cleanup(func() {
		var n int64
		if err := f.m.pg.QueryRow(`SELECT id FROM tasks WHERE id=$1`, id).Scan(&n); err == nil {
			_ = f.m.pg.DeleteTask(n)
		}
	})
	// 显式确认为「审核关闭」。
	if _, err := f.m.pg.Exec(`UPDATE tasks SET review_enabled=false, review_auto=false WHERE id=$1`, id); err != nil {
		t.Fatalf("reset flags: %v", err)
	}

	raw, _ := json.Marshal(map[string]any{"review_auto": true})
	req := httptest.NewRequest("PATCH", "/api/tasks/"+task.ID, strings.NewReader(string(raw)))
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("审核关闭时开启自动处置应返回 409，实际 %d body=%s", w.Code, w.Body.String())
	}

	// 落库状态必须没变 —— 拒绝要真的拒绝，不能只回错误码还写进去了。
	var enabled, auto bool
	if err := f.m.pg.QueryRow(`SELECT review_enabled, review_auto FROM tasks WHERE id=$1`, id).Scan(&enabled, &auto); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if auto {
		t.Fatal("接口返回 409 但 review_auto 已被写成 true，僵尸开关仍然产生了")
	}
	if enabled {
		t.Fatal("拒绝路径不该顺带打开 review_enabled")
	}
}

// TestTaskCanEnableAutoWhenReviewOn 审核开着时，开启自动处置必须成功。
func TestTaskCanEnableAutoWhenReviewOn(t *testing.T) {
	f := newSideHTTPFixture(t)

	task, err := f.m.CreateTask("auto with review", "goal", nil, 0, 0)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	id := task.ID
	t.Cleanup(func() {
		var n int64
		if err := f.m.pg.QueryRow(`SELECT id FROM tasks WHERE id=$1`, id).Scan(&n); err == nil {
			_ = f.m.pg.DeleteTask(n)
		}
	})
	if _, err := f.m.pg.Exec(`UPDATE tasks SET review_enabled=true, review_auto=false WHERE id=$1`, id); err != nil {
		t.Fatalf("reset flags: %v", err)
	}

	raw, _ := json.Marshal(map[string]any{"review_auto": true})
	req := httptest.NewRequest("PATCH", "/api/tasks/"+task.ID, strings.NewReader(string(raw)))
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("审核开启时开启自动处置应成功，实际 %d body=%s", w.Code, w.Body.String())
	}
	var auto bool
	if err := f.m.pg.QueryRow(`SELECT review_auto FROM tasks WHERE id=$1`, id).Scan(&auto); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !auto {
		t.Fatal("接口返回 200 但 review_auto 未落库")
	}
}

// TestTaskCloseReviewClearsAutoStillWorks 回归：原有方向（关审核→关自动）不能被撞坏。
// 与 review_http_test.go 的 TestTaskCloseReviewAlsoClearsAuto 互补——那条走内存态，
// 这条确认在「反向拦截」加进来之后两个方向仍然自洽。
func TestTaskCloseReviewClearsAutoAndStaysOff(t *testing.T) {
	f := newSideHTTPFixture(t)

	task, err := f.m.CreateTask("close review", "goal", nil, 0, 0)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	id := task.ID
	t.Cleanup(func() {
		var n int64
		if err := f.m.pg.QueryRow(`SELECT id FROM tasks WHERE id=$1`, id).Scan(&n); err == nil {
			_ = f.m.pg.DeleteTask(n)
		}
	})
	if _, err := f.m.pg.Exec(`UPDATE tasks SET review_enabled=true, review_auto=true WHERE id=$1`, id); err != nil {
		t.Fatalf("reset flags: %v", err)
	}

	raw, _ := json.Marshal(map[string]any{"review_enabled": false})
	req := httptest.NewRequest("PATCH", "/api/tasks/"+task.ID, strings.NewReader(string(raw)))
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusConflict {
		t.Fatalf("关闭审核应成功或因待审队列返回 409，实际 %d body=%s", w.Code, w.Body.String())
	}
	if w.Code != http.StatusOK {
		return // 有待审意图时本来就会 409，此路径不测联动
	}
	var enabled, auto bool
	if err := f.m.pg.QueryRow(`SELECT review_enabled, review_auto FROM tasks WHERE id=$1`, id).Scan(&enabled, &auto); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if enabled || auto {
		t.Fatalf("关闭审核后应同时清掉两个开关，实际 review_enabled=%v review_auto=%v", enabled, auto)
	}
}

var _ = db.Task{}
