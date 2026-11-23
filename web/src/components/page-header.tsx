/**
 * page-header.tsx：页面标题区（标题 + 副标题 + 右侧操作区）。
 * 主要函数/类型：
 *   - PageHeader：统一的页面标题区。标题固定 text-xl/font-semibold/tracking-tight，
 *     副标题固定 text-sm/muted-foreground，右侧可放操作按钮、Tab 等。
 *
 * 为什么有这个组件：全站曾有 10 种标题写法（text-xl / text-lg / text-2xl、
 * font-semibold 前后顺序不一、tracking-tight 时有时无）。标题是每个页面的第一
 * 印象，样式不统一会让整个产品显得像拼凑出来的 —— 尤其在页面之间来回切换时，
 * 标题会「跳一下」。这里把主流写法（13 个页面在用）固化下来。
 */
import type * as React from "react";

import { cn } from "@/lib/utils";

interface PageHeaderProps {
  title: React.ReactNode;
  description?: React.ReactNode;
  /** 右侧操作区：按钮、Tab、状态标签等。 */
  actions?: React.ReactNode;
  className?: string;
  /** 紧凑模式：用于卡片内/抽屉内的次级标题。 */
  size?: "default" | "sm";
  /** 标题左侧的图标。图标尺寸跟着标题走（默认 h-5 w-5，紧凑模式 h-4 w-4）。 */
  icon?: React.ReactNode;
}

export function PageHeader({ title, description, actions, className, size = "default", icon }: PageHeaderProps) {
  return (
    <div className={cn("flex flex-wrap items-start justify-between gap-3", className)}>
      <div className="flex min-w-0 items-start gap-2.5">
        {icon ? (
          <span
            className={cn(
              "text-muted-foreground mt-0.5 shrink-0",
              size === "default" ? "[&_svg]:size-5" : "[&_svg]:size-4",
            )}
          >
            {icon}
          </span>
        ) : null}
        <div className="min-w-0">
          <h1 className={cn("font-semibold tracking-tight", size === "default" ? "text-xl" : "text-sm")}>{title}</h1>
          {description ? (
            <p className={cn("text-muted-foreground mt-0.5", size === "default" ? "text-sm" : "text-xs")}>
              {description}
            </p>
          ) : null}
        </div>
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </div>
  );
}
