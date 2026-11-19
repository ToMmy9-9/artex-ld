// misc 域：工具执行历史（commands）、LLM 调用记录与按模型 token 聚合、一键更新。
/**
 * 主要函数/类型：
 *   - miscApi / MiscApi：杂项域 API 聚合对象及其类型
 *   - commands / commandStats：工具执行历史分页列表与各工具调用次数
 *   - llmRecords/llmRecordDetail/llmTasks/llmRecordsDeleteTask/tokensByModel：LLM 调用记录与按模型 token 聚合
 *   - checkUpdate / applyUpdate / rollbackUpdate：一键更新的检查、应用与回滚
 */
import { arr, del, get, http, post, put } from "@/lib/api/http";

function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("artex_token");
}
import type {
  CommandRecord,
  LLMRecordDetail,
  LLMRecordItem,
  LLMTask,
  ModelTokenStat,
  ToolStat,
  UpdateCheck,
  EgressReport,
  ProxyPoolEntry,
  ProxySwitchLog,
  UpdateSettings,
  DailyTokenBucket,
  SystemMetrics,
  ReportSettings,
  ReportRecord,
  ReportTemplate,
  KnowledgeDoc,
  KnowledgeHit,
  KnowledgeLog,
  KnowledgeConfig, } from "@/lib/types";


// Empty Go slices can arrive as null from older servers. Keep proxy-pool
// consumers safe across initial load, import, and health-check refreshes.
function normalizeProxyPool<T extends { entries: ProxyPoolEntry[]; switch_log: ProxySwitchLog[] }>(pool: T): T {
  return { ...pool, entries: arr(pool.entries), switch_log: arr(pool.switch_log) };
}

export const miscApi = {
  // ---- 交付级报告：生成 / 状态 / 下载 / 出具方 Logo 上传 ----
  reportGenerate: (
    task: string,
    input?: {
      title?: string;
      client?: string;
      locale?: string;
      format?: string;
      findingIds?: string[];
      templateId?: number;
    },
  ) => {
    const sp = new URLSearchParams();
    if (task) sp.set("task", task);
    if (input?.title) sp.set("title", input.title);
    if (input?.client) sp.set("client_name", input.client);
    if (input?.locale) sp.set("locale", input.locale);
    if (input?.format) sp.set("format", input.format);
    if (input?.templateId) sp.set("template_id", String(input.templateId));
    if (input?.findingIds?.length) sp.set("finding_ids", input.findingIds.join(","));
    return post<{ status: string; job_id?: string }>(`/report/generate?${sp.toString()}`, {});
  },
  reportStatus: (task?: string) =>
    get<{ status: string; error?: string; filename?: string }>(
      `/report/status${task ? `?task=${encodeURIComponent(task)}` : ""}`,
    ),
  reportDownloadUrl: (task?: string) => `/api/report/download${task ? `?task=${encodeURIComponent(task)}` : ""}`,
  uploadReportLogo: async (file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    const token = getToken();
    const r = await fetch("/api/report-assets/logo", {
      method: "POST",
      body: fd,
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    });
    const body = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error((body as { error?: string }).error || `上传失败(${r.status})`);
    return body as { logo_path: string; url: string };
  },

  // ---- commands (tool execution history, any tool) ----
  commands: (params?: { task?: string; q?: string; page?: number; size?: number }) => {
    const sp = new URLSearchParams();
    if (params?.task) sp.set("task", params.task);
    if (params?.q) sp.set("q", params.q);
    sp.set("page", String(params?.page ?? 0));
    sp.set("size", String(params?.size ?? 50));
    return get<{ commands: CommandRecord[]; total: number }>(`/commands?${sp}`);
  },
  // 各工具调用次数；沿用列表的 task/q 筛选，统计的是整个结果集而非当前页。
  commandStats: (params?: { task?: string; q?: string }) => {
    const sp = new URLSearchParams();
    if (params?.task) sp.set("task", params.task);
    if (params?.q) sp.set("q", params.q);
    return get<{ stats: ToolStat[] }>(`/commands/stats?${sp}`);
  },

  // ---- LLM records ----
  llmRecords: (params?: { model?: string; session?: string; task?: string; page?: number; size?: number }) => {
    const sp = new URLSearchParams();
    if (params?.model) sp.set("model", params.model);
    if (params?.session) sp.set("session", params.session);
    if (params?.task) sp.set("task", params.task);
    if (params?.page !== undefined) sp.set("page", String(params.page));
    sp.set("size", String(params?.size ?? 50));
    return get<{ records: LLMRecordItem[]; total: number }>(`/llm/records?${sp}`);
  },
  llmRecordDetail: (id: number) => get<LLMRecordDetail>(`/llm/records/${id}`),
  llmTasks: () => get<{ tasks: LLMTask[] }>(`/llm/records/tasks`),
  llmRecordsDeleteTask: (task: string) => del<{ deleted: number }>(`/llm/records?task=${encodeURIComponent(task)}`),
  // 按模型聚合本任务的 token 用量（来自常开的 llm_usage 计量账本，逐次调用精确，
  // per-agent 绑定 / 轮询 / 中断消耗都覆盖）。
  tokensByModel: (task: string) =>
    get<{ models: ModelTokenStat[] }>(`/llm/records/by-model?task=${encodeURIComponent(task)}`),

  // ---- 一键更新 ----
  // 检查以后端为准：下载是后端做的，浏览器能连 GitHub 而服务器连不上的情况很常见
  // （服务器在内网、代理只配在浏览器上），那时点更新必然失败。
  // 后端对 GitHub 的查询结果有 30 分钟缓存（未认证的 GitHub API 是 60 次/小时/IP，
  // 顶栏每次整页加载都会查一次，不缓存会很快耗光配额）。force=true 强制回源，
  // 留给用户显式点「检查更新」时用。
  checkUpdate: (force = false) => get<UpdateCheck>(`/update/check${force ? "?force=1" : ""}`),
  // 202 即返回，实际下载在后台跑，进度走 /api/update/stream。
  applyUpdate: () => post<{ ok: boolean; target: string }>(`/update/apply`),
  rollbackUpdate: () => post<{ ok: boolean }>(`/update/rollback`),

  // ---- 合并自 RuoJi6/ARTEX v0.3.27 ----
  activateProxyPool: (id: string) =>
    post<{ ok: boolean; active_id: string; active_proxy: string }>(
      `/settings/proxy-pool/${encodeURIComponent(id)}/activate`,
      {},
    ),
  deleteProxyPool: (id: string) => del<{ ok: boolean }>(`/settings/proxy-pool/${encodeURIComponent(id)}`),
  testProxyPoolEntry: (id: string) =>
    post<{ ok: boolean; result?: EgressReport["checks"][number]["result"] }>(
      `/settings/proxy-pool/${encodeURIComponent(id)}/test`,
      {},
    ),
  importProxyPool: (payload: Record<string, unknown>) =>
    post<{
      entries: ProxyPoolEntry[];
      active_id: string;
      switch_log: ProxySwitchLog[];
      errors: Array<{ line: number; input: string; error: string }>;
    }>(`/settings/proxy-pool/import`, payload).then((r) => ({ ...normalizeProxyPool(r), errors: arr(r.errors) })),
  proxyPool: () =>
    get<{ entries: ProxyPoolEntry[]; active_id: string; active_proxy: string; switch_log: ProxySwitchLog[] }>(
      `/settings/proxy-pool`,
    ).then(normalizeProxyPool),
  testAllProxyPool: () =>
    post<{ entries: ProxyPoolEntry[]; active_id: string; switch_log: ProxySwitchLog[] }>(
      `/settings/proxy-pool/test-all`,
      {},
    ).then(normalizeProxyPool),
  testEgressProxy: (patch: { global_proxy?: string; probe_url?: string; resume?: boolean }) =>
    post<EgressReport>(`/settings/proxy/test`, patch),
  testTaskProxy: (taskID: string) => post<{ ok: boolean; error?: string }>(`/tasks/${taskID}/proxy/test`, {}),
  updateProxyPool: (id: string, payload: Partial<Pick<ProxyPoolEntry, "url" | "name" | "remark" | "enabled">>) =>
    put<{ entry: ProxyPoolEntry }>(`/settings/proxy-pool/${encodeURIComponent(id)}`, payload),
  getUpdateSettings: () => get<UpdateSettings>("/update/settings"),
  saveUpdateSettings: (settings: UpdateSettings) => put<UpdateSettings>("/update/settings", settings),
  tokenDaily: (days = 30) => get<DailyTokenBucket[]>(`/tokens/daily?days=${days}`).then(arr),
  // conversationTokens 同名实现放在 tasksApi（键序锚定只能有一份），此处不再重复定义。
  systemMetrics: () => get<SystemMetrics>("/system/metrics"),
  retryCredentials: (taskId: number, intelligenceId: number) =>
    post<{ queued: boolean }>(`/tasks/${taskId}/intelligence/${intelligenceId}/credentials/retry`),

  // ---- 合并自 RuoJi6/ARTEX v0.3.27 ----
  gc: (ttl = 86400) => post<{ removed: number }>(`/gc?ttl=${ttl}`, {}),
  version: () => http<{ version: string }>("/version", { cache: "no-store" }),

  // ── AntiFox 证据驱动交付：交付级报告（docx / 三语）──
  reportSettings: () => get<ReportSettings>(`/report-settings`),
  saveReportSettings: (patch: Partial<ReportSettings>) => put<ReportSettings>(`/report-settings`, patch),
  reports: () => get<ReportRecord[]>(`/reports`).then(arr),
  reportRecordDownloadUrl: (id: number) => `/api/reports/${id}/download`,
  deleteReport: (id: number) => del<{ ok: boolean }>(`/reports/${id}`),
  reportTemplates: () => get<ReportTemplate[]>(`/report-templates`).then(arr),
  createReportTemplate: (body: Partial<ReportTemplate>) => post<ReportTemplate>(`/report-templates`, body),
  updateReportTemplate: (id: number, body: Partial<ReportTemplate>) => put<ReportTemplate>(`/report-templates/${id}`, body),
  deleteReportTemplate: (id: number) => del<{ ok: boolean }>(`/report-templates/${id}`),
  // ── AntiFox 知识库（两层 RAG：全局通用知识 + 任务专属资料）──
  knowledgeDocs: (scope = "", task = 0) =>
    get<KnowledgeDoc[]>(`/knowledge?scope=${encodeURIComponent(scope)}&task=${task}`).then(arr),
  knowledgeDoc: (id: number) => get<KnowledgeDoc>(`/knowledge/${id}`),
  createKnowledgeDoc: (body: Partial<KnowledgeDoc>) => post<KnowledgeDoc>(`/knowledge`, body),
  updateKnowledgeDoc: (id: number, body: Partial<KnowledgeDoc>) => put<KnowledgeDoc>(`/knowledge/${id}`, body),
  deleteKnowledgeDoc: (id: number) => del<{ ok: boolean }>(`/knowledge/${id}`),
  knowledgeCategories: (task = 0) => get<string[]>(`/knowledge/categories?task=${task}`).then(arr),
  knowledgeSearch: (query: string, taskId = 0, topK = 5) =>
    post<{ mode: string; hits: KnowledgeHit[] }>(`/knowledge/search`, { query, task_id: taskId, top_k: topK }),
  knowledgeLogs: () => get<KnowledgeLog[]>(`/knowledge/logs`).then(arr),
  knowledgeConfig: () => get<KnowledgeConfig>(`/knowledge/config`),
  saveKnowledgeConfig: (body: Partial<KnowledgeConfig>) => put<{ ok: boolean }>(`/knowledge/config`, body),
  testKnowledgeEmbedder: () => post<{ ok: boolean; dim: number }>(`/knowledge/config/test`, {}),
  reindexKnowledge: () => post<{ ok: boolean; indexed: number }>(`/knowledge/reindex`, {}),
  // 知识文档上传走 multipart（文件 + 归属范围），不能走通用 JSON 封装。
  uploadKnowledgeDoc: async (file: File, scope: string, taskId: number, category: string) => {
    const fd = new FormData();
    fd.append("file", file);
    fd.append("scope", scope);
    fd.append("task_id", String(taskId));
    fd.append("category", category);
    const token = getToken();
    const r = await fetch(`/api/knowledge/upload`, {
      method: "POST",
      body: fd,
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    });
    const body = await r.json().catch(() => ({}));
    if (!r.ok) {
      throw new Error((body as { error?: string }).error || `上传失败（HTTP ${r.status}）`);
    }
    return body as KnowledgeDoc;
  },
};

export type MiscApi = typeof miscApi;
