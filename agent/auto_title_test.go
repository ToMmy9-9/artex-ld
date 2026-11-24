package agent

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"norma/llm"
)

type autoTitleProvider struct {
	reply string
	err   error
	req   llm.CompletionRequest
	// thinking 模拟 profile 配置的思考开关（"" = 未配置）。
	thinking string
}

// ConfiguredThinkingType 实现 llm.ThinkingAware，让测试能覆盖
// 「profile 配了 disabled 就真的关闭思考」这条规则。
func (p *autoTitleProvider) ConfiguredThinkingType() string { return p.thinking }

func (p *autoTitleProvider) Complete(_ context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	p.req = req
	if p.err != nil {
		return llm.Message{}, "", llm.Usage{}, p.err
	}
	return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock(p.reply)}}, "stop", llm.Usage{}, nil
}

func (p *autoTitleProvider) Stream(context.Context, llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(func(llm.StreamEvent, error) bool) {}
}

func TestGenerateAutoTitleCleansResponse(t *testing.T) {
	long := strings.Repeat("很长", 30)
	p := &autoTitleProvider{reply: "\n\"标题：" + long + "\"\n第二行"}
	got, err := GenerateAutoTitle(t.Context(), p, "用户希望整理项目设置")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(got, "\r\n") || strings.Contains(got, "标题：") || strings.HasPrefix(got, "\"") || len([]rune(got)) > 40 {
		t.Fatalf("unclean title %q", got)
	}
	// 思考开关跟随 profile 配置，而不是硬编码 "disabled"：
	// 未配置时不发送该字段（避免撞上要求 adaptive thinking 的模型）。
	// 这个 stub 不实现 ThinkingAware（拿不到配置），所以期望空值。
	if p.req.Thinking != "" || p.req.MaxTokens != 96 || len(p.req.Tools) != 0 {
		t.Fatalf("request options: %+v", p.req)
	}

	quoted := &autoTitleProvider{reply: `标题："整理项目设置"`}
	got, err = GenerateAutoTitle(t.Context(), quoted, "整理项目设置")
	if err != nil {
		t.Fatal(err)
	}
	if got != "整理项目设置" {
		t.Fatalf("nested quote/prefix cleanup: got %q", got)
	}
}

func TestGenerateAutoTitleProviderError(t *testing.T) {
	_, err := GenerateAutoTitle(t.Context(), &autoTitleProvider{err: errors.New("boom")}, "内容")
	if err == nil {
		t.Fatal("expected provider error")
	}
}

// TestGenerateAutoTitleFollowsProfileThinking 覆盖「profile 配了 thinking
// 就按配置走」这条规则：
//   - 配 "disabled" → 显式关闭思考（省 token，这类模型允许）
//   - 配 "enabled"  → 不能被覆盖成 disabled（模型强制要求）
//   - 未配置        → 不发送该字段，交给服务端决定
func TestGenerateAutoTitleFollowsProfileThinking(t *testing.T) {
	cases := []struct {
		configured string
		want       string
	}{
		{"disabled", "disabled"},
		{"enabled", "enabled"},
		{"", ""},
	}
	for _, c := range cases {
		p := &autoTitleProvider{reply: "整理项目设置", thinking: c.configured}
		if _, err := GenerateAutoTitle(t.Context(), p, "用户希望整理项目设置"); err != nil {
			t.Fatalf("configured=%q: %v", c.configured, err)
		}
		if p.req.Thinking != c.want {
			t.Fatalf("configured=%q → Thinking=%q, want %q", c.configured, p.req.Thinking, c.want)
		}
	}
}
