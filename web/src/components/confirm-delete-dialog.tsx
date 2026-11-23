/**
 * confirm-delete-dialog.tsx：统一的「删除类」二次确认弹窗。
 *
 * 项目里删除操作散落在十几个页面里，确认方式却各不相同：有的一删就没（无二次确认），
 * 有的用浏览器原生 window.confirm（样式突兀、无法说明影响范围），有的自带一个
 * 写死的 AlertDialog。本组件把这些收敛到同一套交互：
 *
 *   - 标题必须带上要删的实体名（删除拦截规则「xxx」？），避免用户在长列表里点错行；
 *   - 描述必须说明「删了之后会怎样」（命中该规则的命令将不再被拦截……），
 *     而不只是「此操作不可撤销」；
 *   - 可选的条目清单预览（items）沿用任务页批量删除的样式：
 *     max-h-40 的滚动区 + chip 标签 + 「…等 N 个」；
 *   - 确认按钮是 destructive 变体，删除进行中禁用并显示「删除中…」。
 *
 * 组件只负责「问」，不负责「删」：调用方在 onConfirm 里发请求、toast 结果，
 * 并通过 deleting 把进行中状态回传。
 */
"use client";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Spinner } from "@/components/ui/spinner";

export interface ConfirmDeleteDialogProps {
  /** 弹窗是否打开。 */
  open: boolean;
  /** 打开状态变化；删除进行中应由调用方忽略 false。 */
  onOpenChange: (open: boolean) => void;
  /** 标题，要含实体名，如「删除拦截规则「xxx」？」 */
  title: string;
  /** 描述：说明影响范围与条数，而不是只写「此操作不可撤销」。 */
  description?: string;
  /** 将被删除的条目清单（显示前 30 个 + 「…等 N 个」）。 */
  items?: string[];
  /** 确认按钮文案，默认「确认删除」。 */
  confirmText?: string;
  /** 进行中（禁用按钮 + 显示「删除中…」）。 */
  deleting?: boolean;
  /** 点击确认。 */
  onConfirm: () => void;
  /** 删除失败时的回调（用于 toast.error）。 */
  onError?: (msg: string) => void;
}

const PREVIEW_LIMIT = 30;

/**
 * 给清单条目生成不重复的 React key。
 *
 * 条目名可能重名（例如两个 Agent 同名），直接拿字符串当 key 会撞车，
 * React 会告警「Encountered two children with the same key」。这里按出现次数
 * 加后缀，既保持 key 稳定（同一个条目每次渲染拿到的 key 一样），又不会重复。
 */
function uniqueKeys(items: string[]): string[] {
  const seen = new Map<string, number>();
  return items.map((item) => {
    const n = seen.get(item) ?? 0;
    seen.set(item, n + 1);
    return n === 0 ? item : `${item}#${n}`;
  });
}

export function ConfirmDeleteDialog({
  open,
  onOpenChange,
  title,
  description,
  items,
  confirmText = "确认删除",
  deleting = false,
  onConfirm,
  onError,
}: ConfirmDeleteDialogProps) {
  const list = items ?? [];
  const preview = list.slice(0, PREVIEW_LIMIT);
  const hidden = list.length - preview.length;
  const previewKeys = uniqueKeys(preview);

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        // 删除进行中不允许关掉弹窗：否则用户会以为删掉了，界面却还停在原地。
        if (deleting && !next) return;
        onOpenChange(next);
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description ? <AlertDialogDescription>{description}</AlertDialogDescription> : null}
        </AlertDialogHeader>

        {/* 列出将被删除的条目：任务页的批量删除已有这个清单预览，资产页沿用了同一套样式。 */}
        {preview.length > 0 ? (
          <div className="flex max-h-40 flex-wrap gap-1 overflow-y-auto text-muted-foreground text-xs">
            {preview.map((item, i) => (
              <code key={previewKeys[i]} className="rounded bg-muted px-1.5 py-0.5 font-mono" title={item}>
                {item}
              </code>
            ))}
            {hidden > 0 && <span className="self-center">…等 {list.length} 个</span>}
          </div>
        ) : null}

        <AlertDialogFooter>
          <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={deleting}
            onClick={(e) => {
              // preventDefault：默认行为会立刻关闭弹窗，删除失败时用户就看不到上下文了。
              e.preventDefault();
              try {
                onConfirm();
              } catch (err) {
                onError?.(err instanceof Error ? err.message : String(err));
              }
            }}
          >
            {deleting && <Spinner data-icon="inline-start" />}
            {deleting ? "删除中…" : confirmText}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
