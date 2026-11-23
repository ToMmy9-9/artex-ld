/**
 * audit-records.tsx：审计日志列表——登录/平台操作留痕，支持分类/动作/结果筛选与分页。
 * 主要函数/类型：
 *   - AuditRecords：审计日志表格组件（筛选、分页、结果徽标）
 *   - fmtTime：时间戳格式化
 *   - resultBadge：结果（success/fail）对应的徽标样式
 */
"use client";

import * as React from "react";

import { Loader2Icon, RefreshCwIcon, ScrollTextIcon, SearchIcon } from "lucide-react";

import { FilteredEmptyState } from "@/components/filtered-empty-state";
import { TablePagination } from "@/components/table-pagination";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api } from "@/lib/api";
import type { AuditLog } from "@/lib/types";

const PAGE_SIZE = 50;

function fmtTime(value: string) {
  if (!value) return "—";
  const d = new Date(value);
  if (isNaN(d.getTime())) return value;
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

function resultBadge(result: string) {
  if (result === "fail") {
    return (
      <Badge variant="destructive" className="text-[10px]">
        失败
      </Badge>
    );
  }
  return (
    <Badge variant="secondary" className="text-[10px]">
      成功
    </Badge>
  );
}

export function AuditRecords() {
  const [logs, setLogs] = React.useState<AuditLog[]>([]);
  const [total, setTotal] = React.useState(0);
  const [page, setPage] = React.useState(1);
  const [loading, setLoading] = React.useState(false);
  // 后端不可用时日志列表会变空 —— 不给错误提示的话，用户会以为「本来就没有
  // 审计日志」，而不是「读取失败」。这会把故障伪装成空数据。
  const [error, setError] = React.useState("");
  const [category, setCategory] = React.useState("all");
  const [action, setAction] = React.useState("all");
  const [result, setResult] = React.useState("all");
  const [q, setQ] = React.useState("");
  const [qApplied, setQApplied] = React.useState("");
  const [reloadTick, setReloadTick] = React.useState(0);

  React.useEffect(() => {
    const t = setTimeout(() => setQApplied(q.trim()), 300);
    return () => clearTimeout(t);
  }, [q]);

  React.useEffect(() => {
    let alive = true;
    setLoading(true);
    api
      .auditLogs({ category, action, result, q: qApplied, page, limit: PAGE_SIZE })
      .then((r) => {
        if (!alive) return;
        setLogs(r.logs ?? []);
        setTotal(r.total ?? 0);
        setError("");
      })
      .catch((e) => {
        if (!alive) return;
        setLogs([]);
        setTotal(0);
        setError((e as Error)?.message ?? "未知错误");
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [category, action, result, qApplied, page, reloadTick]);

  // 筛选条件变化后回到第一页，否则可能停在一个已不存在的页码上（看到空列表）。
  React.useEffect(() => {
    setPage(1);
  }, [category, action, result, qApplied]);

  // 生效中的筛选条件：分类 / 动作 / 结果任一非「全部」，或搜索框有内容。
  const hasFilters =
    category !== "all" || action !== "all" || result !== "all" || qApplied.trim() !== "";
  const clearFilters = React.useCallback(() => {
    setCategory("all");
    setAction("all");
    setResult("all");
    setQ("");
    setQApplied("");
  }, []);

  // 空单元格：加载中 / 出错 / 无匹配，三种状态互斥。出错时不能显示
  // 「没有匹配的记录」——那会把故障说成空数据，用户以为本来就没日志。
  let emptyCell: React.ReactNode;
  if (loading) {
    emptyCell = <div className="py-10 text-center text-muted-foreground text-sm">加载中…</div>;
  } else if (error) {
    emptyCell = (
      <div className="py-10 text-center text-muted-foreground text-sm">日志读取失败，请刷新重试。</div>
    );
  } else {
    emptyCell = (
      <FilteredEmptyState
        title="没有匹配的审计记录"
        description="当前筛选条件下没有审计记录，可以清除筛选条件后重新浏览。"
        total={total}
        hasFilters={hasFilters}
        onClearFilters={clearFilters}
      />
    );
  }

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-4">
      <Card>
        <CardHeader className="flex-row flex-wrap items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <ScrollTextIcon className="size-4" />
            审计日志
          </CardTitle>
          <Button variant="ghost" size="sm" onClick={() => setReloadTick((n) => n + 1)} disabled={loading}>
            {loading ? <Loader2Icon className="size-4 animate-spin" /> : <RefreshCwIcon className="size-4" />}
            刷新
          </Button>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <Select value={category} onValueChange={setCategory}>
              <SelectTrigger className="w-36">
                <SelectValue placeholder="分类" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部分类</SelectItem>
                <SelectItem value="auth">登录认证</SelectItem>
              </SelectContent>
            </Select>
            <Select value={action} onValueChange={setAction}>
              <SelectTrigger className="w-44">
                <SelectValue placeholder="动作" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部动作</SelectItem>
                <SelectItem value="login">登录</SelectItem>
                <SelectItem value="login_failed">登录失败</SelectItem>
                <SelectItem value="login_locked">锁定</SelectItem>
                <SelectItem value="init">初始化密码</SelectItem>
                <SelectItem value="change_password">修改密码</SelectItem>
              </SelectContent>
            </Select>
            <Select value={result} onValueChange={setResult}>
              <SelectTrigger className="w-32">
                <SelectValue placeholder="结果" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部结果</SelectItem>
                <SelectItem value="success">成功</SelectItem>
                <SelectItem value="fail">失败</SelectItem>
              </SelectContent>
            </Select>
            <div className="relative min-w-48 flex-1">
              <SearchIcon className="pointer-events-none absolute left-2 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="搜索消息或 IP"
                className="pl-8"
              />
            </div>
          </div>

          {error ? (
            <Alert variant="destructive">
              <AlertDescription>审计日志加载失败：{error}。请点击刷新重试。</AlertDescription>
            </Alert>
          ) : null}

          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-44">时间</TableHead>
                  <TableHead className="w-28">分类</TableHead>
                  <TableHead className="w-40">动作</TableHead>
                  <TableHead className="w-24">结果</TableHead>
                  <TableHead className="w-36">来源 IP</TableHead>
                  <TableHead>说明</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {logs.map((l) => (
                  <TableRow key={l.id}>
                    <TableCell className="text-xs text-muted-foreground tabular-nums">{fmtTime(l.created_at)}</TableCell>
                    <TableCell className="text-xs">{l.category || "—"}</TableCell>
                    <TableCell className="font-mono text-xs">{l.action || "—"}</TableCell>
                    <TableCell>{resultBadge(l.result)}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{l.ip || "—"}</TableCell>
                    <TableCell className="text-xs">{l.message || "—"}</TableCell>
                  </TableRow>
                ))}
                {logs.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={6} className="p-0">
                      {emptyCell}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>

          <TablePagination
            page={page}
            pageSize={PAGE_SIZE}
            total={total}
            onPageChange={setPage}
            onPageSizeChange={() => setPage(1)}
          />
        </CardContent>
      </Card>
    </div>
  );
}
