package server

import "testing"

// TestBlockCommandTarget 验证从 shell 命令里提取 method+URL 的规则：
// 只认能确定的形态，抽不出就放弃（宁缺毋滥）。
func TestBlockCommandTarget(t *testing.T) {
	cases := []struct {
		cmd    string
		method string
		url    string
		ok     bool
	}{
		{`curl -X DELETE http://t.local/api/orders/1`, "DELETE", "http://t.local/api/orders/1", true},
		{`curl --request PATCH https://t.local/x`, "PATCH", "https://t.local/x", true},
		{`curl -d 'a=1' http://t.local/login`, "POST", "http://t.local/login", true},
		{`curl http://t.local/api/me`, "GET", "http://t.local/api/me", true},
		{`ls -la /tmp`, "", "", false},
		{`echo hello`, "", "", false},
	}
	for _, c := range cases {
		m, u, ok := blockCommandTarget(c.cmd)
		if ok != c.ok {
			t.Fatalf("%q ok=%v want %v", c.cmd, ok, c.ok)
		}
		if !c.ok {
			continue
		}
		if m != c.method || u != c.url {
			t.Fatalf("%q => (%q,%q), want (%q,%q)", c.cmd, m, u, c.method, c.url)
		}
	}
}

func TestHostAndPathOfURL(t *testing.T) {
	if got := hostOfURL("https://API.T.local:8443/a/b?x=1"); got != "api.t.local" {
		t.Fatalf("host = %q", got)
	}
	if got := pathOfURL("https://t.local/api/orders/1?x=1"); got != "/api/orders/1" {
		t.Fatalf("path = %q", got)
	}
	if got := pathOfURL("https://t.local/"); got != "" {
		t.Fatalf("root path = %q, want empty", got)
	}
	if got := pathOfURL("https://t.local"); got != "" {
		t.Fatalf("no path = %q, want empty", got)
	}
}

func TestHostInScope(t *testing.T) {
	hosts := []string{"api.t.local:8443", "www.t.local"}
	if !hostInScope("api.t.local", hosts) {
		t.Fatal("host with port should match bare host")
	}
	if hostInScope("evil.local", hosts) {
		t.Fatal("unrelated host must not match")
	}
}
