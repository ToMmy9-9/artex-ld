package db

// conversation_activities 写入的死锁重试契约。
//
// 真实故障现场（后端日志）：
//
//	[conv 1822] append activity failed: 错误: 检测到死锁 (SQLSTATE 40P01)
//
// 成因：conversation_activities.conversation_id 有指向 conversations(id) 的外键，
// INSERT 时必须对该行取 ROW SHARE 锁；而 TouchConversation / UpdateConversation
// 对同一行取排他锁。两个会话交叉写入对方的行时，PG 判定死锁并回滚其中一方。
//
// 死锁是 PG 主动回滚的结果，被回滚的语句重试一般就能成功。修复前只打一行日志
// 就把这条活动丢掉——对话里凭空少一步，事后无法追溯，属于静默数据丢失。

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func deadlockErr() error {
	return &pgconn.PgError{Code: "40P01", Message: "deadlock detected"}
}

// TestExecRetryDeadlockRetriesUntilSuccess 死锁后重试，最终成功且返回 nil。
func TestExecRetryDeadlockRetriesUntilSuccess(t *testing.T) {
	attempts := 0
	err := execRetryDeadlock(func() error {
		attempts++
		if attempts < 3 {
			return deadlockErr()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("重试后应成功：%v", err)
	}
	if attempts != 3 {
		t.Fatalf("尝试次数 = %d，期望 3（两次死锁 + 一次成功）", attempts)
	}
}

// TestExecRetryDeadlockGivesUpAfterLimit 一直死锁时必须放弃并把错误交回调用方，
// 不能无限重试把请求挂死。
func TestExecRetryDeadlockGivesUpAfterLimit(t *testing.T) {
	attempts := 0
	err := execRetryDeadlock(func() error {
		attempts++
		return deadlockErr()
	})
	if err == nil {
		t.Fatal("持续死锁应最终返回错误，而不是假装成功")
	}
	if !isPostgresDeadlock(err) {
		t.Fatalf("应返回原始死锁错误：%v", err)
	}
	want := len(retryDelays) + 1
	if attempts != want {
		t.Fatalf("尝试次数 = %d，期望 %d（首次 + %d 次重试）", attempts, want, len(retryDelays))
	}
}

// TestExecRetryDeadlockDoesNotRetryOtherErrors 非死锁错误（约束冲突、连接断开、
// 业务错误）必须立刻返回，不能被当成死锁无谓重试——否则唯一约束冲突会变成
// "重试几次后报一个迟到的错"，还会把写操作重复执行。
func TestExecRetryDeadlockDoesNotRetryOtherErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"唯一约束冲突", &pgconn.PgError{Code: "23505", Message: "duplicate key"}},
		{"外键违约", &pgconn.PgError{Code: "23503", Message: "fk violation"}},
		{"普通错误", errors.New("connection reset")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			attempts := 0
			err := execRetryDeadlock(func() error {
				attempts++
				return c.err
			})
			if attempts != 1 {
				t.Fatalf("%s 不应重试，尝试次数 = %d", c.name, attempts)
			}
			if !errors.Is(err, c.err) {
				t.Fatalf("应原样返回错误 %v，实际 %v", c.err, err)
			}
		})
	}
}
