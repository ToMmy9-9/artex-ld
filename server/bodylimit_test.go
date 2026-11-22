// bodylimit_test.go：验证「请求体超限」这条路径上用户看到的东西。
//
// 为什么单独测这个：中间件只负责掐断，状态码由各 handler 决定。如果 handler
// 忘了判断 *http.MaxBytesError，超限就会退化成别的错误 —— 实测 POST
// /api/workspace/write 传 20MB 时回的是 400 "非法路径"，因为 decode 失败后
// req.Path 是空串，代码继续往下走进了路径校验分支。用户传了个大文件，却被
// 告知路径不对，根本指不到真正的原因。
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// limitThenServe 复现中间件的效果：给 r.Body 套上 16MiB 上限再交给 handler。
// 不直接调 limitRequestBody 是为了让用例只依赖 handler 自身的行为，
// 中间件换实现也不影响这里的断言。
func limitThenServe(h http.HandlerFunc, r *http.Request, limit int64) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	h(w, r)
	return w
}

// oversizedWriteBody 造一个超过 limit 的合法 JSON：path 是正常的，
// 只有 content 超长。这样一旦 handler 误报成"路径"问题，测试立刻能看出来。
func oversizedWriteBody(limit int64) string {
	b, _ := json.Marshal(map[string]string{
		"path":    "oversize-probe.txt",
		"content": strings.Repeat("A", int(limit)+1024),
	})
	return string(b)
}

func TestWsWriteReturns413OnOversizedBody(t *testing.T) {
	s := &Server{}
	body := oversizedWriteBody(maxJSONBody)
	r := httptest.NewRequest("POST", "/api/workspace/write", strings.NewReader(body))
	w := limitThenServe(s.wsWrite, r, maxJSONBody)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want 413; body=%s", w.Code, w.Body.String())
	}
	msg := w.Body.String()
	// 必须是"大小"相关的提示。之前这里是"非法路径"，把用户引向完全错误的方向。
	if !strings.Contains(msg, "16MB") {
		t.Fatalf("413 提示应说明大小上限，实际：%s", msg)
	}
	if strings.Contains(msg, "非法路径") {
		t.Fatalf("超限不应报成路径问题：%s", msg)
	}
}

func TestSmallBodyIsNotTreatedAsOversize(t *testing.T) {
	// 反向用例：修复不能把正常的小请求也拦掉。
	// 这里直接验判定层，不进 wsWrite —— 过了体积检查它会走到 wsResolve，
	// 那需要真 manager，跟"是否误判超限"无关。
	body := `{"path":"notes.txt","content":"hello"}`
	r := httptest.NewRequest("POST", "/api/workspace/write", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)

	var dst struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decode(r, &dst); err != nil {
		t.Fatalf("小 body 应能正常解析，err=%v", err)
	}
	if dst.Path != "notes.txt" || dst.Content != "hello" {
		t.Fatalf("解析结果不对：%+v", dst)
	}
}

func TestDecodeOversizedBodyReportsLimitError(t *testing.T) {
	// 小请求不被误判的根因检查：同一份超限 body 交给 decode，
	// 必须能识别成 MaxBytesError，且对普通语法错误不误判。
	// 这条不依赖 Server，能独立跑，是上面两个 handler 修复的基础。
	body := oversizedWriteBody(maxJSONBody)

	r := httptest.NewRequest("POST", "/api/workspace/write", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	var dst struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	err := decode(r, &dst)
	if err == nil {
		t.Fatal("超限 body 应返回读取错误")
	}
	if !maxBytesExceeded(err) {
		t.Fatalf("应识别为超限，实际错误：%v", err)
	}

	// 语法错误必须走另一条路，不能被误判成"文件太大"
	r2 := httptest.NewRequest("POST", "/api/workspace/write", strings.NewReader(`{not json`))
	w2 := httptest.NewRecorder()
	r2.Body = http.MaxBytesReader(w2, r2.Body, maxJSONBody)
	if err := decode(r2, &dst); err == nil || maxBytesExceeded(err) {
		t.Fatalf("语法错误不应判定为超限，err=%v", err)
	}
}

func TestFsWriteFileReturns413OnOversizedBody(t *testing.T) {
	s := &Server{}
	body, _ := json.Marshal(map[string]string{"content": strings.Repeat("A", int(maxJSONBody)+1024)})
	r := httptest.NewRequest("PUT", "/api/skills/demo/files/SKILL.md", strings.NewReader(string(body)))
	r.SetPathValue("name", "demo")
	r.SetPathValue("file", "SKILL.md")
	w := limitThenServe(s.fsWriteFile, r, maxJSONBody)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want 413; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "16MB") {
		t.Fatalf("413 提示应说明大小上限，实际：%s", w.Body.String())
	}
}

func TestMaxBytesExceededOnlyMatchesLimitErrors(t *testing.T) {
	// 这个判断是上面所有 413 的前提。它一旦对普通解析错误也返回 true，
	// 语法错误的请求就会被告知"文件太大"，同样是误导。
	if maxBytesExceeded(nil) {
		t.Fatal("nil 不应判定为超限")
	}
	if maxBytesExceeded(&json.SyntaxError{}) {
		t.Fatal("JSON 语法错误不应判定为超限")
	}
	if !maxBytesExceeded(&http.MaxBytesError{Limit: maxJSONBody}) {
		t.Fatal("MaxBytesError 必须判定为超限")
	}
}
