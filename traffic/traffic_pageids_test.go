package traffic

import (
	"testing"

	mproxy "github.com/lqqyt2423/go-mitmproxy/proxy"
)

// TestPageIDsMatchesPage 锁定「全选全部筛选结果」与列表查询的��径一致性。
//
// PageIDs 是跨页全选的数据来源：如果它与 Page 的筛选条件漂移，用户全选后
// 批量操作（关联漏洞 / 送入重发 / 删除）就会作用到列表里看不到的报文上。
// 这里对几组筛选分别比对两者的 id 集合。
func TestPageIDsMatchesPage(t *testing.T) {
	tr, _ := openTraffic(t)
	status := func(code int) flowOpt { return func(f *mproxy.Flow) { f.Response.StatusCode = code } }
	tr.record(newFlow("api.example.com", "GET", "/api/users", nil, make([]byte, 10), status(200)))
	tr.record(newFlow("api.example.com", "GET", "/api/admin", nil, make([]byte, 100), status(404)))
	tr.record(newFlow("web.example.com", "POST", "/login", nil, make([]byte, 50), status(500)))

	cases := []struct {
		name  string
		query PageQuery
	}{
		{"无筛选", PageQuery{RespMin: -1, RespMax: -1}},
		{"状态码段", PageQuery{Status: "4xx", RespMin: -1, RespMax: -1}},
		{"精确状态码", PageQuery{Status: "500", RespMin: -1, RespMax: -1}},
		{"路径", PageQuery{Path: "/api/admin", RespMin: -1, RespMax: -1}},
		{"主机", PageQuery{Host: "web.example.com", RespMin: -1, RespMax: -1}},
		{"方法", PageQuery{Method: "post", RespMin: -1, RespMax: -1}},
		{"响应大小", PageQuery{RespMin: 60, RespMax: -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, total, err := tr.Page(tc.query, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			ids, err := tr.PageIDs(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			if len(ids) != total {
				t.Fatalf("PageIDs 返回 %d 条，Page 的 total 是 %d", len(ids), total)
			}
			listSet := map[string]bool{}
			for _, r := range rows {
				listSet[r.ID] = true
			}
			for _, m := range ids {
				if !listSet[m.ID] {
					t.Fatalf("PageIDs 返回了列表里没有的 id %q", m.ID)
				}
			}
		})
	}
}

// TestPageIDsSpansPages 确认 PageIDs 不受分页限制：列表一页只放 1 条时，
// 全选仍要拿到全部命中项 —— 这正是跨页全选的价值。
func TestPageIDsSpansPages(t *testing.T) {
	tr, _ := openTraffic(t)
	for i := 0; i < 5; i++ {
		tr.record(newFlow("api.example.com", "GET", "/api/x", nil, []byte("body")))
	}
	page, total, err := tr.Page(PageQuery{RespMin: -1, RespMax: -1}, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || total != 5 {
		t.Fatalf("单页列表应返回 1 条、total 为 5，实际 %d/%d", len(page), total)
	}
	ids, err := tr.PageIDs(PageQuery{RespMin: -1, RespMax: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 5 {
		t.Fatalf("PageIDs 应跨页返回全部 5 条，实际 %d", len(ids))
	}
}
