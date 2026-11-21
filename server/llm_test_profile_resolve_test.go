// llm_test_profile_resolve_test.go：/api/llm/test 只传 profile_id 时必须用库里存的
// provider/model/base_url，而不是默认成 anthropic。
//
// 背景（实测缺陷）：前端"测试已保存配置"只发 {profile_id}，其余字段留空。早期实现
// 只从库里补了 api_key 与 session_header_key，provider 仍是空串，于是 ConfigFrom 走
// default 分支 → FormatAnthropic，拿 anthropic 协议去打一个 openai 兼容端点。
// 实测 profile 268（format=openai, opencode.ai/inference/go/openai/v1）因此返回
// "anthropic: status 403 Request not allowed"，看着像配置坏了；补齐字段再测
// 就是 ok=true / 2915ms。这里用一个假端点把"最终用了哪个 provider"钉死。
package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"artex/db"
)

// 假端点记录它收到的请求路径与鉴权头，足以区分 anthropic(/v1/messages) 与
// openai(/chat/completions) 两条协议路径。
type fakeLLMEndpoint struct {
	mu    sync.Mutex
	paths []string
	auths []string
}

func (f *fakeLLMEndpoint) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		f.mu.Unlock()

		// 按实际协议回一个最小合法响应，让 TestConnection 能拿到 "OK"。
		// TestConnection 默认走流式（与生产同口径），因此这里必须回 SSE 帧；
		// 回普通 JSON 的话请求虽然 200，但解析不出任何文本，会被判成
		// "模型无回复内容（请求已通，但未返回任何文本）"。
		if strings.Contains(r.URL.Path, "/chat/completions") {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			frames := []string{
				`{"id":"fake","object":"chat.completion.chunk","model":"fake-model","choices":[{"index":0,"delta":{"role":"assistant","content":"OK"},"finish_reason":null}]}`,
				`{"id":"fake","object":"chat.completion.chunk","model":"fake-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			}
			for _, f := range frames {
				_, _ = io.WriteString(w, "data: "+f+"\n\n")
			}
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		if strings.Contains(r.URL.Path, "/messages") {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			frames := []string{
				`{"type":"message_start","message":{"id":"msg_fake","type":"message","role":"assistant","model":"fake-model","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"OK"}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
				`{"type":"message_stop"}`,
			}
			for _, f := range frames {
				_, _ = io.WriteString(w, "event: "+"message"+"\n")
				_, _ = io.WriteString(w, "data: "+f+"\n\n")
			}
			return
		}
		_ = body
		http.Error(w, "unexpected path", http.StatusNotFound)
	}
}

func (f *fakeLLMEndpoint) snapshot() (paths, auths []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...), append([]string(nil), f.auths...)
}

// 只传 profile_id 时，provider/model/base_url 必须全部从库里取。
func TestLLMTestProfileIDResolvesProviderAndBaseURL(t *testing.T) {
	m, s := fiFixture(t)
	fake := &fakeLLMEndpoint{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	// 干净地插入一条 openai 格式配置，测试后删掉，避免污染运维的 LLM 池。
	prof := &db.LLMProfile{
		Name:    "t-llm-test-resolve",
		Format:  "openai",
		Model:   "fake-model",
		BaseURL: srv.URL,
		APIKey:  "sk-test-resolve",
	}
	id, err := m.pg.SaveProfile(prof)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.pg.Exec(`DELETE FROM llm_profiles WHERE id=$1`, id) })

	body := []byte(`{"profile_id":` + strconv.FormatInt(id, 10) + `}`)
	req := httptest.NewRequest(http.MethodPost, "/api/llm/test", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.testLLM(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("连接测试应成功，实际响应 %v", resp)
	}

	paths, auths := fake.snapshot()
	if len(paths) == 0 {
		t.Fatal("假端点没收到任何请求")
	}
	// 关键判据：必须走 openai 的 /chat/completions，而不是 anthropic 的 /v1/messages。
	for _, p := range paths {
		if strings.Contains(p, "/messages") {
			t.Fatalf("provider 未从库里解析：请求打到了 anthropic 路径 %q", p)
		}
	}
	if !strings.Contains(paths[0], "/chat/completions") {
		t.Fatalf("期望 openai 的 /chat/completions，实际 %q", paths[0])
	}
	// key 也必须来自库里。
	if len(auths) == 0 || !strings.Contains(auths[0], "sk-test-resolve") {
		t.Fatalf("API Key 未从库里解析，Authorization=%q", firstOrEmpty(auths))
	}

	// 表单显式给了 provider 时必须优先于库里的值（不能被兜底逻辑反盖）。
	fake2paths := func() []string {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		fake.paths = nil
		return nil
	}
	fake2paths()
	body2 := []byte(`{"profile_id":` + strconv.FormatInt(id, 10) + `,"provider":"anthropic","model":"fake-model","base_url":"` + srv.URL + `"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/llm/test", strings.NewReader(string(body2)))
	rec2 := httptest.NewRecorder()
	s.testLLM(rec2, req2)
	paths2, _ := fake.snapshot()
	if len(paths2) == 0 {
		t.Fatalf("显式 provider 的请求没打出去: %s", rec2.Body.String())
	}
	if !strings.Contains(paths2[0], "/messages") {
		t.Fatalf("显式 provider=anthropic 应走 /v1/messages，实际 %q", paths2[0])
	}
}

func firstOrEmpty(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
}
