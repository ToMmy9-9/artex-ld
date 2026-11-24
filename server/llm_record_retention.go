// llm_record_retention.go：LLM 调用记录的保留策略 —— 定时清理超期记录。
// 主要函数/类型：
//   - runLLMRecordRetention：后台循环，按保留天数定期清理 llm_records 里的旧记录。
//   - purgeLLMRecordsOlderThan：执行一次清理，返回删除行数。
package server

import (
	"context"
	"log"
	"time"
)

// llmRecordRetentionInterval 是清理循环的检查间隔。清理是低频动作 —— 一小时
// 一次足够（保留期最短也是 1 天），不必跟调度器的 5 秒 tick 挤在一起。
const llmRecordRetentionInterval = time.Hour

// runLLMRecordRetention loops until ctx is done, purging LLM call records that
// fell outside the configured retention window.
//
// llm_records 存的是每次 LLM 调用的请求/响应原文（生产库实测单行平均 190KB），
// 任务跑一天就能涨 400–700MB。录制的价值在于「排查刚才那次调用」，所以按天
// 清理超期记录：既保住近期调试价值，又不让表无限膨胀撑爆磁盘。
//
// 保留天数为 0 时不做任何清理（用户选择永久保留）。
func (s *Server) runLLMRecordRetention(ctx context.Context) {
	// 启动后先等一段时间再清第一次：刚启动时表里都是最近的记录，没必要扫。
	timer := time.NewTimer(llmRecordRetentionInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		n, err := s.purgeLLMRecordsOlderThan()
		if err != nil {
			log.Printf("[llm-retention] 清理失败: %v", err)
		} else if n > 0 {
			log.Printf("[llm-retention] 已清理 %d 条超期的 LLM 调用记录", n)
			// 删行不还磁盘：普通 VACUUM 只把空间标记为「可重用」，磁盘占用不变。
			// llm_records 是持续增长的大表（生产库实测 2.6GB），必须重写表才能把
			// 空间真正还给操作系统 —— 否则行越删越少、磁盘却一直满。
			// VACUUM FULL 会短暂锁表（重写 2.6GB 实测约 7 秒），清理是一小时
			// 一次的低频动作，这个锁窗可接受。
			if err := s.m.pg.VacuumLLMRecords(ctx); err != nil {
				log.Printf("[llm-retention] VACUUM FULL 失败（磁盘空间未回收）: %v", err)
			} else {
				log.Printf("[llm-retention] 已重写表，磁盘空间已回收")
			}
		}
		timer.Reset(llmRecordRetentionInterval)
	}
}

// purgeLLMRecordsOlderThan deletes records older than the retention window and
// returns how many rows were removed. Returns 0 when retention is disabled.
func (s *Server) purgeLLMRecordsOlderThan() (int64, error) {
	days := s.m.LLMRecordRetentionDays()
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	return s.m.pg.DeleteLLMRecordsOlderThan(cutoff)
}
