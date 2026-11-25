package traffic

import (
	"context"
	"testing"
	"time"
)

// TestQueryRecentExchangesFilters 验证「被拦命令 → 真实报文」的查询：method 精确
// 优先，path 收窄，时间窗外不返回。
func TestQueryRecentExchangesFilters(t *testing.T) {
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("open traffic: %v", err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	now := time.Now().Unix()
	seed := []struct {
		id, host, method, url string
		ts                    int64
	}{
		{"e1", "t.local", "GET", "http://t.local/api/orders/1", now - 10},
		{"e2", "t.local", "DELETE", "http://t.local/api/orders/1", now - 20},
		{"e3", "t.local", "GET", "http://t.local/other", now - 30},
		{"e4", "other.local", "GET", "http://other.local/api/orders/1", now - 5},
		{"e5", "t.local", "GET", "http://t.local/api/orders/1", now - 100000},
	}
	for _, s := range seed {
		if _, err := tr.DB().Exec(`INSERT INTO exchanges(id,ts,host,method,url_template,url,status,content_type,resp_len,path)
VALUES(?,?,?,?,?,?,?,?,?,?)`, s.id, s.ts, s.host, s.method, s.url, s.url, 200, "application/json", 10, s.url); err != nil {
			t.Fatalf("seed %s: %v", s.id, err)
		}
	}
	since := now - 3600

	got := tr.QueryRecentExchanges(context.Background(), since, "t.local", "DELETE", "/api/orders", 5)
	if len(got) != 1 || got[0].ID != "e2" {
		t.Fatalf("method filter = %+v, want [e2]", got)
	}

	// 空 method = 不过滤方法，只按 host+path+时间窗。
	got = tr.QueryRecentExchanges(context.Background(), since, "t.local", "", "/api/orders", 5)
	ids := map[string]bool{}
	for _, e := range got {
		ids[e.ID] = true
	}
	if !ids["e1"] || !ids["e2"] {
		t.Fatalf("path filter missing rows: %+v", got)
	}
	if ids["e3"] || ids["e4"] || ids["e5"] {
		t.Fatalf("path/host/time filter leaked: %+v", got)
	}
}
