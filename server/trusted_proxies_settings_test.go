package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPutSettingsTrustedProxies 验证可信代理网段设置的读写与校验。
//
// 背景：X-Forwarded-For 无条件采信时，直连客户端能伪造它绕过「按 IP 限流」。
// 修复是「只在请求来自可信代理时采信」，而可信网段要能从设置里改 —— 部署在
// nginx 后面时必须告诉平台反代的地址，否则所有请求都会算成反代的 IP。
//
// 校验也要有：写错的网段会让「该信的头不被信」静默失效，用户以为配好了
// 其实没生效 —— 所以写的时候必须报错。
func TestPutSettingsTrustedProxies(t *testing.T) {
	s, pg := newTestServer(t)
	s.m = &Manager{pg: pg, tasks: map[string]*Task{}}
	t.Cleanup(func() {
		_, _ = pg.Exec(`DELETE FROM settings WHERE key = $1`, settingTrustedProxies)
	})

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body))
		s.putSettings(rec, req)
		return rec
	}

	// 1) 写合法值 → 200，且能读回来
	if rec := put(`{"auth_trusted_proxies":"10.0.0.0/8,192.168.1.1"}`); rec.Code != http.StatusOK {
		t.Fatalf("写可信代理网段应返回 200，实际 %d（body=%s）", rec.Code, rec.Body.String())
	}
	got, ok, err := pg.GetSetting(settingTrustedProxies)
	if err != nil || !ok {
		t.Fatalf("读设置失败: %v ok=%v", err, ok)
	}
	if got != "10.0.0.0/8,192.168.1.1" {
		t.Fatalf("设置值=%q, want 10.0.0.0/8,192.168.1.1", got)
	}

	// 2) 写非法值 → 400，且不落库
	if rec := put(`{"auth_trusted_proxies":"not-an-ip"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法网段应返回 400，实际 %d", rec.Code)
	}
	after, _, _ := pg.GetSetting(settingTrustedProxies)
	if after != got {
		t.Fatalf("非法值不该落库：设置变成 %q", after)
	}

	// 3) 空串 = 清空（回到「不采信转发头」）
	if rec := put(`{"auth_trusted_proxies":""}`); rec.Code != http.StatusOK {
		t.Fatalf("清空应返回 200，实际 %d", rec.Code)
	}
}
