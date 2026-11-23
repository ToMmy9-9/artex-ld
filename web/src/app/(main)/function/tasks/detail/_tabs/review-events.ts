// 任务级意图审核开关的跨组件通知。
//
// 审核面板（review-panel）可以在任务运行中途把 review_enabled 从 false 打开，
// 但会话面板（sessions-tab）只在挂载时读一次状态来决定是否轮询待审计数。
// 开启后不通知的话，审核 tab 旁的蓝色待审计数徽标永远不会出现。
//
// 独立成模块是为了避免 review-panel ↔ sessions-tab 的循环 import。
export const REVIEW_ENABLED_CHANGED_EVENT = "artex:review-enabled-changed";

/** broadcastReviewEnabledChange 通知会话面板重新读取审核开关。 */
export function broadcastReviewEnabledChange(): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new Event(REVIEW_ENABLED_CHANGED_EVENT));
}
