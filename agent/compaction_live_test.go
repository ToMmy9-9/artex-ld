package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"strings"
	"testing"
	"time"

	"artex/db"
	"norma/llm"
	"norma/transcript"
)

// recordingProvider records the MaxTokens of each request while delegating to a
// real provider. Used by the live test to prove the escalation actually happens.
type recordingProvider struct {
	inner   llm.Provider
	budgets []int
}

func (p *recordingProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return p.inner.Stream(ctx, req)
}

func (p *recordingProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	p.budgets = append(p.budgets, req.MaxTokens)
	return p.inner.Complete(ctx, req)
}

// TestLiveCompactionEscalates is an opt-in integration test against a real
// OpenAI-compatible endpoint. It builds a production-shaped block (38 dense
// nodes, each ~600 Chinese chars, with ids/edges/constraints), then calls the
// real compress() path and asserts a non-empty digest was produced.
//
// Run:
//
//	ARTEX_LIVE_OC_KEY=... go test ./agent -run TestLiveCompactionEscalates -count=1 -v
//
// Env:
//
//	ARTEX_LIVE_OC_BASE   default https://opencode.ai/inference/go/openai/v1
//	ARTEX_LIVE_OC_MODEL  default minimax-m3.1-flash
//	ARTEX_LIVE_OC_SESSION_HEADER default x-opencode-session
func TestLiveCompactionEscalates(t *testing.T) {
	key := os.Getenv("ARTEX_LIVE_OC_KEY")
	if key == "" {
		t.Skip("set ARTEX_LIVE_OC_KEY to run the live compaction test")
	}
	base := liveEnv("ARTEX_LIVE_OC_BASE", "https://opencode.ai/inference/go/openai/v1")
	model := liveEnv("ARTEX_LIVE_OC_MODEL", "minimax-m3.1-flash")
	header := liveEnv("ARTEX_LIVE_OC_SESSION_HEADER", "x-opencode-session")

	prov, err := Config{
		Format:           llm.FormatOpenAI,
		BaseURL:          base,
		APIKey:           key,
		Model:            model,
		SessionHeaderKey: header,
	}.NewProvider()
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	rec := &recordingProvider{inner: prov}

	const n = 38
	var cg []cgNode
	var edges []cgEdge
	byID := map[int64]*db.Node{}
	members := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		id := int64(1000 + i)
		members = append(members, id)
		cg = append(cg, cgNode{ID: id, Kind: db.KindFact, State: "observed"})
		payload, _ := jsonPayload(map[string]any{
			"summary":    denseSummary(i),
			"confidence": "observed",
		})
		byID[id] = &db.Node{ID: id, Kind: db.KindFact, State: "observed", Payload: payload}
		if i > 0 && i%4 != 0 {
			edges = append(edges, cgEdge{From: id - 1, Rel: "yields", To: id})
		}
	}
	// One shared anchor (context only, not a member).
	anchorPayload, _ := jsonPayload(map[string]any{"summary": "Acme 外网渗透：获取后台管理权限并验证订单越权"})
	byID[900] = &db.Node{ID: 900, Kind: db.KindIntent, State: "running", Payload: anchorPayload}

	g := newColdGraph(cg, edges)
	c := NewCompactor(rec, model)
	c.blockTimeout = 90 * time.Second
	c.retryBlockTimeout = 300 * time.Second

	ctx, cancel := context.WithTimeout(transcript.WithSessionID(context.Background(), "live-probe-exp1"), 10*time.Minute)
	defer cancel()

	start := time.Now()
	body, err := c.compress(ctx, g, block{Members: members, Anchors: []int64{900}}, byID)
	if err != nil {
		t.Fatalf("compress: %v (budgets=%v)", err, rec.budgets)
	}
	if strings.TrimSpace(body) == "" {
		t.Fatalf("empty digest (budgets=%v)", rec.budgets)
	}
	t.Logf("digest produced in %.1fs; budgets=%v; body=%q", time.Since(start).Seconds(), rec.budgets, truncate(body, 120))
	if len(rec.budgets) == 0 || rec.budgets[0] != 1500 {
		t.Fatalf("first attempt budget = %v, want 1500", rec.budgets)
	}
	if len(rec.budgets) > 1 && rec.budgets[1] != 8192 {
		t.Fatalf("escalated budget = %d, want 8192", rec.budgets[1])
	}
}

func denseSummary(i int) string {
	waf := []string{"360panyun", "深信服 secvue", "阿里云 WAF", "长亭雷池", "F5 BIG-IP"}[i%5]
	target := []string{"admin.acme.com", "api.acme.com", "www.acme.com", "shop.acme.com", "m.acme.com"}[i%5]
	return fmt.Sprintf("目标 %s 的接口 /api/v1/order/%d/detail 在未携带 Authorization 时返回 200 且包含 order_id、user_id、mobile、address 等字段；"+
		"复现步骤：①使用 %s 防护下的正常会话访问 /api/v1/order/%d 建立基线；②移除 Authorization 头重放同一请求；③对比响应体确认越权读取他人订单；"+
		"④检查响应头 X-Request-Id 与 WAF 返回 200 而非 403，排除拦截页；⑤限制条件：仅测试编号 %d 的只读 GET，不修改数据、不遍历全量、不导出用户信息。"+
		"证据：响应长度 4821 字节，包含 mobile=138****%04d，置信度 observed。"+
		"补充约束：该端点属于订单域，禁止 POST/PUT/DELETE；如遇 302 跳转需记录 Location 与 Set-Cookie；"+
		"如响应头出现 X-WAF-Request-Id 或 403 拦截页，需以正常会话重新建立基线后再判定；"+
		"同一 IP 每分钟不超过 10 次请求，禁止并发、禁止目录爆破、禁止携带他人 token；"+
		"对比字段需包含 order_id/user_id/mobile/address/amount/created_at 六项，任一缺失视为不完整证据。", target, i, waf, i, i, i)
}

func jsonPayload(v any) ([]byte, error) {
	return json.Marshal(v)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func liveEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
