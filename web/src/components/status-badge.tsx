/**
 * status-badge.tsx：跨状态域的通用状态徽章，依据 lib/status 元数据渲染色调与文案。
 * 主要函数/类型：
 *   - StatusBadge：状态徽章组件（domain + value，可选圆点）
 *   - ToneDot：只渲染语义色圆点的小组件（标签页计数等只需色点的位置）
 */

import { type StatusDomain, statusMeta, type Tone, toneClasses, toneDot } from "@/lib/status";
import { cn } from "@/lib/utils";

export function StatusBadge({
  domain,
  value,
  dot = false,
  className,
}: {
  domain: StatusDomain;
  value: string;
  dot?: boolean;
  className?: string;
}) {
  const meta = statusMeta(domain, value);
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        toneClasses[meta.tone],
        className,
      )}
    >
      {dot && <span className={cn("size-1.5 rounded-full", toneDot[meta.tone])} />}
      {meta.label}
    </span>
  );
}

// ToneDot 单独渲染一个语义色圆点，供只需要色点、不需要文案的位置使用。
export function ToneDot({ tone, className }: { tone: Tone; className?: string }) {
  return <span className={cn("size-1.5 rounded-full", toneDot[tone], className)} />;
}
