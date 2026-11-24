// Real backend client transport: /api/* is proxied to the Go backend (next.config rewrites).
// Domain methods live in the sibling domain files and are aggregated in lib/api.ts, which
// re-exports http/sseUrl for the few call sites that use them directly.
/**
 * 主要函数/类型：
 *   - http / get / post / put / patch / del：带鉴权与统一错误处理的 fetch 封装（MOCK 构建短路到 mock handler）
 *   - sseUrl：构建 SSE 流地址（生产同源、开发直连 :8787）
 *   - openSSE：打开 SSE 连接（同源带 cookie，本地开发跨端口才带凭据）
 *   - getToken：读取 localStorage 中的登录 token
 *   - arr / tq / findingFilterParams / interceptPageQuery：空数组兜底、task 查询串与筛选序列化小工具
 */

import type { FindingQuery, InterceptApprovalFilter } from "@/lib/types";

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("artex_token");
}

/**
 * authFetch：给「不能走 http()」的请求用 —— 文件上传/下载/SSE 之外的裸 fetch。
 *
 * 为什么需要：http() 已经处理了 401（清 token + 跳登录页）和后端返回的
 * `{"error": "..."}` 消息。但项目里有十几处直接写 fetch(...)，各写各的：
 * 有的只抛状态码（用户看到「export: 401」却不知道要重新登录），
 * 有的抛中文、有的抛英文。全部走这个函数，行为就一致了。
 */
export async function authFetch(input: string, init?: RequestInit): Promise<Response> {
  const token = getToken();
  // 只有 body 是字符串（JSON）时才设 Content-Type。FormData / Blob / URLSearchParams
  // 必须让浏览器自己设 —— 手设成 application/json 会让服务端解析失败。
  const isJsonBody = typeof init?.body === "string";
  const r = await fetch(input, {
    ...init,
    headers: {
      ...(isJsonBody ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers as Record<string, string> | undefined),
    },
  });
  if (r.status === 401) {
    if (typeof window !== "undefined") {
      localStorage.removeItem("artex_token");
      document.cookie = "artex_token=; path=/; max-age=0";
      window.location.href = "/login";
    }
    throw new Error("未授权");
  }
  if (!r.ok) {
    // 尽量把后端的可读错误取出来（与 http() 同款逻辑）。
    let message = `${init?.method ?? "GET"} ${input}: ${r.status}`;
    try {
      const payload = (await r.json()) as { error?: unknown };
      if (typeof payload.error === "string" && payload.error.trim()) {
        message = payload.error.trim();
      }
    } catch {
      // 非 JSON 响应（如文件下载的 404 HTML）保留状态码兜底。
    }
    throw new Error(message);
  }
  return r;
}

/**
 * downloadBlob：把一个响应存成文件下载。
 *
 * 用于报告、导出、工作区下载这类「后端返回二进制」的场景。
 * 文件名优先取 Content-Disposition（后端会带中文名），取不到则用传入的兜底名。
 */
export async function downloadBlob(input: string, fallbackName: string, init?: RequestInit): Promise<void> {
  const r = await authFetch(input, init);
  const blob = await r.blob();
  const disp = r.headers.get("Content-Disposition") ?? "";
  const m = disp.match(/filename="?([^"]+)"?/);
  const filename = m?.[1] ?? fallbackName;
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/**
 * uploadForm：POST 一个 FormData。
 *
 * FormData 的 Content-Type 必须由浏览器自己设（带 boundary）——手设成
 * application/json 会让服务端解析失败，所以这里不走 authFetch 的 JSON 分支，
 * 但仍然复用它的 401 处理与可读错误消息。
 */
export async function uploadForm<T>(input: string, fd: FormData, method = "POST"): Promise<T> {
  const token = getToken();
  const r = await fetch(input, {
    method,
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    body: fd,
  });
  if (r.status === 401) {
    if (typeof window !== "undefined") {
      localStorage.removeItem("artex_token");
      document.cookie = "artex_token=; path=/; max-age=0";
      window.location.href = "/login";
    }
    throw new Error("未授权");
  }
  if (!r.ok) {
    let message = `上传失败: ${r.status}`;
    try {
      const payload = (await r.json()) as { error?: unknown };
      if (typeof payload.error === "string" && payload.error.trim()) {
        message = payload.error.trim();
      }
    } catch {
      try {
        const text = (await r.text()).trim();
        if (text) message = text;
      } catch {
        // 保留状态码兜底。
      }
    }
    throw new Error(message);
  }
  return (await r.json()) as T;
}

export async function http<T>(path: string, init?: RequestInit): Promise<T> {
  // 动态 import 让 6800 行 mock 数据只在 MOCK 构建进 bundle：此处必须内联
  // process.env 判定（而非经 lib/mock/enabled 的 MOCK 常量），DefinePlugin 才能
  // 在生产构建把它折成 if(false) 并整体消除该 import() 依赖。
  if (process.env.NEXT_PUBLIC_MOCK === "1")
    return (await import("@/lib/mock/handler")).mockHandle<T>(init?.method ?? "GET", path, init?.body ?? null);
  const token = getToken();
  const r = await fetch(`/api${path}`, {
    ...init,
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers as Record<string, string> | undefined),
    },
  });
  if (r.status === 401) {
    if (typeof window !== "undefined") {
      localStorage.removeItem("artex_token");
      document.cookie = "artex_token=; path=/; max-age=0";
      window.location.href = "/login";
    }
    throw new Error("未授权");
  }
  if (!r.ok) {
    const fallback = `${init?.method ?? "GET"} ${path}: ${r.status}`;
    let message = fallback;
    try {
      const payload = (await r.json()) as { error?: unknown };
      if (typeof payload.error === "string" && payload.error.trim()) {
        message = payload.error.trim();
      }
    } catch {
      // Keep the status-based fallback for empty or non-JSON error responses.
    }
    throw new Error(message);
  }
  if (r.status === 204) return undefined as T;
  return r.json();
}

// sseUrl builds a URL for Server-Sent Events streams.
//
// Production (static export / Docker): the Go backend serves the web UI *and* the
// SSE streams on the same port, so we default to a same-origin (relative) URL.
// This is what makes reverse-proxy deploys work: a page loaded from
// https://<domain>/ connects to https://<domain>/api/..., which the proxy forwards
// to the backend — no need to expose :8787 publicly. Hardcoding :8787 here used to
// break exactly that (https://<domain>:8787/... is unreachable when only 443 is open).
//
// Dev (next dev): SSE must NOT go through the Next.js `/api` rewrite — that proxy
// buffers the streamed response, so event frames never reach the browser (the
// EventSource opens but receives 0 messages). So in dev only we connect straight
// to the Go backend on :8787, whose CORS is open.
//
// Override either default with NEXT_PUBLIC_SSE_BASE (set it to "" to force same-origin).
// Token is appended as ?token= because SSE can't carry cookies cross-origin.

// sseUrl 构建 SSE 地址。同源流由浏览器携带登录 cookie，避免把 JWT 放进
// URL 查询串而泄漏到访问日志；仅跨源、无法使用该 cookie 时才附加 token。
export function sseUrl(path: string): string {
  const base =
    process.env.NEXT_PUBLIC_SSE_BASE ??
    (process.env.NODE_ENV !== "production" && typeof window !== "undefined"
      ? `${window.location.protocol}//${window.location.hostname}:8787`
      : "");
  const token = base !== "" ? getToken() : null;
  const sep = path.includes("?") ? "&" : "?";
  return token ? `${base}${path}${sep}token=${encodeURIComponent(token)}` : `${base}${path}`;
}

// openSSE 打开 SSE 流。同源流已由浏览器携带 cookie，只在本地 Next/Go 开发
// 组合下才允许带凭据跨端口；自定义远端 SSE 端点保持原样。
export function openSSE(path: string): EventSource {
  const url = sseUrl(path);
  const target = new URL(url, window.location.href);
  // Same-origin streams already send cookies. Only opt into cross-port cookies
  // for the local Next/Go dev pair; keep custom remote SSE endpoints unchanged.
  const local = ["localhost", "127.0.0.1", "[::1]"].includes(target.hostname);
  return new EventSource(url, { withCredentials: local && target.hostname === window.location.hostname });
}

export const get = <T>(p: string) => http<T>(p);
export const post = <T>(p: string, body?: unknown) =>
  http<T>(p, { method: "POST", body: body ? JSON.stringify(body) : undefined });
export const put = <T>(p: string, body?: unknown) =>
  http<T>(p, { method: "PUT", body: body ? JSON.stringify(body) : undefined });
export const patch = <T>(p: string, body?: unknown) =>
  http<T>(p, { method: "PATCH", body: body ? JSON.stringify(body) : undefined });
export const del = <T>(p: string, body?: unknown) =>
  http<T>(p, { method: "DELETE", body: body ? JSON.stringify(body) : undefined });

// Go serializes nil slices as JSON null — coerce to [].
// arr 把「可能缺失」的字段兜底成空数组。
// 除了 null/undefined，**非数组值**（例如后端返回了对象、演示模式返回了 {}）也必须
// 兜成 []：调用方普遍直接 .map / .find，非数组会抛 TypeError 让整页白屏。
export const arr = <T>(x: T[] | null | undefined): T[] => (Array.isArray(x) ? x : []);
export const tq = (task?: string, sep: "?" | "&" = "?") => (task ? `${sep}task=${encodeURIComponent(task)}` : "");

// findingFilterParams 把发现页的筛选条件序列化成 query string。列表 / 分组 /
// 资产树 / 导出共用同一份,新增筛选项只改这里(后端也只解析这一份)。
export function findingFilterParams(q: Omit<FindingQuery, "page" | "pageSize">): URLSearchParams {
  const p = new URLSearchParams();
  if (q.severity && q.severity !== "all") p.set("severity", q.severity);
  if (q.status && q.status !== "all") p.set("status", q.status);
  if (q.vulnclass && q.vulnclass !== "all") p.set("vulnclass", q.vulnclass);
  if (q.task && q.task !== "all") p.set("task_id", q.task);
  if (q.query?.trim()) p.set("q", q.query.trim());
  if (q.sort) p.set("sort", q.sort);
  if (q.assetScope) p.set("asset_scope", q.assetScope);
  if (q.assetIDs?.length) p.set("asset_ids", q.assetIDs.join(","));
  // 日期区间与方向必须一起序列化：列表 / 分组 / 资产树 / 导出共用本函数，
  // 少了这三项时前端「筛选生效了」（fingerprint 变了会重取），但后端结果原样不变。
  if (q.timeDirection) p.set("time_direction", q.timeDirection);
  if (q.from) p.set("from", q.from);
  if (q.to) p.set("to", q.to);
  return p;
}

export function interceptPageQuery(page: number, size: number, filter: InterceptApprovalFilter) {
  const query = new URLSearchParams({ page: String(page), size: String(size) });
  if (filter.status) query.set("status", filter.status);
  if (filter.decision_source) query.set("decision_source", filter.decision_source);
  // 后端 interceptFilterParams 读的是 q/from/to；少了这三行，审批记录页的
  // 关键字搜索与时间筛选都是静默 no-op（UI 会重新请求，结果却完全不变）。
  if (filter.q?.trim()) query.set("q", filter.q.trim());
  if (filter.from) query.set("from", filter.from);
  if (filter.to) query.set("to", filter.to);
  return query.toString();
}
