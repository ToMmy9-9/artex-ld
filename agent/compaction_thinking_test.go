package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"norma/llm"
)

// TestCompactSummaryOmitsThinkingField 端到端验证压缩请求里不带 thinking 字段。
//
// 为什么这条最关键：MiniMax-M3.1-Flash 这类模型**强制要求 adaptive thinking**，
// 收到 thinking.type="disabled" 会直接回 HTTP 400。压缩是长任务的必需品
// （上下文压不下去 → 任务因超限失败），所以这一条 400 就等于该模型下压缩功能
// 完全不可用。生产库可核到的记录是：11-24 09:00–18:32 共 34 条 disabled
// 报错（更早的记录已被 llm_records 保留策略清理，原提交信息里的"360 次"无法复核）。
//
// 这里的 profile 没配置 thinking（与生产库 profile 267 一致），所以请求体里
// 必须**根本没有** thinking 字段，而不是 thinking.type=disabled。
func TestCompactSummaryOmitsThinkingField(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"summary body"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	prov, err := llm.NewProvider(llm.Config{
		Format:  llm.FormatOpenAI,
		BaseURL: srv.URL,
		APIKey:  "k",
		Model:   "MiniMax-M3.1-Flash",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	c := NewCompactor(prov, "MiniMax-M3.1-Flash")
	out, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("压缩返回空 body")
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal request body: %v\n%s", err, raw)
	}
	if th, ok := m["thinking"]; ok {
		t.Fatalf("压缩请求不应带 thinking 字段，实际=%v —— 该模型会回 HTTP 400", th)
	}
}

// TestCompactSummaryHonoursExplicitDisabled profile 显式配了 disabled 时，
// 压缩请求应带上 thinking.type=disabled（用户的选择，这类模型允许）。
func TestCompactSummaryHonoursExplicitDisabled(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"summary body"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	prov, err := llm.NewProvider(llm.Config{
		Format:       llm.FormatOpenAI,
		BaseURL:      srv.URL,
		APIKey:       "k",
		Model:        "some-model",
		ThinkingType: "disabled",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	c := NewCompactor(prov, "some-model")
	if _, err := c.compress(context.Background(), &coldGraph{}, block{Members: []int64{1, 2}}, nil); err != nil {
		t.Fatalf("compress: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal request body: %v\n%s", err, raw)
	}
	th, ok := m["thinking"].(map[string]any)
	if !ok || th["type"] != "disabled" {
		t.Fatalf("thinking=%v, want {type: disabled}", m["thinking"])
	}
}
