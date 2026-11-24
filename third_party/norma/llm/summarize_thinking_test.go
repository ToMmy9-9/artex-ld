package llm

import (
	"context"
	"iter"
	"testing"
)

// TestSummarizeThinkingFollowsProviderConfig 钉住「总结类调用的 thinking 取值」。
//
// 背景：压缩摘要、自动标题、凭据提取、AI 报告、侧边问答摘要都曾硬编码
// `Thinking: "disabled"`（省 token）。但 MiniMax-M3.1-Flash 这类模型
// **强制要求 adaptive thinking**，收到 disabled 直接回 HTTP 400 ——
// 实测压缩功能因此在三天内失败 360 次，且功能看起来「只是偶尔失败」。
//
// server/intercept.go 早就踩过同一个坑并修好（改成传 profile 的设置），
// 但这几处一直没跟上。这条用例防止再有人硬编码。
func TestSummarizeThinkingFollowsProviderConfig(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		want       string
	}{
		{"未配置 → 保持 disabled（省 token 的原意）", "", "disabled"},
		{"显式 disabled → disabled", "disabled", "disabled"},
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
// 退回旧行为「disabled」—— 不能因为拿不到配置就改成 enabled（会白烧 token）。
func TestSummarizeThinkingUnknownProvider(t *testing.T) {
	if got := SummarizeThinking(stubProvider{}); got != "disabled" {
		t.Fatalf("未知 provider 的 SummarizeThinking = %q, want \"disabled\"", got)
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
