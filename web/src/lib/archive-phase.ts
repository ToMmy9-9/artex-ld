// archive-phase.ts：把归档记录里的内部阶段标记翻成中文。
//
// 为什么需要：归档列表在状态徽章下面直接渲染 `{archive.phase} · {archive.progress}%`，
// 而 phase 是后端流程内部的英文标记（stage_files / snapshot_database / compact_traffic…），
// 于是中文界面上出现 "snapshot_database · 20%" 这种半截英文。
//
// 取值来源：server/task_archives.go 里所有 UpdateTaskArchiveProgress 的 phase 参数，
// 加上 db 侧直接写入的 queued / starting / failed / interrupted / ready / database_restored。
// 后端新增阶段时这里要同步补一条 —— 未收录的标记会退化成「处理中」，
// 宁可用一句泛化中文，也不把内部标记暴露给用户。

const ARCHIVE_PHASE_LABELS: Record<string, string> = {
  // 队列与终态
  queued: "排队中",
  starting: "准备中",
  ready: "已完成",
  failed: "已失败",
  interrupted: "上次中断",
  // 归档流程
  stage_files: "整理任务文件",
  snapshot_database: "导出数据快照",
  snapshot_traffic: "导出流量记录",
  compress: "打包压缩",
  compact_traffic: "清理流量热数据",
  compact_database: "清理已归档数据",
  // 还原流程
  verify_package: "校验归档包",
  restore_traffic: "还原流量记录",
  restore_files: "还原任务文件",
  restore_database: "还原数据快照",
  database_restored: "数据快照已还原",
  consume_package: "启用还原结果",
  // 删除流程
  stage_package_delete: "摘除归档包",
  delete_metadata: "清理归档记录",
};

export function archivePhaseLabel(phase: string | null | undefined): string {
  const key = (phase ?? "").trim();
  if (!key) return "处理中";
  return ARCHIVE_PHASE_LABELS[key] ?? "处理中";
}
