/**
 * not-found.tsx：404 页面，路由未匹配时展示「页面未找到」并提供返回首页的链接。
 * 主要函数/类型：
 *   - NotFound：渲染 404 提示与返回按钮
 */
"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";

export default function NotFound() {
  return (
    <div className="flex h-dvh flex-col items-center justify-center space-y-2 text-center">
      <h1 className="font-semibold text-2xl">页面未找到</h1>
      <p className="text-muted-foreground">你访问的页面不存在或已被移动。</p>
      <Link prefetch={false} replace href="/function/tasks">
        <Button variant="outline">返回任务列表</Button>
      </Link>
    </div>
  );
}
