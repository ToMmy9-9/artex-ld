/**
 * clickable-row.tsx：让 div 具备与 button 相同的键盘可达性。
 * 主要函数/类型：
 *   - ClickableRow：可点击的 div，自动补齐 role/tabIndex/键盘激活。
 *
 * 为什么需要它：技能树、Agent 列表这些地方的可点击行原本是裸 div + onClick，
 * **纯键盘用户完全无法操作**（Tab 键跳不过去，回车/空格也没反应）。
 *
 * 为什么不用 <button> 包裹：这些行内部还嵌套了 <Button>（新建文件、删除等），
 * button 套 button 是非法 HTML。改成 role="button" + tabIndex + 键盘处理，
 * 既不破坏嵌套结构，又补齐了可达性。
 *
 * 键盘行为遵循原生 button 约定：Enter 和 Space 都能激活。
 */
"use client";

import type * as React from "react";

import { cn } from "@/lib/utils";

interface ClickableRowProps extends React.ComponentProps<"div"> {
  /** 点击时触发的回调。键盘激活（Enter/Space）也会调用它。 */
  onActivate?: () => void;
  /** 无障碍标签。列表项建议填「xxx 的展开/切换」。 */
  ariaLabel?: string;
  /**
   * 渲染成什么元素。默认 div；行内 chip 场景用 span（div 是块级会破坏布局）。
   */
  as?: "div" | "span";
}

export function ClickableRow({
  onActivate,
  onClick,
  onKeyDown,
  ariaLabel,
  className,
  as = "div",
  ...props
}: ClickableRowProps) {
  const Comp = as;
  return (
    <Comp
      role="button"
      tabIndex={0}
      aria-label={ariaLabel}
      className={cn("focus-visible:ring-ring focus-visible:ring-2 focus-visible:outline-none", className)}
      onClick={onClick}
      onKeyDown={(event) => {
        // 先让调用方处理（可能 preventDefault），再补上激活行为。
        onKeyDown?.(event);
        if (event.defaultPrevented) return;
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault(); // Space 默认会滚动页面
          onActivate?.();
        }
      }}
      {...props}
    />
  );
}
