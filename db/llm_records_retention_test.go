package db

import (
	"testing"
	"time"
)

// TestDeleteLLMRecordsOlderThan 验证保留策略只删超期记录，且 cutoff 之前/之后的
// 边界正确。
//
// llm_records 存的是每次 LLM 调用的请求/响应原文（生产库实测单行平均 190KB），
// 任务跑一天涨 400–700MB。保留策略按天清理超期记录，是让这张表不无限膨胀的
// 唯一办法 —— 所以「删哪些、留哪些」的边界必须准。
func TestDeleteLLMRecordsOlderThan(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("测试库不可用: %v", err)
	}
	defer d.Close()

	// 造三条：老的（2 天前）、中间的（1 天前）、新的（现在）
	old := time.Now().Add(-48 * time.Hour)
	mid := time.Now().Add(-24 * time.Hour)
	now := time.Now()
	ids := []int64{}
	for _, ts := range []time.Time{old, mid, now} {
		var id int64
		if err := d.QueryRow(
			`INSERT INTO llm_records(ts, model, status) VALUES ($1,'retention-test','ok') RETURNING id`,
			ts,
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

	// cutoff = 36 小时前 → 我插的那条 48 小时前的记录必须被删，
	// 而 24 小时前与刚插入的两条必须留下。
	//
	// 这里刻意**不断言删除的绝对行数**：测试库是所有测试共用的，别的测试
	// 留下的历史记录也会被一起删掉（实测删除数会在 1 与 2 之间浮动，导致
	// 断言间歇性失败）。只验证「我插的那几条的存活情况」，与库里其它数据无关。
	cutoff := time.Now().Add(-36 * time.Hour)
	if _, err := d.DeleteLLMRecordsOlderThan(cutoff); err != nil {
		t.Fatalf("DeleteLLMRecordsOlderThan: %v", err)
	}

	exists := func(id int64) bool {
		t.Helper()
		var ok bool
		if err := d.QueryRow(`SELECT EXISTS(SELECT 1 FROM llm_records WHERE id=$1)`, id).Scan(&ok); err != nil {
			t.Fatalf("检查记录 %d: %v", id, err)
		}
		return ok
	}

	// 48 小时前那条 —— 超期，必须被删
	if exists(ids[0]) {
		t.Fatal("48 小时前的记录超期了却没被删掉")
	}
	// 24 小时前与刚插入的 —— 未超期，必须保留
	for i, id := range ids[1:] {
		if !exists(id) {
			t.Fatalf("记录 %d（索引 %d）不该被删除", id, i)
		}
	}
}

// TestDeleteLLMRecordsOlderThanZeroCutoff 验证零值 cutoff 是「不删任何东西」
// 而不是「删掉全部」—— 传零值时的语义必须安全。
func TestDeleteLLMRecordsOlderThanZeroCutoff(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("测试库不可用: %v", err)
	}
	defer d.Close()

	// 插一条 ts 很老（必定超期）的记录：零值 cutoff 也不该删它。
	// 零值 cutoff 的语义是「时间零之前」，没有任何记录的 ts 早于它 ——
	// 如果实现把零值当成「无限早」，这条就会被删掉，测试能抓到。
	old := time.Now().Add(-365 * 24 * time.Hour)
	var id int64
	if err := d.QueryRow(
		`INSERT INTO llm_records(ts, model, status) VALUES ($1,'retention-zero','ok') RETURNING id`,
		old,
	).Scan(&id); err != nil {
		t.Fatalf("插入测试记录: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec(`DELETE FROM llm_records WHERE id=$1`, id)
	})

	n, err := d.DeleteLLMRecordsOlderThan(time.Time{})
	if err != nil {
		t.Fatalf("DeleteLLMRecordsOlderThan(zero): %v", err)
	}
	if n != 0 {
		t.Fatalf("零值 cutoff 删了 %d 行, want 0", n)
	}

	var exists bool
	if err := d.QueryRow(`SELECT EXISTS(SELECT 1 FROM llm_records WHERE id=$1)`, id).Scan(&exists); err != nil {
		t.Fatalf("检查记录: %v", err)
	}
	if !exists {
		t.Fatal("零值 cutoff 把超期记录删掉了 —— 零值必须表示「什么都不删」")
	}
}

// TestCountLLMRecordsAndSize 验证统计接口可用（前端要展示「多少条 · 占多大」）。
func TestCountLLMRecordsAndSize(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("测试库不可用: %v", err)
	}
	defer d.Close()

	n, err := d.CountLLMRecords()
	if err != nil {
		t.Fatalf("CountLLMRecords: %v", err)
	}
	if n < 0 {
		t.Fatalf("count=%d, 不该为负", n)
	}
	size, err := d.LLMRecordsSizeBytes()
	if err != nil {
		t.Fatalf("LLMRecordsSizeBytes: %v", err)
	}
	if size <= 0 {
		t.Fatalf("size=%d, 表不可能没有体积", size)
	}
}
