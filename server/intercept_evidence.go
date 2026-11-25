// intercept_evidence.go：把「被平台拦截的 shell 命令」映射到已录制的真实报文。
//
// 需求背景：拦截提醒如果只回一句"禁止执行"，模型只知道被拒，不知道"被拒的是哪一次
// 真实交互"。把当次录制的请求/响应一起塞回去，提醒才有可核查的事实基础——
// 与 AntiFox 那条"证据不是模型写的摘要，而是真实报文"的原则一致。
//
// 匹配方式刻意保守：
//   - 只从命令里抽 method + URL（支持 curl -X/-d 与裸 URL），不猜；
//   - 只在最近时间窗内、限定为任务相关 host 的报文里找；
//   - 找不到就返回空，绝不因为"看起来像"而塞一条错的证据。
package server

import (
	"context"
	"regexp"
	"strings"
	"time"

	"artex/traffic"

	"artex/guard"
)

// blockEvidenceWindowSec 回看多久内的报文。拦截发生在"即将执行"的瞬间，
// 真正相关的报文通常就是刚刚录到的那几条；窗口给 30 分钟足够覆盖同一条工作流。
const blockEvidenceWindowSec = 30 * 60

// blockEvidenceMax 最多取几条。提醒里只内联 3 条，其余给出数量提示。
const blockEvidenceMax = 5

var (
	// 裸 URL（http/https）
	reBlockURL = regexp.MustCompile(`https?://[^\s'"\\|;&)]+`)
	// curl -X POST / --request PATCH
	reBlockMethod = regexp.MustCompile(`(?:-X|--request)[\s=]+([A-Za-z]+)`)
	// curl -d / --data / --data-raw / --data-binary → 默认 POST
	reBlockData = regexp.MustCompile(`(?:-d|--data(?:-raw|-binary)?)[\s=]`)
)

// blockCommandTarget extracts (method, url) from a shell command. Returns ok=false
// when the command carries no HTTP request we can pin to a recorded exchange.
func blockCommandTarget(cmd string) (method, url string, ok bool) {
	m := reBlockURL.FindString(cmd)
	if m == "" {
		return "", "", false
	}
	url = strings.TrimRight(m, ".,;")
	method = "GET"
	if mm := reBlockMethod.FindStringSubmatch(cmd); len(mm) == 2 {
		method = strings.ToUpper(strings.TrimSpace(mm[1]))
	} else if reBlockData.MatchString(cmd) {
		// curl -d 隐含 POST，即使没写 -X。
		method = "POST"
	}
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
	default:
		method = "GET"
	}
	return method, url, true
}

// trafficMatcherForTask builds the guard.BlockedTraffic matcher bound to one task.
// Returns nil when traffic capture is unavailable, so the guard simply omits the
// evidence paragraph instead of printing an empty header.
func (s *Server) trafficMatcherForTask(taskID int64) func(context.Context, string, string) []guard.BlockedTraffic {
	tr := s.m.Traffic()
	if tr == nil || taskID <= 0 {
		return nil
	}
	// 用户可以在设置里关掉「拦截提醒附带流量证据」——关闭时 matcher 为 nil，
	// guard 就只回文字提醒，不再查库。
	if !s.interceptTrafficEvidenceEnabled() {
		return nil
	}
	hosts, _ := s.m.pg.Assets().HostsByTask(taskID)
	return func(ctx context.Context, tool, cmd string) []guard.BlockedTraffic {
		if tool != "Bash" && tool != "shell_open" && tool != "shell_send" {
			return nil
		}
		method, rawURL, ok := blockCommandTarget(cmd)
		if !ok {
			return nil
		}
		host := hostOfURL(rawURL)
		if host == "" {
			return nil
		}
		// 任务 host 白名单存在时收窄到本任务；否则依赖 host 本身（命令里写死的
		// 目标）——仍然只取真实报文，只是可能不专属本任务。
		if len(hosts) > 0 && !hostInScope(host, hosts) {
			return nil
		}
		path := pathOfURL(rawURL)
		since := time.Now().Add(-blockEvidenceWindowSec * time.Second).Unix()
		// 两段查询：
		//  ① 同一 method 的报文——最贴合"被拦的就是这次请求"；
		//  ② 被拦的写操作通常一条都没发出去（命令在平台层就被拒），此时退回
		//     "同一 host+path 的邻近报文"，让模型仍能看到这个端点上真实发生过什么。
		// ②刻意排在①之后而不是合并：method 精确匹配的证据更强，不能被邻近报文挤掉。
		if got := tr.QueryRecentExchanges(ctx, since, host, method, path, blockEvidenceMax); len(got) > 0 {
			return toBlockedTraffic(got)
		}
		got := tr.QueryRecentExchanges(ctx, since, host, "", path, blockEvidenceMax)
		return toBlockedTraffic(got)
	}
}

// toBlockedTraffic converts traffic rows to the guard-facing shape.
func toBlockedTraffic(rows []traffic.ExchangeMeta) []guard.BlockedTraffic {
	out := make([]guard.BlockedTraffic, 0, len(rows))
	for _, r := range rows {
		out = append(out, guard.BlockedTraffic{
			ID:     r.ID,
			Method: r.Method,
			URL:    r.URL,
			Status: r.Status,
		})
	}
	return out
}

// hostInScope reports whether host matches one of the task's host candidates
// (which may carry a port).
func hostInScope(host string, hosts []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if h == host || strings.HasPrefix(h, host+":") {
			return true
		}
	}
	return false
}

// hostOfURL returns the hostname (no port) of an absolute URL.
func hostOfURL(raw string) string {
	rest := raw
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		rest = rest[i+1:]
	}
	if i := strings.LastIndex(rest, ":"); i >= 0 && !strings.Contains(rest, "]") {
		rest = rest[:i]
	}
	return strings.ToLower(strings.TrimSpace(rest))
}

// pathOfURL returns the path portion (without query), or "" for the root.
func pathOfURL(raw string) string {
	rest := raw
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}
	i := strings.IndexAny(rest, "/?#")
	if i < 0 {
		return ""
	}
	rest = rest[i:]
	if j := strings.IndexAny(rest, "?#"); j >= 0 {
		rest = rest[:j]
	}
	if rest == "/" {
		return ""
	}
	return rest
}
