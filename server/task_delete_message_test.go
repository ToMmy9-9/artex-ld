// task_delete_message_test.go：任务删除收尾文案与归档列表的筛选参数。
//
// 两处都是"中文界面上冒出一句英文/一段本该报错却静默返回"的同类问题：
//   - 删除已提交但外部清理失败时，cleanup_warning 写的是 err.Error()，
//     toast 里出现 "task deletion committed; external cleanup incomplete: ..."；
//   - 旁路问答排空超时，409 正文是 "context deadline exceeded"；
//   - GET /api/task-archives?state=乱填 静默返回 0 条，看着像归档数据丢了。
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestTaskDeleteCleanupMessageNamesWhatFailed(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "任务文件",
			err:  fmt.Errorf("%w：%w", errCleanupTaskFiles, errors.New("permission denied")),
			want: "任务文件",
		},
		{
			name: "流量热存储",
			err:  fmt.Errorf("%w：%w", errCleanupTrafficHotStore, errors.New("remove staged traffic: busy")),
			want: "流量热存储",
		},
		{
			name: "两处都失败",
			err: errors.Join(
				fmt.Errorf("%w：%w", errCleanupTaskFiles, errors.New("permission denied")),
				fmt.Errorf("%w：%w", errCleanupTrafficHotStore, errors.New("busy")),
			),
			want: "流量热存储与任务文件",
		},
		{
			name: "认不出的一律兜底",
			err:  errors.New("boom"),
			want: "外部数据",
		},
	}
	for _, c := range cases {
		// 真实链路里这个错误被 taskDeleteCommittedError 包了一层，必须能穿透。
		msg := taskDeleteCleanupMessage(&taskDeleteCommittedError{err: c.err})
		if !strings.Contains(msg, c.want) {
			t.Errorf("%s -> %q，应说明 %q", c.name, msg, c.want)
		}
		// 用户需要知道"要我自己动手"：数据库里已经删了，不会自己好。
		if !strings.Contains(msg, "任务本身已删除") || !strings.Contains(msg, "手工清理") {
			t.Errorf("%s 的文案没交代后果：%q", c.name, msg)
		}
		for _, leak := range []string{"permission denied", "busy", "cleanup", "finalize"} {
			if strings.Contains(msg, leak) {
				t.Errorf("%s 的文案泄漏了英文原文 %q：%q", c.name, leak, msg)
			}
		}
	}
}

func TestTaskDeleteCleanupMessageHidesLocalPath(t *testing.T) {
	// 修复前这串本地绝对路径会被前端 toast 原样显示出来。
	err := &taskDeleteCommittedError{err: fmt.Errorf("%w：%w", errCleanupTaskFiles,
		&os.PathError{Op: "remove", Path: `D:\data\tasks\12\evidence\shot.png`, Err: syscall.EACCES})}
	msg := taskDeleteCleanupMessage(err)
	for _, leak := range []string{`D:\`, "evidence", "remove", "eacces"} {
		if strings.Contains(strings.ToLower(msg), strings.ToLower(leak)) {
			t.Fatalf("文案泄漏内部细节 %q：%q", leak, msg)
		}
	}
	if !hasCJK(msg) {
		t.Fatalf("文案必须是中文：%q", msg)
	}
	// 日志侧仍要看得到原文，否则排障时无从下手。
	if !strings.Contains(err.Error(), `D:\data\tasks\12`) {
		t.Fatalf("Error() 应保留原始细节供日志使用：%q", err.Error())
	}
}

func TestSideDrainMessageTranslatesTimeout(t *testing.T) {
	got := sideDrainMessage(context.DeadlineExceeded)
	if !hasCJK(got) || !strings.Contains(got, "超时") {
		t.Fatalf("超时应翻成中文，实际：%q", got)
	}
	if strings.Contains(got, "context") || strings.Contains(got, "deadline") {
		t.Fatalf("不该出现 Go 运行时措辞：%q", got)
	}
	// 包了一层也要认得出。
	if wrapped := sideDrainMessage(fmt.Errorf("drain side questions: %w", context.DeadlineExceeded)); !hasCJK(wrapped) {
		t.Fatalf("被包装的超时没被识别：%q", wrapped)
	}
	// 流程自己写的中文原因原样透传，别被超时文案覆盖。
	zh := errors.New("旁路上下文尚未保存，请重试")
	if got := sideDrainMessage(zh); got != zh.Error() {
		t.Fatalf("中文原因被改写：%q", got)
	}
}

// archiveFilterFixture 建一条 paused 任务并给它排一个归档，返回任务 id。
func archiveFilterFixture(t *testing.T) (*Manager, int64) {
	t.Helper()
	m, tid := pausedTaskFixture(t)
	if err := m.SetTaskPaused(strconv.FormatInt(tid, 10), true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.pg.QueueTaskArchive(tid); err != nil {
		t.Fatalf("前置条件失败，排不上归档：%v", err)
	}
	return m, tid
}

func getArchivesPage(t *testing.T, s *Server, query string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/task-archives?"+query, nil)
	s.listTaskArchives(rec, req)
	var body map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
	}
	return rec.Code, body
}

func TestListTaskArchivesRejectsUnknownState(t *testing.T) {
	// 修复前：state=乱填 静默返回 total=0，用户会以为归档全没了。
	m, _ := archiveFilterFixture(t)
	s := &Server{m: m}

	code, body := getArchivesPage(t, s, "state="+url.QueryEscape("乱填"))
	if code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400（body=%v）", code, body)
	}
	msg, _ := body["error"].(string)
	if !hasCJK(msg) || !strings.Contains(msg, "归档状态") {
		t.Fatalf("应给出中文原因，实际：%q", msg)
	}
	for _, leak := range []string{"archive_queued", "sql", "WHERE"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("错误文案泄漏内部细节 %q：%q", leak, msg)
		}
	}
}

func TestListTaskArchivesTreatsAllTokenAsNoFilter(t *testing.T) {
	// 页面下拉里的「全部状态」发的是 all；直接丢给 db 会被当成一个不存在的状态，
	// 于是筛出 0 条。这里连真数据一起验，防止"归一化"写成空操作还看不出来。
	m, tid := archiveFilterFixture(t)
	s := &Server{m: m}
	q := "q=" + strconv.FormatInt(tid, 10)

	code, all := getArchivesPage(t, s, q+"&state=all")
	if code != http.StatusOK {
		t.Fatalf("state=all status=%d want 200（body=%v）", code, all)
	}
	code, noFilter := getArchivesPage(t, s, q)
	if code != http.StatusOK {
		t.Fatalf("不带 state status=%d want 200", code)
	}
	if all["total"] != noFilter["total"] {
		t.Fatalf("state=all 应与不过滤等价：all=%v 无参数=%v", all["total"], noFilter["total"])
	}
	if all["total"] == float64(0) {
		t.Fatalf("夹具里的归档没被查出来，用例本身失效：%v", all)
	}
}

func TestListTaskArchivesAcceptsEveryKnownState(t *testing.T) {
	// 白名单漏一个，页面上对应的筛选项就会变成 400 或 0 条。
	m, tid := archiveFilterFixture(t)
	s := &Server{m: m}
	q := "q=" + strconv.FormatInt(tid, 10)

	for _, state := range []string{
		"archive_queued", "archiving", "archive_failed", "ready",
		"restore_queued", "restoring", "restore_failed",
		"delete_queued", "deleting", "delete_failed",
	} {
		code, body := getArchivesPage(t, s, q+"&state="+state)
		if code != http.StatusOK {
			t.Errorf("state=%s 是合法取值，却回了 %d（%v）", state, code, body)
		}
	}
	// 当前 fixture 的状态是 archive_queued：它能被筛出来，其余合法状态应是 0 条。
	if code, body := getArchivesPage(t, s, q+"&state=archive_queued"); code != 200 || body["total"] == float64(0) {
		t.Errorf("archive_queued 应筛出夹具归档，实际 status=%d body=%v", code, body)
	}
	if code, body := getArchivesPage(t, s, q+"&state=ready"); code != 200 || body["total"] != float64(0) {
		t.Errorf("ready 不该筛出 archive_queued 的归档，实际 status=%d body=%v", code, body)
	}
}
