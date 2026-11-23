/**
 * select-all-control.tsx：列表页「全选」统一控件。
 *
 * 背景：发现 / 任务 / 资产三个列表页都需要「勾选后批量操作（导出、删除）」，
 * 但各自实现过一次，交互不一致：有的只有裸勾选框（只能选当前页），有的才有
 * 下拉菜单（全选全部筛选结果）。这里抽成公共组件，三个页面共用一套交互。
 *
 * 两级选择语义：
 *   - 表头勾选框：只作用于**当前页**，翻页不清空（选中集合是跨页保留的）。
 *   - 下拉「全部筛选结果」：一次性拉取当前筛选命中的全部 id 填进选中集合，
 *     用于「选中 → 导出」这类需要全量数据的场景。
 *
 * 注意条数口径：列表的 total 常是「合并后的行数」（如发现的档案数），与原始
 * 记录数不是一个口径。所以菜单**不预告条数**，选中后由页面的「已选 N 条」与
 * toast 报真实条数，避免用户点下去发现数字对不上。
 */
"use client";

import { ChevronDownIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Spinner } from "@/components/ui/spinner";

export interface SelectAllControlProps {
  /** 当前页可选中的 id 列表（已过滤掉不可选的项）。 */
  pageIds: string[];
  /** 整个选中集合（跨页保留）。 */
  selectedIds: Set<string>;
  /** 批量设置当前页的选中状态。 */
  onToggleSelectedPage: (ids: string[], checked: boolean) => void;
  /** 「全选全部筛选结果」：拉取全量 id 并回填选中集合。 */
  onSelectAllMatching?: () => void;
  /** 全量选择进行中（用于禁用入口防重复点击）。 */
  selectingAllMatching?: boolean;
  /** 清空整个选中集合。 */
  onClearSelection?: () => void;
  /** 表头勾选框的无障碍标签，各页面措辞不同。 */
  selectAllLabel?: string;
  /** 是否显示下拉菜单（不需要跨页全选的场景可关掉）。 */
  showMenu?: boolean;
}

/**
 * SelectAllControl 是表头全选控件：勾选框 + 下拉菜单。
 * 下拉提供「本页全部 / 全部筛选结果 / 全不选」三项。
 */
export function SelectAllControl({
  pageIds,
  selectedIds,
  onToggleSelectedPage,
  onSelectAllMatching,
  selectingAllMatching = false,
  onClearSelection,
  selectAllLabel = "选择当前页全部",
  showMenu = true,
}: SelectAllControlProps) {
  const selectedCount = pageIds.filter((id) => selectedIds.has(id)).length;
  let headerChecked: boolean | "indeterminate" = false;
  if (pageIds.length > 0 && selectedCount === pageIds.length) {
    headerChecked = true;
  } else if (selectedCount > 0) {
    headerChecked = "indeterminate";
  }

  return (
    <div className="flex items-center gap-0.5">
      <Checkbox
        checked={headerChecked}
        onCheckedChange={(checked) => onToggleSelectedPage(pageIds, checked === true)}
        aria-label={selectAllLabel}
      />
      {showMenu && onSelectAllMatching ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              className="size-5 shrink-0 text-muted-foreground"
              aria-label="更多全选方式"
            >
              <ChevronDownIcon className="size-3" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-60">
            <DropdownMenuLabel>选择范围</DropdownMenuLabel>
            <DropdownMenuItem disabled={pageIds.length === 0} onClick={() => onToggleSelectedPage(pageIds, true)}>
              本页全部（{pageIds.length} 条）
            </DropdownMenuItem>
            <DropdownMenuItem disabled={selectingAllMatching} onClick={onSelectAllMatching}>
              {selectingAllMatching ? "正在选择…" : "全部筛选结果"}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            {/* 「全不选」与报告页的「全选 / 全不选」成对按钮保持一致：
                全选之后总要给一条清空的路，否则只能一个个取消勾选。 */}
            <DropdownMenuItem disabled={selectedIds.size === 0} onClick={onClearSelection}>
              全不选（{selectedIds.size} 条）
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </div>
  );
}

/**
 * SelectAllToolbarButton 是工具栏上的「全选」按钮，给没有表头勾选框的视图
 * （如发现的「按漏洞」卡片列表）用。菜单语义与表头下拉一致。
 */
export function SelectAllToolbarButton({
  onSelectAllMatching,
  onClearSelection,
  selectedCount,
  selectingAllMatching = false,
}: {
  onSelectAllMatching: () => void;
  onClearSelection?: () => void;
  selectedCount: number;
  selectingAllMatching?: boolean;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="outline" disabled={selectingAllMatching}>
          {selectingAllMatching ? <Spinner /> : null}
          全选
          <ChevronDownIcon className="size-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel>选择范围</DropdownMenuLabel>
        <DropdownMenuItem disabled={selectingAllMatching} onClick={onSelectAllMatching}>
          全选全部筛选结果
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem disabled={selectedCount === 0} onClick={onClearSelection}>
          全不选{selectedCount > 0 ? `（${selectedCount} 条）` : ""}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
