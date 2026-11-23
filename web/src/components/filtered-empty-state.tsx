/**
 * filtered-empty-state.tsx：筛选后无结果时的统一空状态。
 *
 * 背景：列表页筛选后为空，原来只有一句「没有匹配的流量。」/「没有匹配的发现。」，
 * 用户看完就卡住了 —— 想恢复只能自己逐个把筛选条件改回去。项目里已经有
 * components/ui/empty.tsx（任务复测页在用），但列表页没用，空状态风格也不统一。
 *
 * 这里把三件事收在一处：
 *   1. 统一的视觉（Empty 组件 + 图标 + 标题 + 说明）；
 *   2. 「清除筛选」按钮 —— 有筛选条件时才出现，直接一键复位；
 *   3. 「共 N 条」的真实总数 —— 让用户知道是「筛选没命中」还是「本来就没数据」。
 */
"use client";

import { FilterXIcon, InboxIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";

export interface FilteredEmptyStateProps {
  /** 空状态标题，默认「没有匹配的记录」。 */
  title?: string;
  /** 空状态说明。 */
  description?: string;
  /** 当前筛选命中的总数（未筛选时的总量）。用于区分「本来就没数据」和「筛没了」。 */
  total?: number;
  /** 是否存在生效中的筛选条件；为 false 时不显示「清除筛选」按钮。 */
  hasFilters?: boolean;
  /** 清除全部筛选条件。 */
  onClearFilters?: () => void;
  /** 清除筛选按钮文案。 */
  clearLabel?: string;
  className?: string;
}

/**
 * FilteredEmptyState 是「筛选无结果」的统一空状态。
 * 有筛选条件时给出「清除筛选」出口；没有筛选条件时说明数据本身就是空的，
 * 避免用户反复点「清除筛选」却什么都没变。
 */
export function FilteredEmptyState({
  title = "没有匹配的记录",
  description,
  total,
  hasFilters = false,
  onClearFilters,
  clearLabel = "清除筛选",
  className,
}: FilteredEmptyStateProps) {
  const filtered = hasFilters;
  return (
    <div className={className ?? "py-12"}>
      <Empty>
        <EmptyHeader>
          {filtered ? (
            <FilterXIcon className="size-8 text-muted-foreground" />
          ) : (
            <InboxIcon className="size-8 text-muted-foreground" />
          )}
          <EmptyTitle>{filtered ? title : "暂无数据"}</EmptyTitle>
        </EmptyHeader>
        <EmptyDescription>
          {description ??
            (filtered ? "当前筛选条件下没有匹配的记录，可以清除筛选条件后重新浏览。" : "这里还没有任何记录。")}
          {typeof total === "number" && total > 0 ? `（当前共 ${total} 条记录）` : ""}
        </EmptyDescription>
        {filtered && onClearFilters ? (
          <Button variant="outline" size="sm" onClick={onClearFilters} className="mt-2">
            <FilterXIcon data-icon="inline-start" />
            {clearLabel}
          </Button>
        ) : null}
      </Empty>
    </div>
  );
}
