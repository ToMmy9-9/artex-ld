// Package llmhttp contains the shared outbound HTTP client construction used by
// LLM endpoints. An empty proxy deliberately means direct and does not inherit
// HTTP_PROXY/HTTPS_PROXY from the process environment.
package llmhttp

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// NewClient builds an HTTP client for an LLM endpoint. Supported proxies are
// http, https, and socks5, matching the profile configuration contract.
func NewClient(rawProxy string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// 【二开·review chat 卡死排查】禁用自动 gzip：SSE 流被 gzip 压缩后经中转代理
	// （Clash 等）缓冲不冲刷，SSE 实时性被破坏，客户端永远阻塞在 flate inflate
	// （goroutine dump 实锤：http2gzipReader→flate）。与 llm.defaultHTTPClient 同理。
	transport.DisableCompression = true
	rawProxy = strings.TrimSpace(rawProxy)
	if rawProxy == "" {
		transport.Proxy = nil
	} else {
		proxyURL, err := url.Parse(rawProxy)
		if err != nil {
			return nil, fmt.Errorf("llm: invalid proxy %q: %w", rawProxy, err)
		}
		switch proxyURL.Scheme {
		case "http", "https", "socks5":
			transport.Proxy = http.ProxyURL(proxyURL)
		case "":
			return nil, fmt.Errorf("llm: proxy %q missing scheme (use http://, https:// or socks5://)", rawProxy)
		default:
			return nil, fmt.Errorf("llm: unsupported proxy scheme %q (use http, https or socks5)", proxyURL.Scheme)
		}
	}
	return &http.Client{Transport: transport}, nil
}
