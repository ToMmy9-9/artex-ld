package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"norma/llm"
)

// TestCompressHonoursPerBlockTimeout 验证单个块的压缩请求受 blockTimeout 约束，
// 不会继承整批的共享预算。
//
// 生产事故（2026-11-25 00:30）：一次 major 里 22 个块跑完后共享的 5 分钟
// maxDur 生效，剩下 43 个块在同一秒全部报 "context deadline exceeded"。
// 根因是 compress() 直接用批次的 ctx —— 一个卡住的请求吃掉剩余预算，
// 同批所有兄弟块跟着一起死。修复给每个块加了独立预算。
func TestCompressHonoursPerBlockTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	// 服务端拖延 5 秒才回，模拟卡死的上游。用固定睡眠而不是等 channel 关闭：
	// httptest.Server.Close() 会等未完成的 handler 返回，若 handler 阻塞在测试
	// 后置的 close() 上就自锁了。5 秒 < 外层 10 秒、> per-block 的 150ms，
	// 足以区分"用了 per-block 预算"和"漏用了外层 ctx"。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	prov, err := llm.NewProvider(llm.Config{
		Format:  llm.FormatOpenAI,
		BaseURL: srv.URL,
		APIKey:  "k",
		Model:   "m",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	c := NewCompactor(prov, "m")
	c.blockTimeout = 150 * time.Millisecond // 缩短以便快速断言
	// 超时现在会在更大预算下重试一次；这里把重试预算也缩短，避免测试等 300s。
	c.retryBlockTimeout = 150 * time.Millisecond

	// 外层 ctx 给足 10 秒，若 compress 没用自己的预算就会一直挂到 10 秒。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	_, err = c.compress(ctx, &coldGraph{}, block{Members: []int64{1, 2}}, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error from wedged upstream")
	}
	// 必须远早于外层 10 秒退出，证明用的是 per-block 预算而不是外层 ctx。
	if elapsed > 3*time.Second {
		t.Fatalf("compress took %v — per-block budget not applied (outer ctx leaked through)", elapsed)
	}
}

// TestZeroBlockTimeoutFallsBack 零值必须有兜底。若无兜底，
// context.WithTimeout(ctx, 0) 会立刻到期，反而让每一次压缩都必然失败。
func TestZeroBlockTimeoutFallsBack(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	prov, err := llm.NewProvider(llm.Config{
		Format:  llm.FormatOpenAI,
		BaseURL: srv.URL,
		APIKey:  "k",
		Model:   "m",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	c := NewCompactor(prov, "m")
	c.blockTimeout = 0 // 模拟未初始化

	out, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil)
	if err != nil {
		t.Fatalf("compress with zero blockTimeout must fall back, got: %v", err)
	}
	if out == "" {
		t.Fatal("empty body")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("request never reached upstream (hits=%d)", hits)
	}
}

// TestNewCompactorSetsBlockTimeout 构造器必须给出非零预算，否则上面的兜底
// 会长期生效，掩盖真实的调参意图。
func TestNewCompactorSetsBlockTimeout(t *testing.T) {
	c := NewCompactor(nil, "m")
	if c.blockTimeout <= 0 {
		t.Fatalf("blockTimeout = %v, want > 0", c.blockTimeout)
	}
	// 必须小于整批预算，否则单块就能吃掉全部。
	if c.blockTimeout >= c.maxDur {
		t.Fatalf("blockTimeout(%v) must be < maxDur(%v)", c.blockTimeout, c.maxDur)
	}
}
