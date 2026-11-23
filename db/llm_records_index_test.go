package db

import (
	"strings"
	"testing"
)

// TestLLMRecordsTaskIndexExists 钉住 llm_records 的 task_id 索引。
//
// 背景：llm_records 是全库最大的表（生产库实测 3.9GB / 2 万行，单行平均
// 190KB，因为存了 LLM 调用的请求响应原文）。没有 task_id 索引时：
//   - 「删任务」走的 DELETE 要全表扫，实测 12000 行时耗时 14.27 秒，
//     而且扫过的每一页都要写 WAL，把整个实例拖慢；
//   - 加上表达式索引后同一条查询 11.4ms → 0.42ms（快 27 倍）。
//
// 索引是 CREATE INDEX IF NOT EXISTS 建的，迁移代码在 llmRecordsSchema。
// 但如果有人重建库时漏了迁移、或手工 drop 了索引，性能会悄悄退化回
// 全表扫 —— 这条用例就是那个沉默退化的哨兵。
func TestLLMRecordsTaskIndexExists(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("测试库不可用: %v", err)
	}
	defer d.Close()

	rows, err := d.Query(`SELECT indexdef FROM pg_indexes WHERE tablename='llm_records'`)
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
		// 必须是表达式索引：查询写的是 COALESCE(task_id,'')=?，
		// 普通 task_id btree 索引用不上（COALESCE 会让索引失效）。
		if strings.Contains(def, "idx_llm_records_task") {
			if !strings.Contains(def, "COALESCE") {
				t.Fatalf("idx_llm_records_task 不是表达式索引，COALESCE(task_id,'')=? 用不上它: %s", def)
			}
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历索引: %v", err)
	}
	if !found {
		t.Fatal("llm_records 缺少 idx_llm_records_task 索引 —— 「删任务」会退化成全表扫（实测 14 秒）")
	}
}
