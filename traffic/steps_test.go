package traffic

import (
	"fmt"
	"testing"
)

// seedExchange 往流量库塞一条录制记录（与 record() 落库形状一致）。
func seedExchange(t *testing.T, tr *Traffic, id, host, method, url, path string, ts int64) {
	t.Helper()
	_, err := tr.DB().Exec(`INSERT INTO exchanges(id,ts,host,method,url_template,url,status,content_type,req_len,resp_len,path)
VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		id, ts, host, method, path, url, 200, "text/plain", 0, 0, path)
	if err != nil {
		t.Fatalf("seed exchange %s: %v", id, err)
	}
}

// TestResolveStepsMatchesRecordedExchange 验证：复现步骤里逐字写的请求，
// 能被解析到真实录制的那个包（这是 AntiFox「每步关联真实流量包」的基础）。
func TestResolveStepsMatchesRecordedExchange(t *testing.T) {
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	seedExchange(t, tr, "ex-1", "shop.example.com", "POST", "http://shop.example.com/api/order?id=7", "/api/order", 1000)

	steps := []StepRef{{
		Request: "POST /api/order?id=7 HTTP/1.1\r\nHost: shop.example.com\r\nContent-Type: application/json\r\n\r\n{\"sku\":1}",
	}}
	got := tr.ResolveSteps(0, 2000, steps)
	if len(got) != 1 {
		t.Fatalf("ResolveSteps 返回 %d 个结果，期望 1", len(got))
	}
	if got[0] != "ex-1" {
		t.Fatalf("解析到 %q，期望 %q", got[0], "ex-1")
	}
}

// TestResolveStepsKeepsStepOrder 验证：同一端点的多次相同请求，按步骤先后
// 依次匹配（最早 → 其后），不��颠倒 —— 这是「步骤顺序消歧」的关键。
func TestResolveStepsKeepsStepOrder(t *testing.T) {
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	// 同一 URL 录了三次。
	seedExchange(t, tr, "ex-a", "shop.example.com", "GET", "http://shop.example.com/api/ping", "/api/ping", 1000)
	seedExchange(t, tr, "ex-b", "shop.example.com", "GET", "http://shop.example.com/api/ping", "/api/ping", 2000)
	seedExchange(t, tr, "ex-c", "shop.example.com", "GET", "http://shop.example.com/api/ping", "/api/ping", 3000)

	steps := []StepRef{
		{Request: "GET /api/ping HTTP/1.1\r\nHost: shop.example.com\r\n\r\n"},
		{Request: "GET /api/ping HTTP/1.1\r\nHost: shop.example.com\r\n\r\n"},
	}
	got := tr.ResolveSteps(0, 5000, steps)
	if len(got) != 2 {
		t.Fatalf("ResolveSteps 返回 %d 个结果，期望 2", len(got))
	}
	if got[0] != "ex-a" || got[1] != "ex-b" {
		t.Fatalf("步骤顺序错乱：got=%v，期望 [ex-a ex-b]", got)
	}
}

// TestResolveStepsRespectsTimeWindow 验证：落在时间窗之外的录制不参与匹配。
func TestResolveStepsRespectsTimeWindow(t *testing.T) {
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	seedExchange(t, tr, "ex-old", "shop.example.com", "GET", "http://shop.example.com/api/x", "/api/x", 1000)
	seedExchange(t, tr, "ex-new", "shop.example.com", "GET", "http://shop.example.com/api/x", "/api/x", 9000)

	steps := []StepRef{{Request: "GET /api/x HTTP/1.1\r\nHost: shop.example.com\r\n\r\n"}}
	// 窗口只覆盖 9000 那条。
	got := tr.ResolveSteps(5000, 10000, steps)
	if len(got) != 1 || got[0] != "ex-new" {
		t.Fatalf("时间窗过滤失效：got=%v，期望 [ex-new]", got)
	}
}

// TestParseStepRequestForms 验证：步骤里的请求可能是标准 HTTP 报文，也可能是
// agent 实际执行的 curl/shell 命令 —— 两种都要能解析出 method 与 path。
func TestParseStepRequestForms(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		wantMethod string
		wantPath   string
	}{
		{
			name:       "标准 HTTP 报文",
			text:       "POST /api/order?id=7 HTTP/1.1\nHost: h\nContent-Type: application/json\n\n{\"a\":1}",
			wantMethod: "POST",
			wantPath:   "/api/order",
		},
		{
			name:       "curl 命令",
			text:       `curl -X POST "https://shop.example.com/api/login" -d '{"u":1}'`,
			wantMethod: "POST",
			wantPath:   "/api/login",
		},
		{
			name:       "裸 URL",
			text:       "GET https://shop.example.com/api/list?page=2",
			wantMethod: "GET",
			wantPath:   "/api/list",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method, path, _, _ := parseStepRequest(tc.text)
			if method != tc.wantMethod {
				t.Errorf("method = %q，期望 %q", method, tc.wantMethod)
			}
			if path != tc.wantPath {
				t.Errorf("path = %q，期望 %q", path, tc.wantPath)
			}
		})
	}
}

// TestMetaByIDs 验证：按 id 批量取报文元数据（报告与发现页用它渲染关联包）。
func TestMetaByIDs(t *testing.T) {
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	seedExchange(t, tr, "ex-1", "a.example.com", "GET", "http://a.example.com/x", "/x", 10)
	seedExchange(t, tr, "ex-2", "b.example.com", "GET", "http://b.example.com/y", "/y", 20)

	metas, err := tr.MetaByIDs([]string{"ex-1", "ex-2", "ex-missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("取到 %d 条元数据，期望 2（不存在的 id 应被跳过）", len(metas))
	}
	byID := map[string]ExchangeMeta{}
	for _, m := range metas {
		byID[m.ID] = m
	}
	if byID["ex-1"].Host != "a.example.com" || byID["ex-2"].Host != "b.example.com" {
		t.Fatalf("元数据 host 不对：%+v", metas)
	}
	_ = fmt.Sprint(metas)
}
