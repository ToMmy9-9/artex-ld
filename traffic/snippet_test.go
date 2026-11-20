package traffic

import (
	"strings"
	"testing"
)

// TestBodyMatchSnippetsHighlightsHit 验证：按 body 关键字过滤流量时，每行附带一段
// 高亮摘录，指明关键字**命中在哪儿** —— 否则用户只知道"这行匹配了"，还得逐条打开
// 才知道匹配的是请求头还是响应体。
func TestBodyMatchSnippetsHighlightsHit(t *testing.T) {
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	// 直接往正文表里写两条记录（一命中、一不命中）。
	seed := func(id, req, resp string) {
		t.Helper()
		if _, err := tr.DB().Exec(`INSERT INTO exchanges(id,ts,host,method,url_template,url,status,content_type,req_len,resp_len,path)
VALUES(?,?,?,?,?,?,?,?,?,?,'')`, id, 100, "h", "GET", "/x", "http://h/x", 200, "text/plain", len(req), len(resp)); err != nil {
			t.Fatalf("seed exchange %s: %v", id, err)
		}
		if _, err := tr.DB().Exec(`INSERT OR REPLACE INTO exchange_bodies(id,req_head,req_body,req_blob,resp_head,resp_body,resp_blob)
VALUES(?,?,?,'',?,?,'')`, id, "", []byte(req), "", []byte(resp)); err != nil {
			t.Fatalf("seed body %s: %v", id, err)
		}
	}
	seed("hit-1", "GET /a HTTP/1.1\r\nHost: h\r\nX-Probe: SEEDMARKER42\r\n\r\n", `{"ok":true}`)
	seed("miss-1", "GET /b HTTP/1.1\r\nHost: h\r\n\r\n", `{"ok":false}`)

	snips := tr.BodyMatchSnippets([]string{"hit-1", "miss-1"}, "SEEDMARKER42")
	snip, ok := snips["hit-1"]
	if !ok {
		t.Fatal("hit-1 should have a snippet")
	}
	if !strings.Contains(snip, "\x01SEEDMARKER42\x02") {
		t.Errorf("snippet should wrap the hit in \\x01…\\x02, got %q", snip)
	}
	if _, ok := snips["miss-1"]; ok {
		t.Error("miss-1 has no keyword hit and must not get a snippet")
	}
}

// TestBodyMatchSnippetsEmptyKeyword 验证：空关键字不产生摘录（不会给每一行都塞一段噪音）。
func TestBodyMatchSnippetsEmptyKeyword(t *testing.T) {
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if got := tr.BodyMatchSnippets([]string{"x"}, "  "); len(got) != 0 {
		t.Errorf("empty keyword should yield no snippets, got %v", got)
	}
}

// TestStatusCountsBuckets 验证：状态码分布按 2xx/3xx/4xx/5xx 分桶，且计数覆盖
// **全部**记录（不是当前页）—— 仪表盘要的是真实分布，不是当前页那几行的分布。
func TestStatusCountsBuckets(t *testing.T) {
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	seed := func(id string, status int) {
		t.Helper()
		if _, err := tr.DB().Exec(`INSERT INTO exchanges(id,ts,host,method,url_template,url,status,content_type,req_len,resp_len,path)
VALUES(?,?,?,?,?,?,?,?,?,?,'')`, id, 100, "h", "GET", "/x", "http://h/x", status, "text/plain", 0, 0); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("a", 200)
	seed("b", 204)
	seed("c", 301)
	seed("d", 404)
	seed("e", 500)

	buckets, err := tr.StatusCounts(nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if buckets[2] != 2 || buckets[3] != 1 || buckets[4] != 1 || buckets[5] != 1 {
		t.Errorf("buckets = %v, want {2:2, 3:1, 4:1, 5:1}", buckets)
	}

	// 主机维度过滤。
	buckets, err = tr.StatusCounts([]string{"other"}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 0 {
		t.Errorf("host-scoped buckets = %v, want empty", buckets)
	}
}
