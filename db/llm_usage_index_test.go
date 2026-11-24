package db

import (
	"strings"
	"testing"
)

// TestLLMUsageTaskExprIndexExists 钉住 llm_usage 的表达式索引。
//
// 背景：llm_usage 的归档/统计查询（taskArchiveAggregates）写的是
// COALESCE(task_id,'')=? OR exploration_id=?。裸的 task_id B-tree 索引用不上
// —— 表达式包住列会让索引失效，Postgres 只能全表扫。
//
// 实测（生产库）：pg_stat_user_tables 里 llm_usage 的 seq_scan=4499、
// seq_tup_read=7019 万，而 idx_scan 只有 96 —— 基本全靠全表扫。
//
// llm_records 早就有对应的表达式索引（idx_llm_records_task），llm_usage
// 之前漏了。这条用例就是那个不对称不再复发的哨兵。
func TestLLMUsageTaskExprIndexExists(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("测试库不可用: %v", err)
	}
	defer d.Close()

	// 先跑一次建表/建索引（生产由 server 启动时的 EnsureLLMUsageTable 负责）。
	// 这样用例不依赖「测试库之前是否被迁移过」。
	if err := d.EnsureLLMUsageTable(); err != nil {
		t.Fatalf("EnsureLLMUsageTable: %v", err)
	}

	rows, err := d.Query(`SELECT indexdef FROM pg_indexes WHERE tablename='llm_usage'`)
	if err != nil {
		t.Fatalf("查询索引: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var def string
		if err := rows.Scan(&def); err != nil {
			t.Fatalf("扫描索引: %v", err)
		}
		// 必须是表达式索引：COALESCE(task_id,'')=? 用不上裸 task_id 索引。
		if strings.Contains(def, "idx_llm_usage_task_expr") {
			if !strings.Contains(def, "COALESCE") {
				t.Fatalf("idx_llm_usage_task_expr 不是表达式索引，COALESCE(task_id,'')=? 用不上它: %s", def)
			}
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历索引: %v", err)
	}
	if !found {
		t.Fatal("llm_usage 缺少 idx_llm_usage_task_expr 表达式索引 —— 归档统计会退化成全表扫")
	}
}
