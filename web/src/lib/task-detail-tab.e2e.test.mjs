// 任务详情页默认 tab 的端到端验证。
//
// 为什么不能在浏览器里点着验：本机的浏览器可视化通道坏了（300x200 的截图都报超 10MiB），
// DOM 快照也返回空。所以把验证拆成两段最要紧的：
//   1) 接口契约：前端 findingsPage 真正拼出的 query（task_id + limit）能否拿到准确的 total
//   2) 纯逻辑：拿到的 total 与任务状态交给 defaultTabFor，是否落到正确的 tab
// 两段合起来就是用户点进详情页时看到的第一屏，没有第三处可能出错的地方。
//
// 取数方式：以前这里写死任务 9/10/11，换一台机器、清一次库就整组红掉 ——
// 红的是环境，不是代码。现在改成从实例里现挑三个符合特征的任务；
// 挑不到就跳过并说明缺哪一种，让"数据没了"和"契约坏了"看起来是两件事。
//
// 用法：ARTEX_E2E_BASE / ARTEX_E2E_USER / ARTEX_E2E_PASS 可覆盖默认值。

import { defaultTabFor } from "./task-detail-tab.ts";
import assert from "node:assert/strict";
import test from "node:test";

const BASE = process.env.ARTEX_E2E_BASE ?? "http://127.0.0.1:8795/api";
const USER = process.env.ARTEX_E2E_USER ?? "ARTEX";
const PASS = process.env.ARTEX_E2E_PASS ?? "UiProbe@2026!";

// 与 web/src/lib/api/http.ts 的 findingFilterParams 保持一致：task 走 task_id 这个参数名，
// 分页参数是 page/limit（不是 page_size）。名字写错不会报错，只会静默返回全量或空，
// 所以这里宁可重复一遍也不从实现里 import —— 要验的就是两边对得上。
function buildQuery({ task, page, pageSize }) {
  const p = new URLSearchParams();
  if (task && task !== "all") p.set("task_id", task);
  p.set("page", String(page));
  p.set("limit", String(pageSize));
  return p.toString();
}

// request 在实例没起来时返回 null，让调用方跳过而不是甩一个连接拒绝的堆栈：
// 前端单独跑测试时不该因为"后端没开"而变红。
async function request(path, init = {}) {
  try {
    return await fetch(`${BASE}${path}`, init);
  } catch (err) {
    const code = err?.cause?.code ?? err?.code;
    if (code === "ECONNREFUSED" || code === "ENOTFOUND") return null;
    throw err;
  }
}

const SKIP_NO_INSTANCE = `验证实例未运行（${BASE}），跳过端到端用例`;

async function login() {
  const r = await request(`/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username: USER, password: PASS }),
  });
  if (!r) return null;
  assert.equal(r.status, 200, `登录失败 HTTP ${r.status}`);
  const { token } = await r.json();
  return token;
}

async function findingsTotal(token, task) {
  const q = buildQuery({ task, page: 1, pageSize: 1 });
  const r = await request(`/exploration/findings?${q}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  assert.equal(r.status, 200, `发现接口 HTTP ${r.status}（query=${q}）`);
  const body = await r.json();
  // 这条断言很关键：如果后端某天改成分页参数叫 page_size，这里会立刻炸，
  // 而不是让页面上"有发现却默认打开会话"这种软故障悄悄上线。
  assert.ok(
    body && typeof body.total === "number",
    `发现接口应返回带 total 的分页对象，实际：${JSON.stringify(body).slice(0, 120)}`,
  );
  return body.total;
}

// 从实例现挑三种特征的任务：已结束无发现 / 有发现 / 未结束且无发现。
// 只做 GET，不改任何数据。
let fixturePromise = null;
function fixtures(token) {
  if (!fixturePromise) fixturePromise = discover(token);
  return fixturePromise;
}

async function discover(token) {
  const r = await request(`/tasks`, { headers: { Authorization: `Bearer ${token}` } });
  assert.equal(r.status, 200, `任务列表 HTTP ${r.status}`);
  const body = await r.json();
  // 列表接口的形状是 {active, tasks:[...]}，不是裸数组。
  const list = Array.isArray(body) ? body : (body.tasks ?? []);
  const terminal = new Set(["done", "failed", "timeout"]);
  const found = { overview: null, findings: null, sessions: null };
  for (const task of list) {
    if (!task?.id) continue;
    const total = await findingsTotal(token, task.id);
    const row = { id: task.id, status: task.status, total };
    // 一个任务只归类一次：有发现的优先给 findings，避免把同一个任务
    // 同时当成"无发现"的反例，掩盖真正的问题。
    if (total > 0) {
      found.findings ??= row;
    } else if (terminal.has(task.status)) {
      found.overview ??= row;
    } else {
      found.sessions ??= row;
    }
    if (found.overview && found.findings && found.sessions) break;
  }
  return found;
}

function need(found, kind, t) {
  const row = found[kind];
  if (row) return row;
  const label = { overview: "已结束且无发现", findings: "有发现", sessions: "未结束且无发现" };
  t.skip(`实例里没有「${label[kind]}」的任务，跳过（这是数据缺口，不是代码问题）`);
  return null;
}

test("契约：发现接口按 task_id + limit 分页，返回带 total 的对象", async (t) => {
  const token = await login();
  if (!token) return t.skip(SKIP_NO_INSTANCE);
  // 故意用一个不存在的任务：即使库里没有数据，也能验证参数名没被写错 ——
  // 写错时后端会把过滤条件丢掉，返回的是全量统计而不是 0。
  const total = await findingsTotal(token, "999999999");
  assert.equal(total, 0, `不存在的任务不该有发现，实际 total=${total}`);
});

test("端到端：已结束无发现的任务默认落到「总览」", async (t) => {
  const token = await login();
  if (!token) return t.skip(SKIP_NO_INSTANCE);
  const row = need(await fixtures(token), "overview", t);
  if (!row) return;
  assert.equal(defaultTabFor({ status: row.status }, row.total), "overview", `status=${row.status} total=${row.total}`);
});

test("端到端：有发现的任务默认落到「发现」", async (t) => {
  const token = await login();
  if (!token) return t.skip(SKIP_NO_INSTANCE);
  const row = need(await fixtures(token), "findings", t);
  if (!row) return;
  assert.ok(row.total > 0, `total=${row.total}`);
  assert.equal(defaultTabFor({ status: row.status }, row.total), "findings", `status=${row.status} total=${row.total}`);
});

test("端到端：发现数只统计本任务，不串到别的任务", async (t) => {
  const token = await login();
  if (!token) return t.skip(SKIP_NO_INSTANCE);
  const found = await fixtures(token);
  const empty = need(found, "overview", t);
  const hit = need(found, "findings", t);
  if (!empty || !hit) return;
  const emptyAgain = await findingsTotal(token, empty.id);
  const hitAgain = await findingsTotal(token, hit.id);
  assert.equal(emptyAgain, 0, `任务 ${empty.id} 不应沾到任务 ${hit.id} 的发现`);
  assert.ok(hitAgain > 0, `任务 ${hit.id} 自己的发现不能漏`);
  // 换个顺序再取一次，防止是"第一次请求把结果缓存成另一个任务"的假象。
  assert.equal(await findingsTotal(token, hit.id), hitAgain);
  assert.equal(await findingsTotal(token, empty.id), 0);
});

test("端到端：还在跑且没发现的任务默认留在「会话」", async (t) => {
  const token = await login();
  if (!token) return t.skip(SKIP_NO_INSTANCE);
  const row = need(await fixtures(token), "sessions", t);
  if (!row) return;
  assert.ok(!["done", "failed", "timeout"].includes(row.status), `任务 ${row.id} 应未结束，实际 ${row.status}`);
  // 没跑完的活，用户想看的是"它现在在干什么"，实时流水就在会话页。
  assert.equal(defaultTabFor({ status: row.status }, row.total), "sessions");
});

test("端到端：三种状态各落各自的页，不是同一个默认值", async (t) => {
  const token = await login();
  if (!token) return t.skip(SKIP_NO_INSTANCE);
  const found = await fixtures(token);
  const rows = [need(found, "overview", t), need(found, "findings", t), need(found, "sessions", t)];
  if (rows.some((row) => !row)) return;
  const tabs = rows.map((row) => defaultTabFor({ status: row.status }, row.total));
  // 这条断言防的是"改了等于没改"：如果 defaultTabFor 退化成恒返回 sessions，
  // 上面单条用例可能因为任务恰好都在跑而全部通过，只有把三者放一起比对才看得出来。
  assert.equal(new Set(tabs).size, 3, `三个任务应落到三个不同页，实际：${JSON.stringify(tabs)}`);
});
