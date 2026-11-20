package db

import (
	"testing"
)

// TestAuditLogRecordAndList 验证：审计日志能写入、能按分类/动作/结果筛选、分页倒序读回。
// 审计是"事后追责"的唯一依据，写不进去或读不出来等于没有。
func TestAuditLogRecordAndList(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { d.Close() })

	// 只清本测试写入的行，避免动到别的用例的审计数据。
	const marker = "audit-selftest-marker"
	defer func() {
		_, _ = d.Exec(`DELETE FROM audit_logs WHERE message LIKE $1`, "%"+marker+"%")
	}()

	d.RecordAudit("ARTEX", "auth", "login", "success", "1.2.3.4", marker+" 登录成功", map[string]any{"k": "v"})
	d.RecordAudit("ARTEX", "auth", "login_failed", "fail", "5.6.7.8", marker+" 密码错误", nil)
	d.RecordAudit("ARTEX", "task", "delete", "success", "9.9.9.9", marker+" 删任务", nil)

	logs, total, err := d.ListAuditLogs("auth", "", "", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total < 2 {
		t.Fatalf("auth-category rows = %d, want >= 2", total)
	}
	for _, l := range logs {
		if l.Category != "auth" {
			t.Errorf("category filter leaked a %q row", l.Category)
		}
	}

	logs, total, err = d.ListAuditLogs("", "", "login_failed", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total < 1 {
		t.Fatalf("action-filtered rows = %d, want >= 1", total)
	}
	for _, l := range logs {
		if l.Action != "login_failed" {
			t.Errorf("action filter leaked a %q row", l.Action)
		}
	}

	logs, _, err = d.ListAuditLogs("", "", "", "fail", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range logs {
		if l.Result != "fail" {
			t.Errorf("result filter leaked a %q row", l.Result)
		}
	}

	// 分页：第一页应按 id 倒序（最新在前）。
	all, _, err := d.ListAuditLogs("", "", "", "", 500, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].ID < all[i].ID {
			t.Fatalf("rows are not newest-first: %d before %d", all[i-1].ID, all[i].ID)
		}
	}
}
