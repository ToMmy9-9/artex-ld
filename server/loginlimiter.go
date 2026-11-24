package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"artex/db"
)

// 登录失败限流器：按 IP 与账号两个维度各记一个滑动窗口计数，窗口内失败数超阈值即锁定一段
// 冷却期。进程内存态（单实例部署够用，重启清零可接受），不做 DB 写——避免每次失败都落库
// 反成放大攻击面。锁定/解锁等关键事件由调用方写审计日志，可在日志页查看。
//
// 双维度的意义：仅按账号锁 → 攻击者可用错误口令刷某账号把合法用户锁死（DoS）；叠加按 IP
// 锁 → 单一来源横扫多个用户名也会被按 IP 挡下。两桶任一命中即拒绝。
//
// 与上游 AntiFox 同源，差别在于本仓库只有单一管理员账号（用户名固定 ARTEX），
// 故账号维度退化为「登录名」这一维度，IP 维度仍然独立生效。

type loginBucket struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]*loginBucket
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{buckets: map[string]*loginBucket{}}
}

// loginPolicy 是限流器的可调参数，从 settings 读取（未设则用安全默认）。
type loginPolicy struct {
	enabled       bool
	userThreshold int
	ipThreshold   int
	window        time.Duration
	cooldown      time.Duration
}

// loginPolicy 组装当前策略：各项可用 settings 覆盖，非法值回退默认。
func (s *Server) loginPolicy() loginPolicy {
	pol := loginPolicy{
		enabled:       true,
		userThreshold: 10,
		ipThreshold:   30,
		window:        15 * time.Minute,
		cooldown:      15 * time.Minute,
	}
	if s.m == nil || s.m.pg == nil {
		return pol
	}
	pg := s.m.pg
	pol.enabled = pg.GetBool("auth.lockout_enabled", true)
	if v := pg.GetIntSetting("auth.lockout_user_threshold"); v > 0 {
		pol.userThreshold = v
	}
	if v := pg.GetIntSetting("auth.lockout_ip_threshold"); v > 0 {
		pol.ipThreshold = v
	}
	if v := pg.GetIntSetting("auth.lockout_window_minutes"); v > 0 {
		pol.window = time.Duration(v) * time.Minute
	}
	if v := pg.GetIntSetting("auth.lockout_cooldown_minutes"); v > 0 {
		pol.cooldown = time.Duration(v) * time.Minute
	}
	return pol
}

func ipKey(ip string) string     { return "ip:" + ip }
func userKey(user string) string { return "u:" + user }

// locked 报告该 IP 或账号是否处于锁定冷却中，并返回剩余冷却时长（取两桶较大者）。
func (l *loginLimiter) locked(ip, user string) (bool, time.Duration) {
	// 限流器可能没被初始化（测试直接构造 Server 时就是 nil）。没有限流器 = 不限流，
	// 不能因此 panic —— 认证路径不能因为限流器缺失而整个挂掉。
	if l == nil {
		return false, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	var remain time.Duration
	for _, k := range []string{ipKey(ip), userKey(user)} {
		if b := l.buckets[k]; b != nil && now.Before(b.lockedUntil) {
			if r := time.Until(b.lockedUntil); r > remain {
				remain = r
			}
		}
	}
	return remain > 0, remain
}

// fail 记一次失败到 IP 桶与账号桶。返回 true 表示本次失败使某个桶【新进入】锁定状态
// （供调用方写一条锁定审计）。策略关闭时不记任何状态，locked 永远为假。
func (l *loginLimiter) fail(ip, user string, pol loginPolicy) bool {
	if l == nil || !pol.enabled {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.sweepLocked(now, pol)
	newlyLocked := false
	if l.recordLocked(ipKey(ip), now, pol.window, pol.cooldown, pol.ipThreshold) {
		newlyLocked = true
	}
	if user != "" && l.recordLocked(userKey(user), now, pol.window, pol.cooldown, pol.userThreshold) {
		newlyLocked = true
	}
	return newlyLocked
}

// recordLocked 在一个桶上记一次失败，必要时滑动窗口，达阈值则设定冷却到期时间。
// 返回 true 表示本次恰好触发锁定。调用方持有 l.mu。
func (l *loginLimiter) recordLocked(key string, now time.Time, window, cooldown time.Duration, threshold int) bool {
	b := l.buckets[key]
	if b == nil {
		b = &loginBucket{windowStart: now}
		l.buckets[key] = b
	}
	if now.Sub(b.windowStart) > window {
		b.windowStart = now
		b.count = 0
	}
	b.count++
	if b.count >= threshold {
		b.lockedUntil = now.Add(cooldown)
		b.count = 0
		b.windowStart = now
		return true
	}
	return false
}

// reset 清掉该 IP 与账号的失败计数（登录成功时调用）。
func (l *loginLimiter) reset(ip, user string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, ipKey(ip))
	delete(l.buckets, userKey(user))
}

// sweepLocked 在桶数量偏大时清掉既不在锁定中、窗口也已过期的陈旧桶，防止海量不同
// 用户名/来源把 map 撑大成内存压力。调用方持有 l.mu。
func (l *loginLimiter) sweepLocked(now time.Time, pol loginPolicy) {
	if len(l.buckets) < 4096 {
		return
	}
	for k, b := range l.buckets {
		if now.After(b.lockedUntil) && now.Sub(b.windowStart) > pol.window {
			delete(l.buckets, k)
		}
	}
}

// 注意：这里**没有** clientIP(r) 这个「无条件信任转发头」的版本了。
// 它曾被登录限流与审计使用，但直连的客户端能伪造 X-Forwarded-For 让「按 IP
// 限流」永远不触发。改用 (*Server).clientIPFrom —— 只在请求来自可信代理时
// 才采信转发头。留着一个可被误用的旧函数本身就是隐患，故直接删除。

// --- 可信代理（XFF 防伪造） ------------------------------------------------
//
// clientIP 原先无条件采信 X-Forwarded-For / X-Real-IP。服务默认监听 :8787
// （所有网卡），任何能直连的客户端都能伪造这两个头 —— 把每次请求的来源 IP
// 报成不同的值，就能让「按 IP 限流」永远不触发（每个桶只失败 1 次）。
// 登录限流是防暴力破解的主要手段，这条路绕过去等于没有限流。
//
// 正确做法：只有当请求**确实来自可信代理**时才采信转发头。直连时用
// RemoteAddr（连接的真实来源，伪造不了）。部署在 nginx/caddy 后面时，
// 把代理的地址填进 settings.trusted_proxies（CIDR 列表，支持逗号分隔）。
//
// 未配置 trusted_proxies 时：不采信任何转发头，一律用 RemoteAddr。
// 这会让「前置了反代但没配这个」的部署把限流变成「按反代 IP 计数」——
// 但反代通常只有一个 IP，IP 维度会先于账号维度触发，仍有防护，只是更严。

// trustedProxyNetworks 解析 settings.trusted_proxies 里的 CIDR 列表。
// 解析失败的项跳过（宁可少信一个代理，也不要因为一个写错的网段把
// 整个 header 信任打开）。
func (s *Server) trustedProxyNetworks() []*net.IPNet {
	if s == nil || s.m == nil || s.m.pg == nil {
		return nil
	}
	raw, ok, err := s.m.pg.GetSetting(settingTrustedProxies)
	if err != nil || !ok {
		return nil
	}
	return parseCIDRList(raw)
}

// parseCIDRList 解析逗号分隔的 CIDR / 单 IP 列表。解析失败的项跳过（宁可少信
// 一个代理，也不要因为一个写错的网段把整个 header 信任打开）。
func parseCIDRList(raw string) []*net.IPNet {
	var out []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(part); err == nil {
			out = append(out, n)
			continue
		}
		// 也允许直接写单个 IP。
		if ip := net.ParseIP(part); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		}
	}
	return out
}

// isTrustedProxy 判断请求的对端地址是否落在可信代理网段里。
func isTrustedProxy(remoteAddr string, nets []*net.IPNet) bool {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIPFrom 返回本次请求应归属的来源 IP，以及是否采信了转发头。
//
// 与 clientIP 的区别：clientIP 无条件信任转发头（可被伪造绕过限流），
// 这个版本只在请求来自可信代理时才采信。
func (s *Server) clientIPFrom(r *http.Request) string {
	return clientIPWithNets(r, s.trustedProxyNetworks())
}

// clientIPWithNets 是 clientIPFrom 的纯函数版本（便于测试）。
func clientIPWithNets(r *http.Request, nets []*net.IPNet) string {
	remote := r.RemoteAddr
	if !isTrustedProxy(remote, nets) {
		// 直连（或代理不可信）：用连接的真实来源，伪造不了。
		host := remote
		if h, _, err := net.SplitHostPort(remote); err == nil {
			host = h
		}
		return host
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return strings.TrimSpace(xr)
	}
	host := remote
	if h, _, err := net.SplitHostPort(remote); err == nil {
		host = h
	}
	return host
}

// trustedProxiesSetting 读取可信代理网段设置（读失败返回空串 —— 取不到配置
// 就退回到「不采信转发头」这个安全默认）。
func trustedProxiesSetting(pg *db.DB) string {
	if pg == nil {
		return ""
	}
	v, _, err := pg.GetSetting(settingTrustedProxies)
	if err != nil {
		return ""
	}
	return v
}
