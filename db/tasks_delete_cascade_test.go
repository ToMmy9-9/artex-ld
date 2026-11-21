package db

import (
	"strconv"
	"testing"
)

// 任务删除必须连带清掉它的拦截记录：intercept_pending.task_id 是 text 且没有
// 指向 tasks 的外键，不显式删就会全部变成孤儿——仍出现在「审批记录」页、
// 状态停在 pending、点进去是已不存在的任务，既不能处置也不会收敛。
// 实测线上 1659 行里 1393 行是这种孤儿。
func TestDeleteTaskCascadeRemovesInterceptPending(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer d.Close()

	task, err := d.CreateTask("intercept cascade", "cascade goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	taskID := strconv.FormatInt(task.ID, 10)
	if _, err := d.CreateInterceptPending(0, 0, taskID, "work#1", "Bash", []byte(`{"command":"ls"}`), "[模型] 只读探测"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateDecidedIntercept(0, 0, taskID, "work#1", "Bash", []byte(`{"command":"pwd"}`), "allowed", "[模型] 只读"); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := d.QueryRow(`SELECT count(*) FROM intercept_pending WHERE task_id=$1`, taskID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 2 {
		t.Fatalf("前置数据不足，只有 %d 条拦截记录", before)
	}

	if _, err := d.DeleteTaskCascade(task.ID, false, false); err != nil {
		t.Fatal(err)
	}

	var after int
	if err := d.QueryRow(`SELECT count(*) FROM intercept_pending WHERE task_id=$1`, taskID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != 0 {
		t.Fatalf("任务删除后仍有 %d 条拦截记录残留（孤儿数据）", after)
	}
}
