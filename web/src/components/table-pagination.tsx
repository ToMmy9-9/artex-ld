/**
 * table-pagination.tsx：表格分页工具条，含页码窗口省略与每页条数选择。
 * 主要函数/类型：
 *   - TablePagination：分页条组件（page/pageSize/total 受控）
 *   - pageWindows：页码序列（含省略号）计算
 */
"use client";

import { useEffect, useRef } from "react";

import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Pagination, PaginationContent, PaginationEllipsis, PaginationItem } from "@/components/ui/pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

interface TablePaginationProps {
  page: number; // 1-based by default; see pageBase
  pageSize: number;
  total: number; // total (after filtering)
  onPageChange: (p: number) => void;
  onPageSizeChange: (s: number) => void;
  pageSizeOptions?: number[];
  // 页码基准。1 = 第 1 页是 1（多数页面）；0 = 第 1 页是 0（部分后端分页接口
  // 用 offset/limit 推导出来的 0-based 页码）。组件内部一律用 1-based 计算，
  // 只在对外读写时做偏移，避免每个页面自己写 ±1 的换算。
  pageBase?: 0 | 1;
}

// pageWindows returns the sequence of page numbers / ellipsis to render.
function pageWindows(page: number, total: number): (number | "...")[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1);
  const out: (number | "...")[] = [1];
  if (page > 3) out.push("...");
  const lo = Math.max(2, page - 1);
  const hi = Math.min(total - 1, page + 1);
  for (let i = lo; i <= hi; i++) out.push(i);
  if (page < total - 2) out.push("...");
  out.push(total);
  return out;
}

export function TablePagination({
  page,
  pageSize,
  total,
  onPageChange,
  onPageSizeChange,
  pageSizeOptions = [10, 20, 50],
  pageBase = 1,
}: TablePaginationProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  // 归一到 1-based 内部表示。
  const safePage = Math.min(Math.max(1, page - pageBase + 1), totalPages);
  const from = total === 0 ? 0 : (safePage - 1) * pageSize + 1;
  const to = Math.min(safePage * pageSize, total);
  const onPageChangeRef = useRef(onPageChange);
  onPageChangeRef.current = onPageChange;
  const baseRef = useRef(pageBase);
  baseRef.current = pageBase;

  useEffect(() => {
    if (page !== safePage) onPageChangeRef.current(safePage - 1 + baseRef.current);
  }, [page, safePage]);

  const go = (target: number) => onPageChange(target - 1 + pageBase);

  return (
    <div className="flex flex-wrap items-center justify-between gap-2 border-t px-4 py-2 lg:px-6">
      <div className="text-muted-foreground flex items-center gap-2 text-xs">
        <Select value={String(pageSize)} onValueChange={(v) => onPageSizeChange(Number(v))}>
          <SelectTrigger size="sm" className="h-7 w-16">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {pageSizeOptions.map((s) => (
              <SelectItem key={s} value={String(s)}>
                {s}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span>条/页</span>
        {total > 0 ? (
          <span className="tabular-nums">
            {from}–{to} / 共 {total} 条
          </span>
        ) : (
          <span>共 0 条</span>
        )}
      </div>

      {totalPages > 1 && (
        <Pagination className="mx-0 w-auto justify-end">
          <PaginationContent>
            <PaginationItem>
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={safePage === 1}
                onClick={() => go(safePage - 1)}
                aria-label="上一页"
              >
                <ChevronLeftIcon className="size-4" />
              </Button>
            </PaginationItem>
            {pageWindows(safePage, totalPages).map((p, i) =>
              p === "..." ? (
                <PaginationItem key={`el-${i}`}>
                  <PaginationEllipsis />
                </PaginationItem>
              ) : (
                <PaginationItem key={p}>
                  <Button
                    variant={p === safePage ? "outline" : "ghost"}
                    size="icon-sm"
                    onClick={() => go(p)}
                    aria-current={p === safePage ? "page" : undefined}
                  >
                    {p}
                  </Button>
                </PaginationItem>
              ),
            )}
            <PaginationItem>
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={safePage === totalPages}
                onClick={() => go(safePage + 1)}
                aria-label="下一页"
              >
                <ChevronRightIcon className="size-4" />
              </Button>
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      )}
    </div>
  );
}
