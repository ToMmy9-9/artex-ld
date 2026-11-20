// scope_match_test.go：受管执行入口的范围判定测试。
//
// 这是"管控"真正生效的地方：外部工具要打的目标必须落在任务授权范围内。
// 判定太松 = 越权扫描；判定太紧 = 合法目标被拒、agent 反复重试。
package db

import "testing"

func scopeMatcher() *ScopeMatcher {
	return BuildScopeMatcher([]TaskScope{
		{Kind: "root_domain", Domain: "example.com"},
		{Kind: "subdomain", Domain: "admin.example.com"},
		{Kind: "ip", Net: "10.0.0.5/32"},
		{Kind: "cidr", Net: "192.168.1.0/24"},
	}, nil)
}

func TestInScopeAcceptsAuthorizedTargets(t *testing.T) {
	m := scopeMatcher()
	cases := []string{
		"example.com",
		"www.example.com",           // 子域命中根域
		"deep.sub.example.com",      // 更深子域
		"admin.example.com",         // 显式子域
		"http://example.com:8080/x", // URL
		"https://www.example.com/api",
		"10.0.0.5",
		"192.168.1.1",
		"192.168.1.254",
	}
	for _, c := range cases {
		if !m.InScope(c) {
			t.Errorf("target %q should be in scope", c)
		}
	}
}

func TestInScopeRejectsOutOfScopeTargets(t *testing.T) {
	m := scopeMatcher()
	cases := []string{
		"evil.com",
		"example.com.evil.com", // 后缀伪装
		"notexample.com",
		"10.0.0.6",
		"192.168.2.1",
		"8.8.8.8",
		"",
	}
	for _, c := range cases {
		if m.InScope(c) {
			t.Errorf("target %q should NOT be in scope", c)
		}
	}
}

func TestInScopeRejectsEverythingWhenNoScope(t *testing.T) {
	m := EmptyScopeMatcher()
	for _, c := range []string{"example.com", "10.0.0.5", "http://x"} {
		if m.InScope(c) {
			t.Errorf("empty scope must authorize nothing, but %q passed", c)
		}
	}
}

func TestInScopeCIDRMustBeCovered(t *testing.T) {
	m := scopeMatcher()
	// A /24 request inside an authorized /24 is fine.
	if !m.InScope("192.168.1.0/24") {
		t.Error("authorized /24 should pass")
	}
	// A wider request that merely overlaps is NOT covered.
	if m.InScope("192.168.0.0/16") {
		t.Error("a /16 overlapping but exceeding the authorized /24 must be rejected")
	}
	if m.InScope("10.0.0.0/8") {
		t.Error("a /8 covering the authorized single IP must be rejected")
	}
}

func TestInScopeSuffixConfusionRejected(t *testing.T) {
	m := scopeMatcher()
	for _, c := range []string{"example.com.attacker.net", "notexample.com", "example.company"} {
		if m.InScope(c) {
			t.Errorf("%q must not be treated as in scope", c)
		}
	}
}

func TestBuildScopeMatcherIncludesCompanyScopes(t *testing.T) {
	m := BuildScopeMatcher(nil, map[int64][]TaskScope{
		7: {{Kind: "root_domain", Domain: "gov.cn"}},
	})
	if !m.HasAny {
		t.Fatal("company scope should make the matcher non-empty")
	}
	if !m.InScope("www.gov.cn") {
		t.Error("company-scoped host should be in scope")
	}
	if m.InScope("example.com") {
		t.Error("unrelated host must not be in scope")
	}
}

func TestKeywordScopeMatchesSubstring(t *testing.T) {
	m := BuildScopeMatcher([]TaskScope{{Kind: "keyword", Value: "测试单位"}}, nil)
	if !m.InScope("test-测试单位.local") {
		t.Error("keyword scope should match a host containing the keyword")
	}
}
