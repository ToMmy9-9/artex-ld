package agent

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"norma/llm"
)

// scriptedCompactionProvider returns canned completion results and records the
// requests, so tests can assert the budget escalation protocol.
type scriptedCompactionProvider struct {
	results []compactionResult
	reqs    []llm.CompletionRequest
	calls   int
}

type compactionResult struct {
	text  string
	stop  string
	usage llm.Usage
	err   error
}

func (p *scriptedCompactionProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {}
}

func (p *scriptedCompactionProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	p.reqs = append(p.reqs, req)
	i := p.calls
	p.calls++
	if i >= len(p.results) {
		i = len(p.results) - 1
	}
	r := p.results[i]
	if r.err != nil {
		return llm.Message{}, "", llm.Usage{}, r.err
	}
	if r.text == "" {
		return llm.Message{}, r.stop, r.usage, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock(r.text)}}, r.stop, r.usage, nil
}

func newBudgetTestCompactor(p llm.Provider) *Compactor {
	return NewCompactor(p, "MiniMax-M3.1-Flash")
}

// TestCompressEscalatesOnTruncatedEmpty 是本次修复的核心：1.5k 预算被思考吃光、
// 返回 stop=max_tokens 的空正文时，必须升到 8k 重试一次并拿到正文。
func TestCompressEscalatesOnTruncatedEmpty(t *testing.T) {
	p := &scriptedCompactionProvider{results: []compactionResult{
		{stop: "max_tokens", usage: llm.Usage{OutputTokens: 1500}},
		{text: "综合结论", stop: "end_turn", usage: llm.Usage{OutputTokens: 900}},
	}}
	c := newBudgetTestCompactor(p)

	body, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if body != "综合结论" {
		t.Fatalf("body = %q", body)
	}
	if p.calls != 2 {
		t.Fatalf("calls = %d, want 2 (cheap attempt + one escalation)", p.calls)
	}
	if got := p.reqs[0].MaxTokens; got != 1500 {
		t.Fatalf("first budget = %d, want 1500", got)
	}
	if got := p.reqs[1].MaxTokens; got != 8192 {
		t.Fatalf("retry budget = %d, want 8192 (4k measured to fail too)", got)
	}
}

// TestCompressDoesNotEscalateOnNormalEmpty 空正文但 stop 不是 max_tokens 时，
// 升预算没有意义（provider 已经做过同预算重试），应当直接失败而不是烧钱。
func TestCompressDoesNotEscalateOnNormalEmpty(t *testing.T) {
	p := &scriptedCompactionProvider{results: []compactionResult{
		{stop: "end_turn"},
	}}
	c := newBudgetTestCompactor(p)

	if _, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil); err == nil {
		t.Fatal("expected error on empty normal stop")
	}
	if p.calls != 1 {
		t.Fatalf("calls = %d, want 1 (no escalation)", p.calls)
	}
}

// TestCompressCheapPathStaysCheap 普通块不应升级预算：首试成功就结束。
func TestCompressCheapPathStaysCheap(t *testing.T) {
	p := &scriptedCompactionProvider{results: []compactionResult{
		{text: "短结论", stop: "end_turn"},
	}}
	c := newBudgetTestCompactor(p)

	if _, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil); err != nil {
		t.Fatalf("compress: %v", err)
	}
	if p.calls != 1 || p.reqs[0].MaxTokens != 1500 {
		t.Fatalf("calls=%d budget=%d, want 1/1500", p.calls, p.reqs[0].MaxTokens)
	}
}

// TestCompressEscalationErrorCarriesDiagnostics 两次都截断时，错误里必须带
// stop / output_tokens / 最终预算，方便从日志直接判断是不是又一次预算不足。
func TestCompressEscalationErrorCarriesDiagnostics(t *testing.T) {
	p := &scriptedCompactionProvider{results: []compactionResult{
		{stop: "max_tokens", usage: llm.Usage{OutputTokens: 1500}},
		{stop: "max_tokens", usage: llm.Usage{OutputTokens: 8192}},
	}}
	c := newBudgetTestCompactor(p)

	_, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{"stop=max_tokens", "output_tokens=8192", "max_tokens=16384"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
	if p.calls != 3 {
		t.Fatalf("calls = %d, want 3 (1500 -> 8192 -> 16384)", p.calls)
	}
}

// TestCompressEscalatesThirdTier 8192 仍被截断时，再升到 16384 兜底。
// 实测最密的块在 8192 下只剩约 7% 余量（7626/8192），尾部需要第三档。
func TestCompressEscalatesThirdTier(t *testing.T) {
	p := &scriptedCompactionProvider{results: []compactionResult{
		{stop: "max_tokens", usage: llm.Usage{OutputTokens: 1500}},
		{stop: "max_tokens", usage: llm.Usage{OutputTokens: 8192}},
		{text: "第三档产出的完整结论", stop: "end_turn"},
	}}
	c := newBudgetTestCompactor(p)

	body, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if body != "第三档产出的完整结论" {
		t.Fatalf("body = %q", body)
	}
	if p.calls != 3 || p.reqs[2].MaxTokens != 16384 {
		t.Fatalf("calls=%d budgets=%d/%d/%d, want 1500/8192/16384", p.calls, p.reqs[0].MaxTokens, p.reqs[1].MaxTokens, p.reqs[2].MaxTokens)
	}
}

// TestCompressZeroBudgetFieldsFallBack 构造器之外的零值也要有安全兜底，
// 避免 WithTimeout(ctx, 0) 或 max_tokens=0 造成必然失败。
func TestCompressZeroBudgetFieldsFallBack(t *testing.T) {
	p := &scriptedCompactionProvider{results: []compactionResult{
		{stop: "max_tokens"},
		{text: "兜底结论", stop: "end_turn"},
	}}
	c := &Compactor{prov: p} // deliberately no NewCompactor

	body, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if body != "兜底结论" {
		t.Fatalf("body = %q", body)
	}
	if p.reqs[0].MaxTokens != 1500 || p.reqs[1].MaxTokens != 8192 {
		t.Fatalf("budgets = %d -> %d, want 1500 -> 8192", p.reqs[0].MaxTokens, p.reqs[1].MaxTokens)
	}
}

// TestCompressEscalatesTruncatedPartial 正文非空但被截断时也要升级：
// 半截结论落库比多打一次更糟。
func TestCompressEscalatesTruncatedPartial(t *testing.T) {
	p := &scriptedCompactionProvider{results: []compactionResult{
		{text: "这一片探索到……", stop: "max_tokens", usage: llm.Usage{OutputTokens: 1500}},
		{text: "完整结论：发现未授权上传（#12,#28）", stop: "end_turn"},
	}}
	c := newBudgetTestCompactor(p)

	body, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if !strings.HasPrefix(body, "完整结论") {
		t.Fatalf("body = %q, want the escalated full body", body)
	}
	if p.calls != 2 || p.reqs[1].MaxTokens != 8192 {
		t.Fatalf("calls=%d budgets=%v, want 2 calls with escalation", p.calls, p.reqs)
	}
}

// TestCompressKeepsLongestPartialWhenFinalTruncated 两次都截断时不能丢工作：
// 返回较长的那份，而不是报错。
func TestCompressKeepsLongestPartialWhenFinalTruncated(t *testing.T) {
	p := &scriptedCompactionProvider{results: []compactionResult{
		{text: "较长的半截结论，包含更多事实", stop: "max_tokens"},
		{text: "短句", stop: "max_tokens"},
	}}
	c := newBudgetTestCompactor(p)

	body, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if body != "较长的半截结论，包含更多事实" {
		t.Fatalf("body = %q, want the longest partial body", body)
	}
}

// TestCompressRetriesPerAttemptTimeout 单次超时是瞬时的：批次 ctx 还活着时
// 应在更大预算/更长超时下重试，而不是直接把块丢掉。
func TestCompressRetriesPerAttemptTimeout(t *testing.T) {
	p := &scriptedCompactionProvider{results: []compactionResult{
		{err: context.DeadlineExceeded},
		{text: "超时后重试得到的结论", stop: "end_turn"},
	}}
	c := newBudgetTestCompactor(p)

	body, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if body != "超时后重试得到的结论" {
		t.Fatalf("body = %q", body)
	}
	if p.calls != 2 {
		t.Fatalf("calls = %d, want 2 (timeout retry)", p.calls)
	}
}

// TestCompressDoesNotRetryNonTimeoutError HTTP 400/401 之类的错误重试只会烧配额。
func TestCompressDoesNotRetryNonTimeoutError(t *testing.T) {
	p := &scriptedCompactionProvider{results: []compactionResult{
		{err: errors.New("400 invalid params")},
	}}
	c := newBudgetTestCompactor(p)

	if _, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil); err == nil {
		t.Fatal("expected the 400 to surface")
	}
	if p.calls != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on non-timeout error)", p.calls)
	}
}

// TestCompressDoesNotRetryTimeoutAfterBatchCanceled 批次 ctx 已经取消时，
// 单块超时不再是瞬时失败，不能继续发请求。
func TestCompressDoesNotRetryTimeoutAfterBatchCanceled(t *testing.T) {
	p := &scriptedCompactionProvider{results: []compactionResult{
		{err: context.DeadlineExceeded},
	}}
	c := newBudgetTestCompactor(p)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.compress(ctx, &coldGraph{}, block{Members: []int64{1, 2}}, nil); err == nil {
		t.Fatal("expected error")
	}
	if p.calls != 1 {
		t.Fatalf("calls = %d, want 1 (batch ctx is gone)", p.calls)
	}
}
