/**
 * loading-indicator.tsx：加载态指示（首次加载用骨架/转圈，后台刷新用顶部进度条）。
 * 主要函数/类型：
 *   - LoadingIndicator：后台刷新指示器 —— 顶部一条进度线 + 内容区轻微降透明度。
 *   - RefreshingBar：极简顶部进度条，只在指定区域顶部显示。
 *
 * 为什么需要它：列表页「首次加载」和「筛选条件变化后重新加载」是两种不同的
 * 等待。以前很多页面只在首次加载时显示 Spinner，切筛选时表格保持旧数据、
 * 没有任何反馈 —— 用户会以为筛选没生效，再点一次。
 *
 * 后台刷新时不该把旧内容替换成骨架（那会造成内容闪烁），正确做法是：
 * 内容保持可见、整体轻微降透明度 + 顶部一条进度线，让用户知道「在更新」。
 */
"use client";

import type * as React from "react";

import { cn } from "@/lib/utils";

interface RefreshingBarProps {
  /** 是否正在刷新。 */
  active: boolean;
  className?: string;
}

/** RefreshingBar：容器顶部的进度线。刷新时从左向右扫过。 */
export function RefreshingBar({ active, className }: RefreshingBarProps) {
  if (!active) return null;
  return (
    <div
      className={cn("pointer-events-none absolute inset-x-0 top-0 z-20 h-0.5 overflow-hidden", className)}
      role="status"
      aria-label="正在刷新"
    >
      <div className="bg-primary/70 h-full w-1/3 animate-[loading-bar_1.1s_ease-in-out_infinite]" />
    </div>
  );
}

interface LoadingIndicatorProps {
  /** 是否处于后台刷新状态。 */
  refreshing: boolean;
  /** 内容区。刷新时自动降透明度。 */
  children: React.ReactNode;
  className?: string;
}

/**
 * LoadingIndicator 把「后台刷新」的状态表达清楚：
 * 顶部进度条 + 内容降透明度，而不是把内容换成骨架屏（那会闪烁）。
 */
export function LoadingIndicator({ refreshing, children, className }: LoadingIndicatorProps) {
  return (
    <div className={cn("relative", className)}>
      <RefreshingBar active={refreshing} />
      <div className={cn("transition-opacity duration-200", refreshing && "opacity-60")} aria-busy={refreshing}>
        {children}
      </div>
    </div>
  );
}
