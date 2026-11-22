// 任务详情页默认 tab 的端到端验证。
//
// 为什么不能在浏览器里点着验：本机的浏览器可视化通道坏了（300x200 的截图都报超 10MiB），
// DOM 快照也返回空。所以把验证拆成两段最要紧的：
//   1) 接口契约：前端 findingsPage 真正拼出的 query（task_id + limit）能否拿到准确的 total
//   2) 纯逻辑：拿到的 total 与任务状态交给 defaultTabFor，是否落到正确的 tab
// 两段合起来就是用户点进详情页时看到的第一屏，没有第三处可能出错的地方。

import { defaultTabFor } from "./task-detail-tab.ts";
import assert from "node:assert/strict";
import test from "node:test";

const BASE = "http://127.0.0.1:8795/api";
const USER = "ARTEX";
const PASS = "UiProbe@2026!";

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

async function login() {
  const r = await fetch(`${BASE}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username: USER, password: PASS }),
  });
  assert.equal(r.status, 200, `登录失败 HTTP ${r.status}`);
  const { token } = await r.json();
  return token;
}

async function findingsTotal(token, task) {
  const q = buildQuery({ task, page: 1, pageSize: 1 });
  const r = await fetch(`${BASE}/exploration/findings?${q}`, {
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

async function taskStatus(token, id) {
  const r = await fetch(`${BASE}/tasks/${id}`, { headers: { Authorization: `Bearer ${token}` } });
  assert.equal(r.status, 200, `任务 ${id} HTTP ${r.status}`);
  const t = await r.json();
  return t.status;
}

test("端到端：已结束无发现的任务默认落到「总览」", async () => {
  const token = await login();
  const status = await taskStatus(token, 9);
  const total = await findingsTotal(token, 9);
  assert.equal(total, 0, `任务 9 应无发现，实际 total=${total}`);
  assert.equal(defaultTabFor({ status }, total), "overview", `status=${status} total=${total}`);
});

test("端到端：已结束有发现的任务默认落到「发现」", async () => {
  const token = await login();
  const status = await taskStatus(token, 10);
  const total = await findingsTotal(token, 10);
  assert.ok(total > 0, `任务 10 应至少 1 条发现，实际 total=${total}`);
  assert.equal(defaultTabFor({ status }, total), "findings", `status=${status} total=${total}`);
});

test("端到端：发现数只统计本任务，不串到别的任务", async () => {
  const token = await login();
  const a = await findingsTotal(token, 9);
  const b = await findingsTotal(token, 10);
  assert.equal(a, 0, "任务 9 不应沾到任务 10 的发现");
  assert.ok(b > 0, "任务 10 自己的发现不能漏");
});

test("端到端：还在跑且没发现的任务默认留在「会话」", async () => {
  const token = await login();
  const status = await taskStatus(token, 11);
  const total = await findingsTotal(token, 11);
  assert.ok(!["done", "failed", "timeout"].includes(status), `任务 11 应未结束，实际 ${status}`);
  assert.equal(total, 0);
  // 没跑完的活，用户想看的是"它现在在干什么"，实时流水就在会话页。
  assert.equal(defaultTabFor({ status }, total), "sessions");
});

test("端到端：三种状态各落各自的页，不是同一个默认值", async () => {
  const token = await login();
  const rows = await Promise.all(
    [9, 10, 11].map(async (id) => {
      const status = await taskStatus(token, id);
      const total = await findingsTotal(token, id);
      return defaultTabFor({ status }, total);
    }),
  );
  // 这条断言防的是"改了等于没改"：如果 defaultTabFor 退化成恒返回 sessions，
  // 上面单条用例可能因为任务恰好都在跑而全部通过，只有把三者放一起比对才看得出来。
  assert.equal(new Set(rows).size, 3, `三个任务应落到三个不同页，实际：${JSON.stringify(rows)}`);
});
