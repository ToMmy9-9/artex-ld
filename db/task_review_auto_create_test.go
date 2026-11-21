package db

// ═══ [同类 bug 排查 · spawn_task 继承自动处置] ═══
// spawn_task 派生内网子任务时只继承 review_enabled，TaskCreateOptions 根本没有
// ReviewAuto 字段、INSERT 也不写 review_auto 列。后果：父任务开着自动处置，
// 派生出的子任务却永远 review_auto=false，意图全堆到人工面板 —— 与
// 「开了自动处置却不生效」是同一个故障，只是发生在子任务上。
// 二开日期：2026-11-21

import "testing"

// TestCreateTaskWithOptionsPersistsReviewAuto 建任务时 ReviewAuto 必须落库。
func TestCreateTaskWithOptionsPersistsReviewAuto(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { d.Close() })

	task, err := d.CreateTaskWithOptions("review auto persist", "goal", TaskCreateOptions{
		ReviewEnabled: true,
		ReviewAuto:    true,
	})
	if err != nil {
		t.Fatalf("CreateTaskWithOptions: %v", err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM tasks WHERE id=$1`, task.ID) })

	var enabled, auto bool
	if err := d.QueryRow(`SELECT review_enabled, review_auto FROM tasks WHERE id=$1`, task.ID).Scan(&enabled, &auto); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !enabled {
		t.Fatal("review_enabled 未落库")
	}
	if !auto {
		t.Fatal("review_auto 未落库：spawn_task 派生的子任务会因此永远不开自动处置，" +
			"父任务开着自动、子任务意图却全堆人工面板")
	}
	if !task.ReviewAuto {
		t.Fatal("返回的 Task 结构体里 ReviewAuto 也没带上，调用方会读到默认 false")
	}
}

// TestCreateTaskWithOptionsDefaultsAutoOff 不传时默认关，不能因新增字段而改变旧行为。
func TestCreateTaskWithOptionsDefaultsAutoOff(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { d.Close() })

	task, err := d.CreateTaskWithOptions("review auto default", "goal", TaskCreateOptions{
		ReviewEnabled: true, // 只开审核，不开自动
	})
	if err != nil {
		t.Fatalf("CreateTaskWithOptions: %v", err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM tasks WHERE id=$1`, task.ID) })

	var auto bool
	if err := d.QueryRow(`SELECT review_auto FROM tasks WHERE id=$1`, task.ID).Scan(&auto); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if auto {
		t.Fatal("未传 ReviewAuto 时不应默认开启自动处置")
	}
}
