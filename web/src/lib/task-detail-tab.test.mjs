import { defaultTabFor } from "./task-detail-tab.ts";
import assert from "node:assert/strict";
import test from "node:test";

test("有发现时优先去发现页 —— 不管任务是否还在跑", () => {
  for (const status of ["running", "paused", "done", "failed", "timeout", "queued"]) {
    assert.equal(defaultTabFor({ status }, 3), "findings", `status=${status}`);
  }
});

test("无发现且已结束 → 总览", () => {
  for (const status of ["done", "failed", "timeout"]) {
    assert.equal(defaultTabFor({ status }, 0), "overview", `status=${status}`);
  }
});

test("无发现且还在跑 → 会话（能看实时动静）", () => {
  for (const status of ["created", "queued", "running", "paused"]) {
    assert.equal(defaultTabFor({ status }, 0), "sessions", `status=${status}`);
  }
});

test("任务未加载时保守回会话，不做猜测", () => {
  assert.equal(defaultTabFor(null, 0), "sessions");
  assert.equal(defaultTabFor(null, 99), "sessions");
});

test("status 缺失不当作已结束", () => {
  // 后端 dto 理论上总带 status，但别让缺字段把用户直接丢进总览页。
  assert.equal(defaultTabFor({}, 0), "sessions");
  assert.equal(defaultTabFor({ status: "" }, 0), "sessions");
});

test("发现数判定用 > 0，0 与负数都不算有发现", () => {
  assert.equal(defaultTabFor({ status: "done" }, 0), "overview");
  assert.equal(defaultTabFor({ status: "done" }, 1), "findings");
});
