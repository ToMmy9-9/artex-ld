/**
 * task-detail-tab.ts：任务详情页「默认打开哪一页」的判断。
 *
 * 之所以单独抽出来：这段判断决定了用户点进任务第一眼看到什么，很容易在后续改动里被
 * 悄悄改错（比如误读一个接口并不返回的字段）。抽成纯函数后能直接测。
 *
 * 背景：详情页原本固定落在「会话」—— 原始活动流水。这是最不该默认打开的一页。
 * 任务跑完了，用户想看的是查出什么；任务在跑，想看的才是它现在在干什么。
 */

const TERMINAL_STATUSES = new Set(["done", "failed", "timeout"]);

export interface TabDecisionTask {
  status?: string;
}

/**
 * 决定默认 tab。
 *
 * 只做最小推断，不猜：
 *   - 有发现 → 发现页（用户最终要的结果）
 *   - 无发现且已结束 → 总览（结论与目标达成情况）
 *   - 无发现且还在跑 → 会话（实时动静）
 *
 * @param task        任务对象；为空（未加载）时保守回会话
 * @param findingTotal 该任务的发现总数。调用方需自行查询 —— 注意不要读
 *                     task.distinct_findings，那是列表接口才填的字段，
 *                     单任务详情接口不返回它。
 */
export function defaultTabFor(task: TabDecisionTask | null, findingTotal: number): string {
  if (!task) return "sessions";
  if (findingTotal > 0) return "findings";
  if (TERMINAL_STATUSES.has(task.status ?? "")) return "overview";
  return "sessions";
}
