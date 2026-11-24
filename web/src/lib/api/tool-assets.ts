// tool-assets.ts：外部工具资产的 API 封装。
//
// 用户上传攻防二进制 → 平台登记 → 内置工具管理员 agent 自动认领（判定能力、
// 风险并产出受管权限契约）。本文件只做数据搬运，管控逻辑全在后端。

import { del, get, post, uploadForm } from "@/lib/api/http";
import type { ToolAsset, ToolAssetContract, ToolAssetRun } from "@/lib/types";

export interface ToolAssetsApi {
  toolAssets: () => Promise<{ assets: ToolAsset[]; dir: string }>;
  uploadToolAssets: (files: File[]) => Promise<{
    created: Array<{
      id: number;
      name: string;
      file: string;
      size: number;
      sha256: string;
      platform: string;
      state: string;
      replaced?: boolean;
    }>;
    rejected: Array<{ name: string; reason: string }> | null;
  }>;
  deleteToolAsset: (id: number) => Promise<{ deleted: number }>;
  setToolAssetState: (id: number, state: string, note?: string) => Promise<{ id: number; state: string }>;
  setToolAssetLimits: (
    id: number,
    limits: ToolAssetContract,
  ) => Promise<{
    ok: boolean;
    note?: string;
    reason?: string;
  }>;
  toolAssetRuns: (id: number) => Promise<{ runs: ToolAssetRun[] }>;
}

export const toolAssetsApi: ToolAssetsApi = {
  toolAssets: () => get<{ assets: ToolAsset[]; dir: string }>("/tool-assets"),

  uploadToolAssets: (files: File[]) => {
    const form = new FormData();
    for (const f of files) form.append("file", f);
    // http() 的 JSON 封装不适合 multipart：用 uploadForm（FormData 的
    // Content-Type 必须由浏览器自己设，否则服务端解析失败）。
    return (async () => {
      return (await uploadForm<Awaited<ReturnType<ToolAssetsApi["uploadToolAssets"]>>>(
        "/api/tool-assets/upload",
        form,
      )) as Awaited<ReturnType<ToolAssetsApi["uploadToolAssets"]>>;
    })();
  },

  deleteToolAsset: (id: number) => del<{ deleted: number }>(`/tool-assets/${id}`),
  setToolAssetState: (id: number, state: string, note?: string) =>
    post<{ id: number; state: string }>(`/tool-assets/${id}/state`, { state, note }),
  setToolAssetLimits: (id: number, limits: ToolAssetContract) =>
    post<{ ok: boolean; note?: string; reason?: string }>(`/tool-assets/${id}/limits`, {
      contract_limits: limits,
    }),
  toolAssetRuns: (id: number) => get<{ runs: ToolAssetRun[] }>(`/tool-assets/${id}/runs`),
};
