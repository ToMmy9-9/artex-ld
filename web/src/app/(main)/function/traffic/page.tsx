/**
 * page.tsx：流量记录页，分页浏览录制的 HTTP 交换，支持多条件筛选、排序及按主机/筛选/全部清理。
 * 主要函数/类型：
 *   - TrafficPage：页面主体，拉取 api.traffic 列表 / trafficHosts 主机清单 / trafficExchange 详情
 *   - requestWithHost：为缺少 Host 头的原始请求补上主机
 *   - MethodBadge / fmtBytes：方法徽标与字节数格式化
 */
"use client";

import * as React from "react";

import {
  ArrowDownWideNarrowIcon,
  ArrowUpNarrowWideIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  EraserIcon,
  FilterXIcon,
  ListChecksIcon,
  Loader2Icon,
  RadioTowerIcon,
  SearchIcon,
  Trash2Icon,
} from "lucide-react";
import { toast } from "sonner";

import { HttpCodeBlock } from "@/components/http-code-block";
import { LinkTrafficDialog } from "@/components/link-traffic-dialog";
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { SortableHead } from "@/components/ui/sortable-head";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { api } from "@/lib/api";
import { useStoredSortPreference } from "@/lib/sort-preference";
import type { TrafficDetail, TrafficExchange, TrafficHost, TrafficResp } from "@/lib/types";
import { cn, fmtTime, statusTone } from "@/lib/utils";

function fmtBytes(n: number) {
  if (n <= 0) return "0 B";
  // GB matters for the reclaimed-space figure a full purge reports; a capture-heavy
  // instance can hand back several.
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), units.length - 1);
  const v = n / 1024 ** i;
  return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`;
}

function MethodBadge({ method }: { method: string }) {
  return <Badge className="shrink-0 font-mono">{method}</Badge>;
}

// Older captures may predate Host persistence because net/http keeps Host
// outside Request.Header. Fill it for display while newly recorded traffic is
// fixed at the recorder layer as well.
function requestWithHost(raw: string, exchange: TrafficExchange): string {
  if (!raw.trim() || /^host\s*:/im.test(raw)) return raw;
  let host = exchange.host;
  try {
    host = new URL(exchange.url).host || host;
  } catch {
    // Relative or legacy URLs fall back to the indexed host.
  }
  const newline = raw.includes("\r\n") ? "\r\n" : "\n";
  const firstLineEnd = raw.indexOf(newline);
  if (firstLineEnd < 0) return `${raw}${newline}Host: ${host}`;
  return `${raw.slice(0, firstLineEnd + newline.length)}Host: ${host}${newline}${raw.slice(firstLineEnd + newline.length)}`;
}

// Fixed method set (server filters exact-match); avoids deriving options from a
// single page, which would only ever list the methods on that page.
const METHODS = ["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"];
const PAGE_SIZES = [25, 50, 100, 200];
type HostCountSortDirection = "asc" | "desc";

// Server-sortable columns. The list is sent to the backend verbatim as `sort`,
// which whitelists these same names, so keep them in sync with traffic.Page.
const SORT_FIELDS = ["ts", "status", "resp_len"] as const;
type SortField = (typeof SORT_FIELDS)[number];
const SORT_STORAGE_KEY = "traffic-sort";

// Status-class buckets for the filter dropdown; the value is sent as `status`,
// which the backend reads as either an exact code or an "Nxx" class band.
const STATUS_BUCKETS = ["2xx", "3xx", "4xx", "5xx"];
import { TrafficHistory } from "@/components/traffic-history";

export default function TrafficPage() {
  return <TrafficHistory />;
}
