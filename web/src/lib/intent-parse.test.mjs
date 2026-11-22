import { buildDescriptionAndGoal, extractAssets, parseIntent, suggestCategory } from "./intent-parse.ts";
import assert from "node:assert/strict";
import test from "node:test";

const categories = [
  { id: 2, name: "内网渗透" },
  { id: 4, name: "漏洞利用" },
  { id: 5, name: "漏洞挖掘" },
  { id: 6, name: "代码审计" },
  { id: 3, name: "权限提升" },
];

test("extractAssets：抓域名", () => {
  assert.deepEqual(extractAssets("测试 example.com 这个站点"), ["example.com"]);
});

test("extractAssets：抓带端口的域名", () => {
  assert.deepEqual(extractAssets("看一下 test.example.com:8080 的后台"), ["test.example.com:8080"]);
});

test("extractAssets：抓 IP", () => {
  assert.deepEqual(extractAssets("对 192.168.1.10 做弱口令检测"), ["192.168.1.10"]);
});

test("extractAssets：抓 CIDR", () => {
  assert.deepEqual(extractAssets("扫描 10.0.0.0/24 网段"), ["10.0.0.0/24"]);
});

test("extractAssets：URL 保留完整路径，且其域名不重复计入", () => {
  const got = extractAssets("请求 https://api.example.com/v1/login 有没有注入");
  assert.equal(got.length, 1, `应只识别一个资产，实际 ${JSON.stringify(got)}`);
  assert.equal(got[0], "https://api.example.com/v1/login");
});

test("extractAssets：多个资产都抓到且去重", () => {
  const got = extractAssets("先看 a.com，再看 10.1.1.1，然后回到 a.com");
  assert.deepEqual(got, ["a.com", "10.1.1.1"]);
});

test("extractAssets：没有资产的纯意图描述返回空数组", () => {
  assert.deepEqual(extractAssets("帮我找找有没有注入漏洞"), []);
});

test("extractAssets：不把普通英文单词误认成域名", () => {
  assert.deepEqual(extractAssets("check the login page for ssrf"), []);
});

test("suggestCategory：注入类关键词命中漏洞挖掘", () => {
  const r = suggestCategory("测试 example.com 的登录接口有没有 sql 注入", categories);
  assert.equal(r.categoryId, 5);
  assert.ok(r.matchedKeywords.length > 0);
});

test("suggestCategory：内网关键词命中内网渗透", () => {
  const r = suggestCategory("从这台机器做内网横向到域控", categories);
  assert.equal(r.categoryId, 2);
});

test("suggestCategory：提权命中权限提升", () => {
  const r = suggestCategory("已经拿到 shell，想本地提权", categories);
  assert.equal(r.categoryId, 3);
});

test("suggestCategory：源码/审计命中代码审计", () => {
  const r = suggestCategory("对这份源码做代码审计", categories);
  assert.equal(r.categoryId, 6);
});

test("suggestCategory：无关键词时不乱推荐", () => {
  const r = suggestCategory("随便看看", categories);
  assert.equal(r.categoryId, null);
  assert.equal(r.reason, "");
});

test("suggestCategory：关键词命中但分类不存在时给出说明而不硬塞 id", () => {
  const r = suggestCategory("做内网横向", [{ id: 9, name: "别的场景" }]);
  assert.equal(r.categoryId, null);
  assert.match(r.reason, /内网渗透/);
});

test("suggestCategory：多个簇命中时取命中词更多的那个", () => {
  // "提权" + "内网" 各命中；构造一方明显更多
  const r = suggestCategory("内网 横向 域控 代理 隧道 提权", categories);
  assert.equal(r.categoryId, 2, "内网命中词更多，应优先内网渗透");
});

test("buildDescriptionAndGoal：描述带出测试对象", () => {
  const { description, goal } = buildDescriptionAndGoal("测试 example.com", ["example.com"]);
  assert.match(description, /测试对象：example\.com/);
  assert.ok(goal.length > 0);
});

test("buildDescriptionAndGoal：已含目标动词时 goal 原样保留", () => {
  const raw = "拿下 example.com 的后台管理权限";
  const { goal } = buildDescriptionAndGoal(raw, ["example.com"]);
  assert.equal(goal, raw);
});

test("buildDescriptionAndGoal：无目标动词时补成祈使句并带上资产", () => {
  const { goal } = buildDescriptionAndGoal("测试 example.com 登录接口", ["example.com"]);
  assert.match(goal, /example\.com/);
  assert.match(goal, /漏洞结论/);
});

test("buildDescriptionAndGoal：原句已点过测试对象时不再重复套一层", () => {
  // 曾经生成过 "对 example.com 完成example.com 的登录接口…" —— 测试对象出现两次。
  const { goal } = buildDescriptionAndGoal("测试 example.com 的登录接口有没有 SQL 注入", ["example.com"]);
  const hits = goal.match(/example\.com/g) ?? [];
  assert.equal(hits.length, 1, `测试对象应只出现一次，实际：${goal}`);
  assert.match(goal, /漏洞结论/);
});

test("buildDescriptionAndGoal：原句没点测试对象时才补「对 X 完成」", () => {
  const { goal } = buildDescriptionAndGoal("做一轮资产梳理", ["10.0.0.0/24"]);
  assert.match(goal, /^对 10\.0\.0\.0\/24 完成/);
});

test("parseIntent：端到端把一句话变成可提交参数", () => {
  const p = parseIntent("请测试 shop.example.com 的商品搜索有没有 sql 注入", categories);
  assert.deepEqual(p.assets, ["shop.example.com"]);
  assert.equal(p.suggestedCategoryId, 5);
  assert.ok(p.description.includes("shop.example.com"));
  assert.ok(p.goal.length > 0);
  assert.match(p.suggestedReason, /漏洞挖掘/);
});

test("parseIntent：只有意图没有资产时也不崩", () => {
  const p = parseIntent("找找有没有弱口令", categories);
  assert.deepEqual(p.assets, []);
  assert.equal(p.suggestedCategoryId, 5);
  assert.ok(p.description.length > 0);
  assert.ok(p.goal.length > 0);
});
