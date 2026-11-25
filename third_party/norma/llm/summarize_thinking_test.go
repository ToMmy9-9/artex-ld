package llm

import (
	"context"
	"encoding/json"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSummarizeThinkingFollowsProviderConfig 钉住「总结类调用的 thinking 取值」。
//
// 背景：压缩摘要、自动标题、凭据提取、AI 报告、侧边问答摘要都曾硬编码
// `Thinking: "disabled"`（省 token）。但 MiniMax-M3.1-Flash 这类模型
// **强制要求 adaptive thinking**，收到 disabled 直接回 HTTP 400 ——
// 实测压缩功能因此直接不可用（可核记录：11-24 09:00–18:32 共 34 条 disabled
// 报错；更早数据已被 llm_records 保留策略清理），且现象看起来像「偶尔失败」。
//
// server/intercept.go 早就踩过同一个坑并修好（改成传 profile 的设置），
// 但这几处一直没跟上。这条用例防止再有人硬编码。
func TestSummarizeThinkingFollowsProviderConfig(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		want       string
	}{
		{"未配置 → 不发送该字段（交给服务端决定）", "", ""},
		{"显式 disabled → disabled（用户的选择）", "disabled", "disabled"},
		{"模型要求 enabled → 不能覆盖成 disabled", "enabled", "enabled"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &openaiProvider{cfg: Config{ThinkingType: c.configured}}
			if got := SummarizeThinking(p); got != c.want {
				t.Fatalf("SummarizeThinking(configured=%q) = %q, want %q", c.configured, got, c.want)
			}
		})
	}
}

// TestSummarizeThinkingUnknownProvider 不暴露配置的 provider（如测试 stub）
// 退回「不发送」—— 不能因为拿不到配置就硬发 disabled（会撞上要求 thinking
// 的模型）。这比旧行为安全：服务端自己决定总不会 400。
func TestSummarizeThinkingUnknownProvider(t *testing.T) {
	if got := SummarizeThinking(stubProvider{}); got != "" {
		t.Fatalf("未知 provider 的 SummarizeThinking = %q, want \"\"（不发送）", got)
	}
}

// stubProvider 是不实现 ThinkingAware 的最小 Provider。
type stubProvider struct{}

func (stubProvider) Stream(ctx context.Context, req CompletionRequest) iter.Seq2[StreamEvent, error] {
	return func(yield func(StreamEvent, error) bool) {}
}

func (stubProvider) Complete(ctx context.Context, req CompletionRequest) (Message, string, Usage, error) {
	return Message{}, "", Usage{}, nil
}

// TestSummarizeThinkingOmitsFieldEndToEnd 端到端验证：总结类调用在 profile
// 未配置 thinking 时，请求体里**根本不带** thinking 字段。
//
// 这条是本次修复的核心保证：MiniMax-M3.1-Flash 这类模型要求 adaptive
// thinking，收到 thinking.type="disabled" 会回 HTTP 400。生产库里 profile 267
// （MiniMax-M3.1-Flash）的 thinking_type 正是空的 —— 所以「未配置就不发送」
// 是唯一正确的默认行为。
func TestSummarizeThinkingOmitsFieldEndToEnd(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.Write([]byte(sse(`data: {"choices":[{"index":0,"delta":{"content":"ok"}}]}`, ``, `data: [DONE]`, ``)))
	}))
	defer srv.Close()

	p, err := NewProvider(Config{
		Format:  FormatOpenAI,
		BaseURL: srv.URL,
		APIKey:  "k",
		Model:   "m",
		// profile 未配置 thinking（生产库里 MiniMax-M3.1-Flash 就是这样）
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	collect(t, p, CompletionRequest{
		Messages: []Message{UserText("go")},
		Thinking: SummarizeThinking(p),
	})

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	if th, ok := m["thinking"]; ok {
		t.Fatalf("未配置 thinking 时请求体不应带 thinking 字段，实际=%v（模型会回 400）", th)
	}
}
