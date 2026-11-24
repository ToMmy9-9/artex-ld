package db

import (
	"context"
	"testing"
	"time"
)

// TestVacuumLLMRecordsReclaimsSpace 验证 VACUUM FULL 真的把磁盘空间还给操作系统。
//
// 为什么这条重要：普通 VACUUM 只把删掉的行标记成「空间可重用」，**磁盘占用
// 不变**。llm_records 是持续增长的大表（生产库实测 2.6GB / 9300 行，单行平均
// 190KB），保留策略每天删掉几千行后，磁盘必须靠重写表才能真正回收 —— 否则
// 行越删越少、磁盘却一直满。
//
// 用例做法：插一批行 → 删掉它们 → 确认表体积没降（普通 DELETE 就是这样）
// → VACUUM → 确认体积降下来了。
func TestVacuumLLMRecordsReclaimsSpace(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("测试库不可用: %v", err)
	}
	defer d.Close()
	ctx := context.Background()

	// 插一批足够大的行，让体积变化看得见（单行 ~200KB）。
	const n = 40
	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		body := make([]byte, 200<<10) // 200KB
		for j := range body {
			body[j] = byte('a' + (i+j)%26)
		}
		var id int64
		if err := d.QueryRow(
			`INSERT INTO llm_records(ts, model, status, raw_request) VALUES ($1,'vacuum-test','ok',$2) RETURNING id`,
			time.Now(), string(body),
		).Scan(&id); err != nil {
			t.Fatalf("插入测试记录: %v", err)
		}
		ids = append(ids, id)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = d.Exec(`DELETE FROM llm_records WHERE id=$1`, id)
		}
	})

	before, err := d.LLMRecordsSizeBytes()
	if err != nil {
		t.Fatalf("LLMRecordsSizeBytes(before): %v", err)
	}

	// 删掉刚插的（模拟保留策略清理）。
	for _, id := range ids {
		if _, err := d.Exec(`DELETE FROM llm_records WHERE id=$1`, id); err != nil {
			t.Fatalf("删除: %v", err)
		}
	}
	deleted, err := d.CountLLMRecords()
	if err != nil {
		t.Fatalf("CountLLMRecords: %v", err)
	}

	if err := d.VacuumLLMRecords(ctx); err != nil {
		t.Fatalf("VacuumLLMRecords: %v", err)
	}
	after, err := d.LLMRecordsSizeBytes()
	if err != nil {
		t.Fatalf("LLMRecordsSizeBytes(after): %v", err)
	}
	t.Logf("行数=%d 体积 %d → %d 字节", deleted, before, after)
	if after >= before {
		t.Fatalf("VACUUM FULL 后体积没降（before=%d after=%d）—— 磁盘空间没回收", before, after)
	}
}
