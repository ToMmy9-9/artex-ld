package server

import (
	"net"
	"net/http/httptest"
	"testing"
)

// TestClientIPIgnoresForgedForwardedFor 验证直连时伪造 XFF 头无效。
//
// 背景：clientIP 原先无条件采信 X-Forwarded-For。服务默认监听 :8787（所有
// 网卡），直连的客户端可以随便伪造这个头 —— 把每次请求的来源 IP 报成不同的
// 值，就能让「按 IP 限流」永远不触发（每个桶只失败 1 次）。登录限流是防
// 暴力破解的主要手段，这条路绕过去等于没有限流。
//
// 未配置 trusted_proxies 时必须一律用 RemoteAddr（连接的真实来源，伪造不了）。
func TestClientIPIgnoresForgedForwardedFor(t *testing.T) {
	s := &Server{} // 没有 m.pg → 无可信代理配置
	_ = s

	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	req.RemoteAddr = "10.1.2.3:5555"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")

	got := s.clientIPFrom(req)
	if got != "10.1.2.3" {
		t.Fatalf("直连时伪造的转发头被采信了: got %q, want 10.1.2.3（RemoteAddr）", got)
	}
}

// TestClientIPUsesForwardedForFromTrustedProxy 验证请求来自可信代理时，
// 转发头照常采信（否则前置 nginx 的部署会全部算成代理的 IP，限流过严）。
func TestClientIPUsesForwardedForFromTrustedProxy(t *testing.T) {
	// 直接构造可信网段，避免依赖 DB 设置。
	nets := mustParseCIDRs(t, "10.0.0.0/8")

	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	req.RemoteAddr = "10.0.0.9:5555"
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.9")

	if got := clientIPWithNets(req, nets); got != "203.0.113.7" {
		t.Fatalf("可信代理的转发头应被采信: got %q, want 203.0.113.7", got)
	}
}

// TestClientIPIgnoresForwardedForFromUntrustedProxy 验证不在可信网段的对端
// 即使带了转发头也不采信。
func TestClientIPIgnoresForwardedForFromUntrustedProxy(t *testing.T) {
	nets := mustParseCIDRs(t, "10.0.0.0/8")
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	req.RemoteAddr = "203.0.113.50:5555"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	if got := clientIPWithNets(req, nets); got != "203.0.113.50" {
		t.Fatalf("非可信代理的转发头不该被采信: got %q, want 203.0.113.50", got)
	}
}

func mustParseCIDRs(t *testing.T, list string) []*net.IPNet {
	t.Helper()
	return parseCIDRList(list)
}
