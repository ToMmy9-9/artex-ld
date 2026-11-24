// tool-assets-panel.tsx：外部工具（上传的攻防二进制）的可视化管理。
//
// 用户动作只有「上传」；其余（判定能力、风险、产出受管权限契约）由内置的
// 工具管理员 agent 自动完成。本面板负责展示它的结论，并允许管理员收紧
// 平台权限上限、隔离/驳回某个工具、查看执行审计。

"use client";

import * as React from "react";

import { AlertTriangle, Check, ChevronDown, ChevronRight, FileUp, Loader2, ShieldAlert, Trash2, X } from "lucide-react";
import { toast } from "sonner";

import { ConfirmDeleteDialog } from "@/components/confirm-delete-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { ToolAsset, ToolAssetContract, ToolAssetRun } from "@/lib/types";
import { fmtBytes } from "@/lib/utils";

const STATE_LABEL: Record<ToolAsset["state"], string> = {
  uploaded: "待认领",
  probing: "探测中",
  managed: "已认领",
  rejected: "已驳回",
  quarantined: "已隔离",
};

const STATE_CLASS: Record<ToolAsset["state"], string> = {
  uploaded: "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300",
  probing: "border-sky-500/40 bg-sky-500/10 text-sky-700 dark:text-sky-300",
  managed: "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  rejected: "border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-300",
  quarantined: "border-zinc-500/40 bg-zinc-500/10 text-zinc-700 dark:text-zinc-300",
};

const RISK_LABEL: Record<string, string> = {
  low: "低",
  medium: "中",
  high: "高",
  critical: "严重",
};

const RISK_CLASS: Record<string, string> = {
  low: "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  medium: "border-sky-500/40 bg-sky-500/10 text-sky-700 dark:text-sky-300",
  high: "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300",
  critical: "border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-300",
};

const SCOPE_MODE_LABEL: Record<string, string> = {
  enforced: "范围强制",
  advisory: "范围仅提示",
  none: "不涉网络",
};

const SCOPE_MODE_CLASS: Record<string, string> = {
  enforced: "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  advisory: "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300",
  none: "border-zinc-500/40 bg-zinc-500/10 text-zinc-700 dark:text-zinc-300",
};

const CATEGORY_LABEL: Record<string, string> = {
  scanner: "扫描器",
  brute: "爆破器",
  poc: "POC 框架",
  proxy: "代理",
  recon: "信息收集",
  crawler: "爬虫",
  utility: "通用工具",
};

function contractSummary(c: ToolAssetContract | undefined): string[] {
  if (!c) return [];
  const out: string[] = [];
  const params = (c.params ?? []).map((p) => p.flag);
  if (params.length) out.push(`允许参数 ${params.join(" / ")}`);
  const deny = c.deny_args ?? [];
  if (deny.length) out.push(`禁止 ${deny.join(" ")}`);
  if (c.allow_targets_in_scope_only) out.push("目标限授权范围");
  if (c.max_targets) out.push(`单次≤${c.max_targets} 目标`);
  if (c.timeout_seconds) out.push(`超时≤${c.timeout_seconds}s`);
  return out;
}

export function ToolAssetsPanel() {
  const [assets, setAssets] = React.useState<ToolAsset[]>([]);
  const [dir, setDir] = React.useState("");
  const [loading, setLoading] = React.useState(false);
  const [uploading, setUploading] = React.useState(false);
  const [expanded, setExpanded] = React.useState<number | null>(null);
  const [runsOf, setRunsOf] = React.useState<number | null>(null);
  const [runs, setRuns] = React.useState<ToolAssetRun[]>([]);
  const [limitsFor, setLimitsFor] = React.useState<ToolAsset | null>(null);
  const [limitsText, setLimitsText] = React.useState("");
  const fileRef = React.useRef<HTMLInputElement>(null);

  // 删除二次确认
  const [deleteTarget, setDeleteTarget] = React.useState<ToolAsset | null>(null);
  const [deleting, setDeleting] = React.useState(false);

  const reload = React.useCallback(async () => {
    setLoading(true);
    try {
      const r = await api.toolAssets();
      setAssets(r.assets ?? []);
      setDir(r.dir ?? "");
    } catch {
      setAssets([]);
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void reload();
  }, [reload]);

  const onUpload = React.useCallback(
    async (files: FileList | null) => {
      if (!files || files.length === 0) return;
      setUploading(true);
      try {
        const r = await api.uploadToolAssets(Array.from(files));
        const created = r.created ?? [];
        const rejected = r.rejected ?? [];
        if (created.length) {
          toast.success(`已上传 ${created.length} 个工具，工具管理员正在自动认领（判定能力与风险）`);
        }
        for (const rej of rejected) toast.error(`${rej.name}：${rej.reason}`);
        await reload();
      } catch (e) {
        toast.error(e instanceof Error ? e.message : "上传失败");
      } finally {
        setUploading(false);
        if (fileRef.current) fileRef.current.value = "";
      }
    },
    [reload],
  );

  const toggle = React.useCallback((id: number) => {
    setExpanded((cur) => (cur === id ? null : id));
  }, []);

  const loadRuns = React.useCallback(
    async (id: number) => {
      if (runsOf === id) {
        setRunsOf(null);
        return;
      }
      setRunsOf(id);
      setRuns([]);
      try {
        const r = await api.toolAssetRuns(id);
        setRuns(r.runs ?? []);
      } catch {
        setRuns([]);
      }
    },
    [runsOf],
  );

  const setState = React.useCallback(
    async (asset: ToolAsset, state: string) => {
      try {
        await api.setToolAssetState(asset.id, state);
        toast.success(`${asset.name} 已置为「${STATE_LABEL[state as ToolAsset["state"]]}」`);
        await reload();
      } catch (e) {
        toast.error(e instanceof Error ? e.message : "操作失败");
      }
    },
    [reload],
  );

  const remove = React.useCallback(
    async (asset: ToolAsset) => {
      setDeleting(true);
      try {
        await api.deleteToolAsset(asset.id);
        toast.success(`已删除 ${asset.name}`);
        setDeleteTarget(null);
        await reload();
      } catch (e) {
        toast.error(e instanceof Error ? e.message : "删除失败");
      } finally {
        setDeleting(false);
      }
    },
    [reload],
  );

  const openLimits = React.useCallback((asset: ToolAsset) => {
    setLimitsFor(asset);
    setLimitsText(JSON.stringify(asset.contract_limits ?? {}, null, 2));
  }, []);

  const saveLimits = React.useCallback(async () => {
    if (!limitsFor) return;
    let parsed: ToolAssetContract;
    try {
      parsed = JSON.parse(limitsText) as ToolAssetContract;
    } catch {
      toast.error("JSON 格式错误");
      return;
    }
    try {
      const r = await api.setToolAssetLimits(limitsFor.id, parsed);
      if (r.ok) toast.success("平台权限上限已更新");
      else toast.warning(r.note ?? r.reason ?? "已保存，但原契约不再合规");
      setLimitsFor(null);
      await reload();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "保存失败");
    }
  }, [limitsFor, limitsText, reload]);

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardContent className="flex flex-wrap items-center justify-between gap-3 pt-6">
          <div className="min-w-0">
            <div className="text-sm font-medium">上传外部工具</div>
            <div className="text-muted-foreground mt-1 text-xs">
              上传后由内置「工具管理员」自动认领：判定能力与风险、登记受管调用契约。 你不需要自己填参数规则。
            </div>
            {dir ? <div className="text-muted-foreground mt-1 truncate font-mono text-xs">受管目录 {dir}</div> : null}
          </div>
          <div className="flex items-center gap-2">
            <input
              ref={fileRef}
              type="file"
              multiple
              className="hidden"
              onChange={(e) => void onUpload(e.target.files)}
            />
            <Button onClick={() => fileRef.current?.click()} disabled={uploading} variant="outline" size="sm">
              {uploading ? (
                <Loader2 className="mr-1.5 size-3.5 animate-spin" />
              ) : (
                <FileUp className="mr-1.5 size-3.5" />
              )}
              选择文件上传
            </Button>
          </div>
        </CardContent>
      </Card>

      {loading ? (
        <div className="text-muted-foreground flex items-center gap-2 py-8 text-sm">
          <Loader2 className="size-4 animate-spin" /> 加载中…
        </div>
      ) : assets.length === 0 ? (
        <div className="text-muted-foreground rounded-md border border-dashed py-10 text-center text-sm">
          还没有上传任何外部工具
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          {assets.map((a) => {
            const open = expanded === a.id;
            const summary = contractSummary(a.contract);
            const limitSummary = contractSummary(a.contract_limits);
            return (
              <Card key={a.id}>
                <CardContent className="pt-5">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <button
                      type="button"
                      className="flex min-w-0 flex-1 items-start gap-2 text-left"
                      onClick={() => toggle(a.id)}
                    >
                      {open ? (
                        <ChevronDown className="text-muted-foreground mt-0.5 size-4 shrink-0" />
                      ) : (
                        <ChevronRight className="text-muted-foreground mt-0.5 size-4 shrink-0" />
                      )}
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium">{a.name}</span>
                          <Badge variant="outline" className={STATE_CLASS[a.state]}>
                            {STATE_LABEL[a.state]}
                          </Badge>
                          {a.risk_level ? (
                            <Badge variant="outline" className={RISK_CLASS[a.risk_level]}>
                              风险 {RISK_LABEL[a.risk_level] ?? a.risk_level}
                            </Badge>
                          ) : null}
                          {a.category ? (
                            <Badge variant="outline">{CATEGORY_LABEL[a.category] ?? a.category}</Badge>
                          ) : null}
                          <Badge variant="outline">{a.platform}</Badge>
                          {a.scope_enforcement ? (
                            <Badge
                              variant="outline"
                              className={SCOPE_MODE_CLASS[a.scope_enforcement] ?? ""}
                              title={
                                a.scope_enforcement === "advisory"
                                  ? "该工具不经过平台流量代理，平台无法拦截越界目标：越界只会警告并记入审计，不会阻止执行。"
                                  : a.scope_enforcement === "none"
                                    ? "该工具不作用于网络目标，无范围概念。"
                                    : "越界目标会被平台直接拒绝，工具不执行。"
                              }
                            >
                              {SCOPE_MODE_LABEL[a.scope_enforcement] ?? a.scope_enforcement}
                            </Badge>
                          ) : null}
                        </div>
                        {a.summary ? (
                          <div className="text-muted-foreground mt-1 text-xs">{a.summary}</div>
                        ) : a.state === "uploaded" ? (
                          <div className="text-muted-foreground mt-1 flex items-center gap-1 text-xs">
                            <Loader2 className="size-3 animate-spin" /> 工具管理员认领中…
                          </div>
                        ) : null}
                      </div>
                    </button>
                    <div className="flex shrink-0 items-center gap-1.5">
                      <span className="text-muted-foreground mr-1 text-xs">{fmtBytes(a.size_bytes)}</span>
                      {a.state === "managed" ? (
                        <>
                          <Button size="sm" variant="ghost" onClick={() => openLimits(a)}>
                            <ShieldAlert className="mr-1.5 size-3.5" />
                            权限上限
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => void setState(a, "quarantined")}>
                            隔离
                          </Button>
                        </>
                      ) : null}
                      {a.state === "quarantined" || a.state === "rejected" ? (
                        <Button size="sm" variant="ghost" onClick={() => void setState(a, "uploaded")}>
                          <Check className="mr-1.5 size-3.5" />
                          恢复
                        </Button>
                      ) : null}
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-destructive hover:text-destructive"
                        aria-label={`删除外部工具 ${a.name}`}
                        onClick={() => setDeleteTarget(a)}
                      >
                        <Trash2 className="size-3.5" />
                      </Button>
                    </div>
                  </div>

                  {open ? (
                    <div className="mt-4 flex flex-col gap-3 border-t pt-4">
                      <div className="grid gap-3 text-xs md:grid-cols-2">
                        <div>
                          <div className="text-muted-foreground mb-1">调用示例</div>
                          <div className="bg-muted/50 rounded px-2 py-1.5 font-mono break-all">
                            {a.usage_example || "—"}
                          </div>
                        </div>
                        <div>
                          <div className="text-muted-foreground mb-1">文件指纹</div>
                          <div className="bg-muted/50 rounded px-2 py-1.5 font-mono break-all">
                            sha256 {a.sha256.slice(0, 32)}…
                          </div>
                        </div>
                      </div>

                      <div className="grid gap-3 text-xs md:grid-cols-2">
                        <div className="rounded border p-2.5">
                          <div className="mb-1 font-medium">生效契约（工具管理员产出）</div>
                          {summary.length ? (
                            <ul className="text-muted-foreground flex flex-col gap-0.5">
                              {summary.map((s, i) => (
                                <li key={i}>· {s}</li>
                              ))}
                            </ul>
                          ) : (
                            <div className="text-muted-foreground">
                              {a.state === "managed" ? "未登记任何参数" : "待认领"}
                            </div>
                          )}
                        </div>
                        <div className="rounded border p-2.5">
                          <div className="mb-1 font-medium">平台权限上限（管理员可收紧）</div>
                          {limitSummary.length ? (
                            <ul className="text-muted-foreground flex flex-col gap-0.5">
                              {limitSummary.map((s, i) => (
                                <li key={i}>· {s}</li>
                              ))}
                            </ul>
                          ) : (
                            <div className="text-muted-foreground">—</div>
                          )}
                          <div className="text-muted-foreground mt-1.5 text-[11px]">
                            工具管理员只能在此范围内收紧，无法放宽。
                          </div>
                          {a.scope_enforcement === "advisory" ? (
                            <div className="mt-2 flex items-start gap-1.5 rounded border border-amber-500/40 bg-amber-500/10 px-2 py-1.5 text-[11px] text-amber-800 dark:text-amber-200">
                              <AlertTriangle className="mt-0.5 size-3 shrink-0" />
                              <span>
                                该工具不经过平台流量代理（原生 socket），
                                <strong>平台无法拦截它访问越界目标</strong>。 越界只会警告并记入审计，不会阻止执行。
                                {a.scope_suggestion === "advisory" ? "（平台建议此类别降级为「范围仅提示」）" : ""}
                              </span>
                            </div>
                          ) : null}
                        </div>
                      </div>

                      <div className="flex items-center gap-2">
                        <Button size="sm" variant="outline" onClick={() => void loadRuns(a.id)}>
                          {runsOf === a.id ? "收起执行记录" : "执行记录"}
                        </Button>
                        {a.managed_by ? (
                          <span className="text-muted-foreground text-xs">由 {a.managed_by} 认领</span>
                        ) : null}
                      </div>

                      {runsOf === a.id ? (
                        runs.length === 0 ? (
                          <div className="text-muted-foreground text-xs">还没有执行记录</div>
                        ) : (
                          <div className="flex flex-col gap-1">
                            {runs.map((r) => (
                              <div
                                key={r.id}
                                className="text-muted-foreground bg-muted/40 rounded px-2 py-1.5 font-mono text-[11px] break-all"
                              >
                                [{r.agent_key || "—"}] {r.tool_name} {r.argv.join(" ")}
                                <span className="ml-2">
                                  exit={r.exit_code} {r.duration_ms}ms
                                </span>
                              </div>
                            ))}
                          </div>
                        )
                      ) : null}
                    </div>
                  ) : null}
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      <ConfirmDeleteDialog
        open={deleteTarget !== null}
        onOpenChange={(o) => !o && setDeleteTarget(null)}
        title={`删除外部工具「${deleteTarget?.name ?? ""}」？`}
        description={
          deleteTarget
            ? `该二进制文件将从受管目录中移除，Agent 将无法再调用它，其调用记录与受管契约一并删除。此操作不可撤销。`
            : undefined
        }
        deleting={deleting}
        onConfirm={() => {
          if (deleteTarget) void remove(deleteTarget);
        }}
      />

      <Dialog open={!!limitsFor} onOpenChange={(o) => !o && setLimitsFor(null)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>平台权限上限 · {limitsFor?.name}</DialogTitle>
            <DialogDescription>
              这是该工具的权限天花板。工具管理员产出的契约只能在此范围内收紧；
              放宽上限需要你自己在这里改，并会立刻重新校验已有契约。
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Label htmlFor="limits-json">契约 JSON</Label>
            <Textarea
              id="limits-json"
              value={limitsText}
              onChange={(e) => setLimitsText(e.target.value)}
              className="min-h-72 font-mono text-xs"
              spellCheck={false}
            />
            <div className="text-muted-foreground flex items-start gap-1.5 text-xs">
              <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
              <span>收紧上限不会放宽任何已有权限；但若原契约超出新上限，平台会清空该契约并要求重新认领。</span>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setLimitsFor(null)}>
              取消
            </Button>
            <Button onClick={() => void saveLimits()}>保存上限</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
