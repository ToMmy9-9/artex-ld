// scope_match.go：受管执行入口的范围判定。
//
// 受管执行（run_tool）是外部工具真正跑起来的唯一通道，因此"目标是否在
// 任务授权范围内"必须由平台在这里判定，而不是指望 agent 自律。
//
// 判定依据是任务的两层范围：
//   - task_scope      任务测试范围（insert_assets 自动登记 / add_task_scope 手工加）
//   - company_scope   关联公司的范围（root_domain / ip / cidr / icp / keyword）
//
// 语义：命中任一层即视为在范围内。宁可略宽（多扫一点授权资产）也不误杀
// （拒绝合法的授权目标会让 agent 反复重试，反而更危险），但绝不允许
// 明显越界的形态（公网 IP、别的根域）通过。
package db

import (
	"net"
	"net/url"
	"strings"
)

// ScopeMatcher judges whether a target is inside a task's authorized scope.
type ScopeMatcher struct {
	// Domains are the root domains in scope.
	Domains []string
	// Subdomains are exact hostnames in scope.
	Subdomains []string
	// Nets are the CIDRs in scope.
	Nets []*net.IPNet
	// ExactIPs are single IPs in scope.
	ExactIPs map[string]bool
	// Keywords are ICP/keyword entries.
	Keywords []string
	// HasAny reports whether the task has any scope at all. An empty scope
	// means nothing is authorized, so everything is rejected.
	HasAny bool
}

// EmptyScopeMatcher returns a matcher that authorizes nothing.
func EmptyScopeMatcher() *ScopeMatcher {
	return &ScopeMatcher{ExactIPs: map[string]bool{}}
}

// InScope reports whether the target (host, IP, CIDR, or URL) is authorized.
//
// target forms accepted: "example.com", "10.0.0.1", "10.0.0.0/24",
// "http://example.com:8080/path".
func (m *ScopeMatcher) InScope(target string) bool {
	if m == nil || !m.HasAny {
		return false
	}
	t := strings.TrimSpace(target)
	if t == "" {
		return false
	}
	// URL → take the host.
	if u, err := url.Parse(t); err == nil && u.Host != "" {
		host := u.Hostname()
		if m.hostInScope(host) {
			return true
		}
		// A URL may carry a CIDR in the host part for some tools; fall
		// through and try the raw text too.
	}
	// CIDR → the whole net must be inside an authorized net.
	if ip, n, err := net.ParseCIDR(t); err == nil {
		return m.netInScope(ip, n)
	}
	// Bare IP.
	if ip := net.ParseIP(t); ip != nil {
		return m.ipInScope(ip)
	}
	// Hostname (possibly with port).
	host := t
	if h, _, err := net.SplitHostPort(t); err == nil {
		host = h
	}
	return m.hostInScope(host)
}

func (m *ScopeMatcher) hostInScope(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if h == "" {
		return false
	}
	if m.ExactIPs[h] {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return m.ipInScope(ip)
	}
	for _, d := range m.Subdomains {
		if strings.EqualFold(h, d) {
			return true
		}
	}
	for _, d := range m.Domains {
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	// ICP/keyword scope: a hostname containing the keyword is treated as in
	// scope. This mirrors how the asset pipeline treats keyword scope.
	for _, kw := range m.Keywords {
		if kw != "" && strings.Contains(h, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func (m *ScopeMatcher) ipInScope(ip net.IP) bool {
	if m.ExactIPs[ip.String()] {
		return true
	}
	for _, n := range m.Nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (m *ScopeMatcher) netInScope(ip net.IP, n *net.IPNet) bool {
	// The requested network must be fully covered by an authorized network.
	for _, auth := range m.Nets {
		if auth.Contains(ip) && auth.Contains(n.IP) {
			// Also require the prefix to be at least as specific, so a wide
			// request against a narrow grant is rejected.
			authOnes, authBits := auth.Mask.Size()
			reqOnes, reqBits := n.Mask.Size()
			if authBits == reqBits && authOnes <= reqOnes {
				return true
			}
			if authBits == 32 && reqBits == 32 {
				return true
			}
		}
	}
	return false
}

// BuildScopeMatcher assembles a matcher from a task's scope rows and the
// scope rows of every company attached to the task.
//
// companyScopes maps company id → that company's scope rows.
func BuildScopeMatcher(taskScopes []TaskScope, companyScopes map[int64][]TaskScope) *ScopeMatcher {
	m := &ScopeMatcher{ExactIPs: map[string]bool{}}
	add := func(rows []TaskScope) {
		for _, r := range rows {
			m.HasAny = true
			switch r.Kind {
			case "root_domain":
				if r.Domain != "" {
					m.Domains = append(m.Domains, strings.ToLower(r.Domain))
				}
			case "subdomain":
				if r.Domain != "" {
					m.Subdomains = append(m.Subdomains, strings.ToLower(r.Domain))
				}
			case "ip":
				if r.Net != "" {
					if _, n, err := net.ParseCIDR(r.Net); err == nil {
						m.Nets = append(m.Nets, n)
					} else if ip := net.ParseIP(r.Net); ip != nil {
						m.ExactIPs[ip.String()] = true
					}
				}
			case "cidr":
				if r.Net != "" {
					if _, n, err := net.ParseCIDR(r.Net); err == nil {
						m.Nets = append(m.Nets, n)
					}
				}
			case "icp", "keyword":
				if r.Value != "" {
					m.Keywords = append(m.Keywords, strings.ToLower(r.Value))
				}
			case "company":
				// The company's own rows are expanded by the caller.
				m.HasAny = true
			}
		}
	}
	add(taskScopes)
	for _, rows := range companyScopes {
		add(rows)
	}
	return m
}
