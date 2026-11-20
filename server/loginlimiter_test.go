package server

import (
	"testing"
	"time"
)

// TestLoginLimiterLocksAfterThreshold 验证：连续失败达阈值后该（IP,账号）被锁定，
// 锁定期间即便密码正确也拒绝 —— 暴力破解防护的第一道闸。
func TestLoginLimiterLocksAfterThreshold(t *testing.T) {
	l := newLoginLimiter()
	pol := loginPolicy{enabled: true, userThreshold: 3, ipThreshold: 10, window: time.Minute, cooldown: time.Minute}

	for i := 0; i < 2; i++ {
		if l.fail("1.2.3.4", "ARTEX", pol) {
			t.Fatalf("failure %d should not lock yet", i+1)
		}
		if locked, _ := l.locked("1.2.3.4", "ARTEX"); locked {
			t.Fatalf("locked too early after %d failures", i+1)
		}
	}
	if !l.fail("1.2.3.4", "ARTEX", pol) {
		t.Fatal("the failure reaching the threshold should report a new lock")
	}
	locked, remain := l.locked("1.2.3.4", "ARTEX")
	if !locked {
		t.Fatal("the account/IP pair should be locked after the threshold")
	}
	if remain <= 0 || remain > pol.cooldown {
		t.Errorf("remaining cooldown = %v, want (0, %v]", remain, pol.cooldown)
	}
	// 别的来源 + 别的账号不受影响（不能因为一个人被锁就把所有人锁了）。
	// 注意：同一账号从别的 IP 来**也会**被挡 —— 账号维度就是全局生效的，否则
	// 攻击者换 IP 即可绕过账号锁定。
	if locked, _ := l.locked("5.6.7.8", "OTHER"); locked {
		t.Error("an unrelated IP/account pair must not be locked")
	}
	if locked, _ := l.locked("5.6.7.8", "ARTEX"); !locked {
		t.Error("the account dimension must lock regardless of the source IP")
	}
}

// TestLoginLimiterResetsOnSuccess 验证：成功登录清零该（IP,账号）的失败计数 ——
// 合法用户偶尔打错一次不该被后续累计锁定。
func TestLoginLimiterResetsOnSuccess(t *testing.T) {
	l := newLoginLimiter()
	pol := loginPolicy{enabled: true, userThreshold: 3, ipThreshold: 10, window: time.Minute, cooldown: time.Minute}

	l.fail("1.2.3.4", "ARTEX", pol)
	l.fail("1.2.3.4", "ARTEX", pol)
	l.reset("1.2.3.4", "ARTEX")
	l.fail("1.2.3.4", "ARTEX", pol)
	if locked, _ := l.locked("1.2.3.4", "ARTEX"); locked {
		t.Error("the counter should have been reset by a successful login")
	}
}

// TestLoginLimiterDisabledPolicy 验证：策略关闭时不记状态、永不锁定。
func TestLoginLimiterDisabledPolicy(t *testing.T) {
	l := newLoginLimiter()
	pol := loginPolicy{enabled: false, userThreshold: 1, ipThreshold: 1, window: time.Minute, cooldown: time.Minute}
	for i := 0; i < 5; i++ {
		if l.fail("1.2.3.4", "ARTEX", pol) {
			t.Fatal("a disabled policy must never lock")
		}
	}
	if locked, _ := l.locked("1.2.3.4", "ARTEX"); locked {
		t.Error("a disabled policy must never report a lock")
	}
}

// TestLoginLimiterIPThresholdIndependent 验证：账号维度与 IP 维度各算各的 ——
// 单一来源横扫多个账号会被 IP 桶挡下。
func TestLoginLimiterIPThresholdIndependent(t *testing.T) {
	l := newLoginLimiter()
	pol := loginPolicy{enabled: true, userThreshold: 100, ipThreshold: 3, window: time.Minute, cooldown: time.Minute}

	// 三个不同账号各失败一次 → IP 桶达阈值 → 锁定该 IP。
	l.fail("1.2.3.4", "alice", pol)
	l.fail("1.2.3.4", "bob", pol)
	l.fail("1.2.3.4", "carol", pol)
	if locked, _ := l.locked("1.2.3.4", "dave"); !locked {
		t.Error("the IP bucket should lock after reaching its own threshold")
	}
}
