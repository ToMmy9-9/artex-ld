/**
 * intent-launcher.tsx：仪表盘顶部的「一句话开跑」入口。
 *
 * 这个组件要解决的具体问题：新建任务原本需要填十几项配置（描述、目标、场景、LLM 链、
 * 超时、心跳、关联企业、拦截规则……），用户得先学会这套管线才能开始 —— 等于
 * "告诉软件每一步怎么操作"。但后端 createTask 只强校验 description + goal 两项，
 * 其余全有默认值。所以这里让用户只写一句"我要测什么、想要什么结果"：
 *
 *   用户写 → 实时解析出目标资产与推荐场景 → 展示"我理解成了什么" → 确认即建任务
 *
 * 设计取舍（对应 AI Native 的判断标准）：
 *   - 用户给结果，不给步骤：输入框只要一句话，不暴露模板/心跳/LLM 链。
 *   - 解析结果必须可见可改：解析是猜测，猜错时要让用户当场看见并改掉，
 *     而不是默默建一个错任务。所以有"理解成了什么"这一栏，标题和目标都可编辑。
 *   - 不做无法解释的自动化：抽不到资产就明说"没识别到测试对象"，不乱填。
 *   - 保留逃生阀：想精细配置的用户走"高级配置"进原表单，功能一个都没少。
 */
"use client";

import * as React from "react";

import { useRouter } from "next/navigation";

import { ArrowRightIcon, CheckIcon, Loader2Icon, PencilIcon, SparklesIcon, TargetIcon, XIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { api } from "@/lib/api";
import { type ParsedIntent, parseIntent } from "@/lib/intent-parse";
import type { TaskCategory } from "@/lib/types";
import { cn } from "@/lib/utils";

/** 示例需求，点一下直接填入输入框 —— 降低"不知道该怎么写"的门槛。 */
const EXAMPLES = [
  "测试 example.com 的登录接口有没有 SQL 注入",
  "对 10.0.0.0/24 网段做资产梳理和弱口令检测",
  "已经拿到 shell，想从这台机器做内网横向",
];

export function IntentLauncher({ onAdvanced }: { onAdvanced?: () => void }) {
  const router = useRouter();
  const [text, setText] = React.useState("");
  const [categories, setCategories] = React.useState<TaskCategory[]>([]);
  const [creating, setCreating] = React.useState(false);
  const [error, setError] = React.useState("");
  // 解析结果：输入变化时重算；用户手工改过 goal/description 后以手工值为准。
  const [parsed, setParsed] = React.useState<ParsedIntent | null>(null);
  const [editedGoal, setEditedGoal] = React.useState<string | null>(null);
  const [editedDesc, setEditedDesc] = React.useState<string | null>(null);
  const [editing, setEditing] = React.useState(false);
  // null = 跟随系统推荐；"" = 用户明确选了"不指定"；其他 = 用户手选的分类 id
  const [catPick, setCatPick] = React.useState<string | null>(null);

  // 场景列表用于推荐。失败不影响主流程（只是不推荐场景），所以静默降级。
  React.useEffect(() => {
    api
      .taskCategories()
      .then((list) => setCategories(list))
      .catch(() => setCategories([]));
  }, []);

  // 输入变化即解析。这里不用 debounce：解析是纯本地同步计算（正则 + 数组过滤），
  // 几毫秒内完成，加延迟反而让"边打字边看到理解结果"变得迟钝。
  React.useEffect(() => {
    const t = text.trim();
    if (!t) {
      setParsed(null);
      setCatPick(null);
      return;
    }
    const p = parseIntent(t, categories);
    setParsed(p);
    // 用户尚未手工编辑时，跟着重新解析走（避免上次的编辑粘在新文本上）
    setCatPick(null);
    setEditedGoal(null);
    setEditedDesc(null);
  }, [text, categories]);

  const suggestedCategoryId = parsed?.suggestedCategoryId ?? null;
  // 场景取值的优先级：用户手选 > 系统推荐 > 无
  let categoryId: number | null = suggestedCategoryId;
  if (catPick !== null) categoryId = catPick === "" ? null : Number(catPick);
  const categoryOverridden = catPick !== null;
  // ToggleGroup 的受控值："跟随推荐"时要映射成推荐项的 id，没有推荐就是空串（全不选）
  let catValue = "";
  if (catPick !== null) catValue = catPick;
  else if (suggestedCategoryId !== null) catValue = String(suggestedCategoryId);
  // 场景那一行右侧的说明，按"当前最该说的一句"来选
  let sceneHint = "没识别到场景，可手选";
  if (categoryOverridden) sceneHint = "已按你选的场景";
  else if (suggestedCategoryId !== null) sceneHint = "系统推荐，可改";

  const finalGoal = editedGoal ?? parsed?.goal ?? "";
  const finalDesc = editedDesc ?? parsed?.description ?? "";
  const canCreate = text.trim().length > 0 && finalGoal.trim().length > 0 && !creating;
  const noAsset = parsed !== null && parsed.assets.length === 0;

  // 底部提示语按"当前最该说的一句"来选，不用嵌套三元堆在 JSX 里。
  let hint = "最小只需一句话";
  if (text.trim()) hint = "Ctrl/⌘ + Enter 快速创建";
  if (noAsset) hint = "补上测试对象后创建会更准";

  async function create() {
    if (!canCreate) return;
    setCreating(true);
    setError("");
    try {
      const task = await api.createTask({
        description: finalDesc.trim(),
        goal: finalGoal.trim(),
        categoryId: categoryId ?? undefined,
      });
      // 建完直接进任务会话页：用户要的是"看到 AI 开始干活"，不是回列表里找它
      router.push(`/function/tasks/detail?id=${encodeURIComponent(task.id)}`);
    } catch (e) {
      setError((e as Error).message || "创建失败");
      setCreating(false);
    }
  }

  return (
    <Card className="relative overflow-hidden border-primary/20 bg-gradient-to-br from-primary/[0.04] to-transparent">
      <CardContent className="flex flex-col gap-3 p-4">
        {/* 标题行 */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2">
            <SparklesIcon className="size-4 text-primary" />
            <div>
              <div className="font-medium text-sm">说一句话，让它自己干</div>
              <div className="text-[11px] text-muted-foreground">
                写清「测什么 + 想要什么结果」即可，其余配置由系统按场景自动带出
              </div>
            </div>
          </div>
          {onAdvanced && (
            <Button variant="ghost" size="sm" className="shrink-0 text-xs" onClick={onAdvanced}>
              高级配置
              <ArrowRightIcon className="size-3" />
            </Button>
          )}
        </div>

        {/* 输入框 */}
        <div className="relative">
          <Textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              // Ctrl/Cmd + Enter 直接开跑，符合"说完了就想让它动"的心理
              if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
                e.preventDefault();
                void create();
              }
            }}
            placeholder="例如：测试 example.com 的登录接口有没有 SQL 注入"
            className="min-h-20 resize-none bg-background/60 pr-3 text-sm"
            aria-label="描述你要做的事"
          />
        </div>

        {/* 示例 chips：只在没输入时出现，避免干扰 */}
        {!text.trim() && (
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-[11px] text-muted-foreground">试试：</span>
            {EXAMPLES.map((ex) => (
              <button
                key={ex}
                type="button"
                onClick={() => setText(ex)}
                className="rounded-full border border-border/60 bg-background/50 px-2.5 py-1 text-[11px] text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground"
              >
                {ex}
              </button>
            ))}
          </div>
        )}

        {/* 解析结果：只说系统真的推出来的东西，推不出就如实说 */}
        {parsed && (
          <div className="flex flex-col gap-2 rounded-md border border-border/50 bg-background/40 p-2.5">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-[11px]">
              <span className="flex items-center gap-1 text-muted-foreground">
                <TargetIcon className="size-3" />
                测试对象
              </span>
              {parsed.assets.length > 0 ? (
                parsed.assets.map((a) => (
                  <Badge key={a} variant="secondary" className="font-mono text-[10px]">
                    {a}
                  </Badge>
                ))
              ) : (
                <span className="text-amber-600 dark:text-amber-400">
                  没识别到域名/IP —— 建议补上，否则 agent 不知道测哪儿
                </span>
              )}
            </div>

            {/* 场景：推荐只是默认值。猜错了要能当场改掉，否则用户只能退到
                「高级配置」重填一遍，这个入口的意义就没了。再点一次已选中的项
                等于「不指定场景」。 */}
            {categories.length > 0 && (
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span className="text-[11px] text-muted-foreground">场景</span>
                <ToggleGroup
                  type="single"
                  value={catValue}
                  onValueChange={(v) => setCatPick(v)}
                  variant="outline"
                  size="sm"
                  spacing={0}
                  className="flex-wrap"
                  aria-label="任务场景"
                >
                  {categories.map((c) => (
                    <ToggleGroupItem key={c.id} value={String(c.id)} className="h-6 px-2 text-[11px]">
                      {c.name}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
                <span className="text-[10px] text-muted-foreground">{sceneHint}</span>
              </div>
            )}

            {/* 为什么推荐 —— 只在推荐还生效时解释；用户已经改过就不用再解释原推荐 */}
            {!categoryOverridden && parsed.suggestedReason && (
              <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground">
                <SparklesIcon className="size-3" />
                {parsed.suggestedReason}
                {parsed.matchedKeywords.length > 0 && (
                  <span className="text-[10px] opacity-70">（{parsed.matchedKeywords.slice(0, 4).join("、")}）</span>
                )}
              </div>
            )}

            {/* 可编辑的"理解结果" */}
            {editing ? (
              <div className="flex flex-col gap-2 pt-1">
                <label className="text-[11px] text-muted-foreground" htmlFor="intent-goal">
                  目标（agent 会按这个判断做完了没有）
                </label>
                <Textarea
                  id="intent-goal"
                  value={finalGoal}
                  onChange={(e) => setEditedGoal(e.target.value)}
                  className="min-h-16 resize-none bg-background/70 text-xs"
                />
                <label className="text-[11px] text-muted-foreground" htmlFor="intent-desc">
                  描述（背景与测试对象）
                </label>
                <Textarea
                  id="intent-desc"
                  value={finalDesc}
                  onChange={(e) => setEditedDesc(e.target.value)}
                  className="min-h-16 resize-none bg-background/70 text-xs"
                />
                <Button variant="ghost" size="sm" className="self-start text-xs" onClick={() => setEditing(false)}>
                  <CheckIcon className="size-3" />
                  改好了
                </Button>
              </div>
            ) : (
              <div className="flex flex-col gap-1">
                <div className="flex items-start gap-1.5 text-[11px]">
                  <span className="shrink-0 text-muted-foreground">目标</span>
                  <span className="min-w-0 flex-1 break-words">{finalGoal}</span>
                  <button
                    type="button"
                    onClick={() => setEditing(true)}
                    className="shrink-0 text-muted-foreground transition-colors hover:text-foreground"
                    aria-label="编辑理解结果"
                  >
                    <PencilIcon className="size-3" />
                  </button>
                </div>
              </div>
            )}
          </div>
        )}

        {/* 错误 */}
        {error && (
          <div className="flex items-start gap-1.5 rounded-md border border-destructive/30 bg-destructive/5 px-2.5 py-1.5 text-[11px] text-destructive">
            <XIcon className="mt-0.5 size-3 shrink-0" />
            <span className="break-words">{error}</span>
          </div>
        )}

        {/* 操作行 */}
        <div className="flex items-center justify-between gap-3">
          <span className="text-[10px] text-muted-foreground">{hint}</span>
          <Button
            size="sm"
            onClick={() => void create()}
            disabled={!canCreate}
            className={cn("shrink-0 gap-1.5", creating && "opacity-80")}
          >
            {creating ? <Loader2Icon className="size-3.5 animate-spin" /> : <ArrowRightIcon className="size-3.5" />}
            {creating ? "正在创建…" : "开跑"}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
