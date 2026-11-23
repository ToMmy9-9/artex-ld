package db

import (
	"testing"
)

// seedScheduleForBatchTest 在测试库造一条计划任务，让批量 vs 逐个的对比
// 真的跑到有数据的路径。测试结束会删掉。
func seedScheduleForBatchTest(t *testing.T, d *DB) {
	t.Helper()
	created, err := d.CreateTaskSchedule(TaskScheduleInput{
		Name:         "batch-test-schedule",
		Enabled:      false,
		TimezoneMode: "beijing",
		ScheduleType: "once",
		RunDate:      "2030-01-01",
		StartTime:    "03:00",
		EndTime:      "04:00",
	})
	if err != nil {
		t.Skipf("无法创建测试计划任务: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec(`DELETE FROM task_schedule_tasks WHERE schedule_id=$1`, created.ID)
		_, _ = d.Exec(`DELETE FROM task_schedule_history WHERE schedule_id=$1`, created.ID)
		_, _ = d.Exec(`DELETE FROM task_schedules WHERE id=$1`, created.ID)
	})
}

// TestScheduleBatchQueriesMatchPerSchedule 验证批量查询与逐个查询结果一致。
//
// 背景：计划任务列表页原来是 N+1 —— ListTaskSchedules 对每个计划查一次关联任务，
// scheduleDTO 又对每个计划查一次运行统计。N 个计划 = 1+2N 次往返（N=50 时
// 101 次）。改成批量查询后必须保证结果与逐个查完全一致，否则列表会显示错的
// 关联任务或错的运行次数。
func TestScheduleBatchQueriesMatchPerSchedule(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("测试库不可用: %v", err)
	}
	defer d.Close()

	items, err := d.ListTaskSchedules()
	if err != nil {
		t.Fatalf("ListTaskSchedules: %v", err)
	}
	if len(items) == 0 {
		// 测试库可能没有计划任务 —— 造一条，这样批量 vs 逐个的对比才真的跑到。
		seedScheduleForBatchTest(t, d)
		items, err = d.ListTaskSchedules()
		if err != nil {
			t.Fatalf("ListTaskSchedules(after seed): %v", err)
		}
		if len(items) == 0 {
			t.Skip("无法在测试库创建计划任务，跳过")
		}
	}

	// 批量关联任务 vs 列表里已填好的 TaskIDs
	batch, err := d.ScheduleTaskIDsBatch()
	if err != nil {
		t.Fatalf("ScheduleTaskIDsBatch: %v", err)
	}
	for _, item := range items {
		want := batch[item.ID]
		got := item.TaskIDs
		if len(got) != len(want) {
			t.Fatalf("schedule %d: 关联任务数 批量=%d 逐个=%d", item.ID, len(want), len(got))
		}
		for i := range want {
			if want[i] != got[i] {
				t.Fatalf("schedule %d: 关联任务[%d] 批量=%d 逐个=%d", item.ID, i, want[i], got[i])
			}
		}
	}

	// 批量运行统计 vs 逐个查询
	statsBatch, err := d.ScheduleRunStatsBatch()
	if err != nil {
		t.Fatalf("ScheduleRunStatsBatch: %v", err)
	}
	for _, item := range items {
		single, err := d.ScheduleRunStats(item.ID)
		if err != nil {
			t.Fatalf("ScheduleRunStats(%d): %v", item.ID, err)
		}
		batched, ok := statsBatch[item.ID]
		if !ok {
			// 没有历史记录的计划不在批量结果里 —— 批量查询用 GROUP BY，
			// 无记录的 schedule 不会出现在结果集中，这是预期差异。
			if single.RunCount != 0 {
				t.Fatalf("schedule %d: 批量结果缺失但逐个查到 run_count=%d", item.ID, single.RunCount)
			}
			continue
		}
		if batched.RunCount != single.RunCount {
			t.Fatalf("schedule %d: run_count 批量=%d 逐个=%d", item.ID, batched.RunCount, single.RunCount)
		}
	}
}
