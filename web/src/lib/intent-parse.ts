/**
 * intent-parse.ts：把用户的一句自然语言需求解析成可提交的任务参数。
 *
 * 为什么要这一层：现在新建任务要填描述、目标、场景分类、LLM 链、超时、心跳、
 * 关联企业、拦截规则等十几项，用户得先学会这套配置才能开始，等于"告诉软件每一步
 * 怎么操作"。但后端 createTask 其实只强校验 description + goal 两项，其余全有默认值
 * （见 server/server.go 的 createTask 与 createTaskReq 注释）。所以字段墙纯粹是
 * UI 层暴露过度，不是能力缺失。
 *
 * 这一层做三件事，都不依赖 LLM，纯本地可解释：
 *   1. 抽目标：从文本里认出域名 / IP / CIDR / URL，用户不必单独填"测试对象"；
 *   2. 推荐场景：按关键词命中已有的任务分类（它已经绑好 planner/worker 模板、
 *      审核开关、资产模式），用户不必懂模板概念；
 *   3. 组织描述与目标：把一句话拆成"背景"和"要达成什么"，喂给 agent。
 *
 * 刻意不做的事：不猜用户没说的事。抽不到资产就留空并提示补全，不瞎填；
 * 场景只给"推荐"，最终由用户确认（AI Native 不等于替用户拍板）。
 */

/** 从需求文本中抽出的结构化线索。字段为空表示文本里确实没有，不是解析失败。 */
export interface ParsedIntent {
  /** 原始输入（已 trim）。 */
  raw: string;
  /** 抽到的目标资产，去重后按出现顺序；可为空数组。 */
  assets: string[];
  /** 推断出的任务描述（背景）。 */
  description: string;
  /** 推断出的任务目标（要达成什么）。 */
  goal: string;
  /** 推荐命中：分类 id 与命中理由；未命中为 null。 */
  suggestedCategoryId: number | null;
  suggestedReason: string;
  /** 命中的场景关键词，用于 UI 展示"为什么推荐它"。 */
  matchedKeywords: string[];
}

/** 分类的最小子集，避免依赖 api 类型造成循环引用。 */
export interface CategoryLike {
  id: number;
  name: string;
}

// ── 目标资产抽取 ────────────────────────────────────────────────

/** URL：带 scheme 的完整地址，取 host 部分。 */
const RE_URL =
  /\bhttps?:\/\/([a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+(?::\d{1,5})?(?:\/[^\s，。；、）)]*)?)/gi;
/** CIDR：如 10.0.0.0/8。 */
const RE_CIDR = /\b(?:\d{1,3}\.){3}\d{1,3}\/\d{1,2}\b/g;
/** 裸 IPv4（排除已作为 CIDR 一部分的情况由去重与顺序处理）。 */
const RE_IPV4 = /\b(?:\d{1,3}\.){3}\d{1,3}\b/g;
/** 域名：至少两段，最后一段是常见 TLD；允许前缀通配符与端口。 */
const RE_DOMAIN =
  /\b(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+(?:com|cn|net|org|io|co|gov|edu|info|biz|me|dev|app|xyz|top|site|online|tech|cloud|vip|cc|tv|hk|tw|jp|us|uk|de|ru|in|br|au|ca|fr)(?::\d{1,5})?\b/gi;

/**
 * 从需求文本抽取目标资产，按在原文中出现的先后顺序返回。
 *
 * 顺序必须按出现位置而不是按类型分组：用户写「先看 a.com，再看 10.1.1.1」时，
 * 展示顺序若变成 IP 在前，会让人以为系统理解错了。所以这里先收集「位置 + 值」，
 * 最后统一按位置排序。
 *
 * 去重规则：同一资产只留第一次。类型之间也要避免重复识别 —— 一个 URL 里的域名
 * 不应该再被当成第二个资产，因此长形态（URL/CIDR）命中后先把那段抹成等长空白，
 * 再做 IP/域名的匹配。
 */
export function extractAssets(text: string): string[] {
  const hits: Array<{ at: number; value: string }> = [];
  const seen = new Set<string>();

  const take = (raw: string, at: number) => {
    const v = raw.trim().replace(/[.,;:，。；：、）)]+$/, "");
    if (!v) return;
    const key = v.toLowerCase();
    if (seen.has(key)) return;
    seen.add(key);
    hits.push({ at, value: v });
  };

  // 第一轮：URL 与 CIDR（长形态），命中后抹掉原文对应片段
  let rest = text.replace(RE_URL, (full: string, _host: string, offset: number) => {
    take(full, offset);
    return " ".repeat(full.length);
  });
  rest = rest.replace(RE_CIDR, (full: string, offset: number) => {
    take(full, offset);
    return " ".repeat(full.length);
  });

  // 第二轮：裸 IPv4 与域名，扫的是被抹过的文本，但位置仍与原文一致
  rest.replace(RE_IPV4, (full: string, offset: number) => {
    take(full, offset);
    return full;
  });
  rest.replace(RE_DOMAIN, (full: string, offset: number) => {
    take(full, offset);
    return full;
  });

  return hits.sort((a, b) => a.at - b.at).map((h) => h.value);
}

// ── 场景推荐 ────────────────────────────────────────────────────

/**
 * 场景关键词表。键是"语义簇"，值是要匹配的词；命中后按分类名的包含关系映射到实际分类。
 *
 * 为什么用"语义簇 → 分类名"而不是写死分类 id：分类是用户可增删改的（task_categories
 * 表，支持自定义），写死 id 一旦用户改名/删除就失效。按名字匹配至少能跟随改名。
 */
const SCENE_HINTS: Array<{ scene: string; keywords: string[] }> = [
  {
    scene: "漏洞挖掘",
    keywords: [
      "漏洞",
      "挖掘",
      "扫描",
      "sql注入",
      "sql 注入",
      "注入",
      "xss",
      "csrf",
      "ssrf",
      "越权",
      "未授权",
      "文件上传",
      "反序列化",
      "rce",
      "命令执行",
      "弱口令",
      "爆破",
      "目录扫描",
      "指纹",
      "渗透测试",
      "漏扫",
    ],
  },
  {
    scene: "漏洞利用",
    keywords: ["利用", "exploit", "getshell", "拿shell", "提权后", "打点", "poc", "exp", "反弹shell"],
  },
  {
    scene: "内网渗透",
    keywords: ["内网", "横向", "域控", "域内", "代理", "隧道", "frp", "c2", "免杀", "横向移动", "smb", "rdp"],
  },
  {
    scene: "权限提升",
    keywords: ["提权", "权限提升", "sudo", "suid", "内核提权", "本地提权"],
  },
  {
    scene: "代码审计",
    keywords: ["代码审计", "源码", "审计", "静态分析", "白盒", "source code", "代码审查", "污点"],
  },
];

/**
 * 按需求文本推荐场景分类。
 *
 * 匹配策略：先算每个语义簇的命中次数，取最高分；再用"分类名是否包含该簇名"或反向
 * 包含来映射到真实分类。映射不到就返回 null —— 宁可不推荐，也不乱推荐。
 */
export function suggestCategory(
  text: string,
  categories: CategoryLike[],
): { categoryId: number | null; reason: string; matchedKeywords: string[] } {
  if (categories.length === 0) {
    return { categoryId: null, reason: "", matchedKeywords: [] };
  }
  const lower = text.toLowerCase();

  let best: { scene: string; hits: string[] } | null = null;
  for (const { scene, keywords } of SCENE_HINTS) {
    const hits = keywords.filter((k) => lower.includes(k.toLowerCase()));
    if (hits.length > 0 && (!best || hits.length > best.hits.length)) {
      best = { scene, hits };
    }
  }
  if (!best) {
    return { categoryId: null, reason: "", matchedKeywords: [] };
  }

  // 精确同名优先，其次互相包含（"代码审计" 与 "代码审计任务" 互认）
  const exact = categories.find((c) => c.name === best.scene);
  const loose = exact ?? categories.find((c) => c.name.includes(best.scene) || best.scene.includes(c.name));
  if (!loose) {
    return {
      categoryId: null,
      reason: `识别到「${best.scene}」类需求，但当前没有对应场景，将按默认配置创建`,
      matchedKeywords: best.hits,
    };
  }
  return {
    categoryId: loose.id,
    reason: `命中关键词 ${best.hits
      .slice(0, 3)
      .map((k) => `「${k}」`)
      .join("")}，推荐场景「${loose.name}」`,
    matchedKeywords: best.hits,
  };
}

// ── 描述与目标组织 ──────────────────────────────────────────────

/**
 * 把一句话拆成 description（背景：测什么）与 goal（要达成什么）。
 *
 * 后端只要求两者非空。用户只写一句话时，最简单可靠的做法是：description 用原句，
 * goal 从原句推导。推导不追求"聪明"，只求不误导 —— 因为它会直接进入 agent 的
 * 提示词，写错比留白伤害更大。
 */
export function buildDescriptionAndGoal(raw: string, assets: string[]): { description: string; goal: string } {
  const text = raw.trim();
  const target = assets[0] ?? "";

  // 描述：原句 + 明确点出测试对象，便于 agent 定位
  const description = target ? `${text}\n\n测试对象：${assets.join("、")}` : text;

  // 目标：把用户的话转成祈使句。已经像目标（含"拿下/获取/验证/找出"等动词）就原样用。
  const hasGoalVerb = /拿下|获取|获得|找出|发现|确认|验证|证明|拿到|挖出|找到|提交|利用|提权|进入|控制/.test(text);
  let goal: string;
  if (hasGoalVerb) {
    goal = text;
  } else if (target) {
    // 原句里往往已经点了名（"测试 example.com 的登录接口…"）。此时再套一层
    // "对 example.com 完成…" 会变成 "对 example.com 完成example.com 的登录接口" ——
    // 重复一遍测试对象，读起来像机器拼接的。已经提过就直接接结论要求。
    goal = text.includes(target) ? `${text}，并给出可复现的漏洞结论` : `对 ${target} 完成${text}，并给出可复现的漏洞结论`;
  } else {
    goal = text;
  }

  return { description, goal };
}

/** 一步完成解析，供 UI 直接调用。 */
export function parseIntent(raw: string, categories: CategoryLike[]): ParsedIntent {
  const text = raw.trim();
  const assets = extractAssets(text);
  const { description, goal } = buildDescriptionAndGoal(text, assets);
  const { categoryId, reason, matchedKeywords } = suggestCategory(text, categories);
  return {
    raw: text,
    assets,
    description,
    goal,
    suggestedCategoryId: categoryId,
    suggestedReason: reason,
    matchedKeywords,
  };
}
