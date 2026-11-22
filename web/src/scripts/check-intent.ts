/**
 * check-intent.ts：「一句话开跑」解析层的回归脚本。
 *
 * 为什么是脚本而不是测试：这个仓前端没有单测框架（只有 biome），为两个纯函数
 * 引入 vitest 不划算。但解析层是纯函数、结论又直接影响 agent 拿到什么输入，
 * 必须有东西能一键验证 —— 所以做成 `npm run check:intent`，失败即 exit 1。
 *
 * 每条用例都来自真实踩过的坑，不是凑数：
 *   - 999.999.999.999 曾被抓成"测试对象"，agent 会照着去测一个不存在的地址
 *   - 版本 1.2.3.4 同样是四段数字，单看格式分不出来，只能看前文
 *   - 「外网渗透」分类库里一直有，但关键词表缺簇，导致永远不推荐
 */
import { buildDescriptionAndGoal, extractAssets, parseIntent, suggestCategory } from "../lib/intent-parse";

/** 库里自带的分类（artex_audit.task_categories 默认值）。分类名是用户可改的，
 *  所以这里只用来验证"名字能映射上"，不当作业务常量。 */
const CATS = [
  { id: 2, name: "内网渗透" },
  { id: 3, name: "权限提升" },
  { id: 4, name: "漏洞利用" },
  { id: 5, name: "漏洞挖掘" },
  { id: 6, name: "代码审计" },
  { id: 6865, name: "外网渗透" },
];

let failed = 0;

function eq(label: string, got: unknown, want: unknown) {
  const g = JSON.stringify(got);
  const w = JSON.stringify(want);
  if (g === w) {
    console.log(`  ok   ${label}`);
    return;
  }
  failed++;
  console.log(`  FAIL ${label}\n         got  ${g}\n         want ${w}`);
}

function scene(text: string) {
  const id = suggestCategory(text, CATS).categoryId;
  return id === null ? null : (CATS.find((c) => c.id === id)?.name ?? null);
}

console.log("目标资产抽取");
eq("非法 IP 不抓", extractAssets("注意 999.999.999.999 是错写的"), []);
eq("版本号不当 IP", extractAssets("版本 1.2.3.4 里面的逻辑有问题吗"), []);
eq("IPv4 正常抓", extractAssets("对 192.168.10.1 做端口扫描"), ["192.168.10.1"]);
eq("CIDR 与其中的 IP 都抓", extractAssets("10.0.0.0/24 里的 10.0.0.5"), ["10.0.0.0/24", "10.0.0.5"]);
eq("域名", extractAssets("测试 example.com 的登录接口"), ["example.com"]);
eq("URL 整体保留", extractAssets("看看 https://srm.ghpepower.com:2004/login 有没有越权"), [
  "https://srm.ghpepower.com:2004/login",
]);
eq("URL 里的域名不重复算", extractAssets("https://a.com/x 和 b.com"), ["https://a.com/x", "b.com"]);
eq("按出现顺序返回", extractAssets("先看 b.com，再看 10.1.1.1"), ["b.com", "10.1.1.1"]);
eq("无资产就是空", extractAssets("帮我看看这个站的安全情况"), []);

console.log("场景推荐");
eq("SQL 注入 -> 漏洞挖掘", scene("测试 example.com 的登录接口有没有 SQL 注入"), "漏洞挖掘");
eq("弱口令 -> 漏洞挖掘", scene("对 10.0.0.0/24 做弱口令检测"), "漏洞挖掘");
eq("内网横向 -> 内网渗透", scene("已经拿到 shell，想从这台机器做内网横向"), "内网渗透");
eq("源码审计 -> 代码审计", scene("审计一下这个源码仓库"), "代码审计");
eq("外网渗透 -> 外网渗透", scene("外网渗透一下 test.cn"), "外网渗透");
eq("做渗透 -> 外网渗透", scene("对 example.com 做渗透"), "外网渗透");
eq("子域 -> 外网渗透", scene("把 a.com 的子域全撸一遍"), "外网渗透");
eq("无所指 -> 不推荐", scene("帮我看看这个站的安全情况"), null);
eq("分类表为空时不报错", suggestCategory("SQL 注入", []).categoryId, null);
eq("分类表里没这个场景时说明原因", suggestCategory("提权", [{ id: 1, name: "别的场景" }]).categoryId, null);

console.log("描述与目标");
{
  const { description, goal } = buildDescriptionAndGoal("测试 a.com 的登录", ["a.com"]);
  eq("描述补出测试对象", description.includes("测试对象：a.com"), true);
  eq("目标已提过对象时不重复拼接", goal.includes("a.com 完成a.com"), false);
  eq("目标不重复", goal, "测试 a.com 的登录，并给出可复现的漏洞结论");
}
{
  const { goal } = buildDescriptionAndGoal("已经拿到 shell，想做内网横向", []);
  eq("无资产时目标沿用原句", goal, "已经拿到 shell，想做内网横向");
}

console.log("端到端");
{
  const p = parseIntent("测试 example.com 的登录接口有没有 SQL 注入", CATS);
  eq("资产", p.assets, ["example.com"]);
  eq("场景", p.suggestedCategoryId, 5);
  eq("目标非空", p.goal.length > 0, true);
  eq("描述非空", p.description.length > 0, true);
}

if (failed > 0) {
  console.error(`\n${failed} 条用例失败`);
  process.exit(1);
}
console.log("\n全部通过");
