// toolkeeper_tools.go：内置「工具管理员」agent（tool_keeper）的专用工具。
//
// 这两个工具只绑定给 tool_keeper，不进通用工具集：
//
//	probe_tool —— 受限探测上传的二进制。平台自己拼 argv（只允许
//	              --help/-h/--version 这类只读参数），agent 不能自己写命令，
//	              所以探测行为不受提示注入影响。
//	claim_tool —— 登记认领结论。契约会被平台校验：只能是权限上限的子集。
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"artex/db"
	"norma/llm"
	"norma/permission"
	actool "norma/tool"
)

// toolProbeTool exposes the whitelisted usage probe.
//
// The agent picks WHICH binary and WHICH usage flag; it never composes an
// arbitrary command line. The platform runs the binary with a fixed argv
// grammar and no proxy environment, so the probe can neither be pointed at a
// network target nor inherit the agent's egress path.
func (s *Server) toolProbeTool() actool.CoreTool {
	return roTool("probe_tool",
		"探测一个已上传的外部工具的用法。只能传 --help/-h/--version 这类只读参数（平台强制白名单，无法指定目标或执行脚本）。返回该工具的用法输出。",
		objSchema(map[string]any{
			"asset_id": map[string]any{"type": "integer", "description": "要探测的工具资产 id（先用 list_tool_assets 查）。"},
			"flag":     map[string]any{"type": "string", "description": "只读探测参数：--help / -h / --version / version。"},
		}, "asset_id", "flag"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var req struct {
				AssetID int64  `json:"asset_id"`
				Flag    string `json:"flag"`
			}
			if err := json.Unmarshal(in, &req); err != nil {
				return actool.Errorf("参数格式错误"), nil
			}
			pg := s.m.pg
			if pg == nil {
				return actool.Errorf("数据库不可用"), nil
			}
			asset, err := pg.ToolAssetByID(req.AssetID)
			if err != nil {
				return actool.Errorf("工具不存在"), nil
			}
			if !probeAllowArgs[req.Flag] {
				return actool.Errorf("探测参数不被允许，只能用 --help/-h/--version/version 这类只读参数"), nil
			}
			// Only binaries inside the managed directory may be probed.
			path, perr := s.toolAssetPath(asset)
			if perr != nil {
				return actool.Errorf(perr.Error()), nil
			}
			out, pErr := s.ProbeToolBinaryWithFlag(path, req.Flag)
			res := map[string]any{"asset_id": asset.ID, "name": asset.Name, "output": out}
			if pErr != nil {
				res["note"] = pErr.Error()
			}
			b, _ := json.Marshal(res)
			return actool.Result{Content: []llm.ContentBlock{llm.TextBlock(string(b))}}, nil
		})
}

// toolListToolAssets lets the keeper see what it is managing.
func (s *Server) toolListToolAssets() actool.CoreTool {
	return roTool("list_tool_assets",
		"列出已上传的外部工具资产：id、名称、平台、大小、sha256、当前状态（uploaded=待认领 / managed=已认领 / rejected=已驳回 / quarantined=已隔离）、以及认领结论（分类/风险/用法/契约）。",
		objSchema(map[string]any{}),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			pg := s.m.pg
			if pg == nil {
				return actool.Errorf("数据库不可用"), nil
			}
			assets, err := pg.ListToolAssets()
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			out := make([]map[string]any, 0, len(assets))
			for _, a := range assets {
				out = append(out, map[string]any{
					"id": a.ID, "name": a.Name, "file": a.DisplayName, "platform": a.Platform,
					"size": a.SizeBytes, "state": a.State, "category": a.Category,
					"risk_level": a.RiskLevel, "summary": a.Summary,
					"usage_example": a.UsageExample,
				})
			}
			b, _ := json.Marshal(map[string]any{"assets": out})
			return actool.Result{Content: []llm.ContentBlock{llm.TextBlock(string(b))}}, nil
		})
}

// toolClaimTool records the keeper's conclusion about a tool.
func (s *Server) toolClaimTool() actool.CoreTool {
	return wrTool("claim_tool",
		"登记对一个外部工具的认领结论：分类、风险等级、一句话说明、调用示例，以及权限契约。"+
			"契约里的参数白名单只能是平台上限的**子集**（只能收紧不能放宽），"+
			"平台会强制校验；放宽的契约会被直接拒绝。",
		objSchema(map[string]any{
			"asset_id":      map[string]any{"type": "integer", "description": "工具资产 id。"},
			"category":      map[string]any{"type": "string", "description": "scanner/brute/poc/proxy/recon/crawler/utility 之一。"},
			"risk_level":    map[string]any{"type": "string", "description": "low/medium/high/critical 之一。"},
			"summary":       map[string]any{"type": "string", "description": "一句话说明这个工具干什么、有什么风险。"},
			"usage_example": map[string]any{"type": "string", "description": "给模型看的调用示例（完整命令行）。"},
			"contract": map[string]any{
				"type":        "object",
				"description": "权限契约。params 是各参数的上限，target 是目标权限约束，deny_args 是禁止片段，passthrough 是允许透传但取值由 target 统一约束的参数。",
				"properties": map[string]any{
					"params": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"flag":     map[string]any{"type": "string", "description": "参数名，不带前导横线。"},
								"required": map[string]any{"type": "boolean", "description": "是否必填。"},
								"allowed":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "取值白名单（支持前缀 * 通配）。"},
								"pattern":  map[string]any{"type": "string", "description": "取值必须匹配的正则。"},
								"risk":     map[string]any{"type": "string", "description": "benign/caution/dangerous。"},
								"note":     map[string]any{"type": "string", "description": "给模型看的说明。"},
							},
						},
					},
					"target": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"kind":          map[string]any{"type": "string", "description": "host/ip/cidr/url/any。"},
							"pattern":       map[string]any{"type": "string", "description": "目标形态约束（可选正则）。"},
							"require_scope": map[string]any{"type": "boolean", "description": "目标是否必须在任务授权范围内。"},
							"max_targets":   map[string]any{"type": "integer", "description": "单次目标数上限。"},
						},
					},
					"allow_targets_in_scope_only": map[string]any{"type": "boolean", "description": "强制目标在授权范围内。"},
					"deny_args":                   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "禁止出现在命令行里的片段。"},
					"passthrough":                 map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "允许透传、取值由 target 统一约束的参数名。"},
					"max_targets":                 map[string]any{"type": "integer", "description": "单次目标数上限。"},
					"timeout_seconds":             map[string]any{"type": "integer", "description": "单次执行超时上限（秒）。"},
				},
			},
		}, "asset_id", "category", "risk_level", "summary"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			pg := s.m.pg
			if pg == nil {
				return actool.Errorf("数据库不可用"), nil
			}
			var req toolAssetClaim
			if err := json.Unmarshal(in, &req); err != nil {
				return actool.Errorf("参数格式错误: "+err.Error()), nil
			}
			asset, err := pg.ToolAssetByID(req.AssetID)
			if err != nil {
				return actool.Errorf("工具不存在"), nil
			}
			// 认领时若给出了类别，平台按该类别重算一份更贴合的上限：认领前
			// 上限是通用的 utility 版（宽松），认领出"这是扫描器"之后，
			// 上限应该收紧到扫描器该有的样子。agent 依旧只能在此之上收紧。
			limits := asset.Limits
			if req.Category != "" {
				fresh := db.DefaultContractJSON(req.Category)
				if err := pg.SetToolAssetLimits(req.AssetID, fresh); err == nil {
					limits = fresh
				}
			}
			if req.Contract != nil {
				parsed, lerr := db.ParseContract(limits)
				if lerr != nil {
					return actool.Errorf("平台权限上限解析失败: "+lerr.Error()), nil
				}
				if verr := db.ValidateContractAgainstLimits(req.Contract, parsed); verr != nil {
					return actool.Errorf("契约未通过平台校验: "+verr.Error()+
						"（平台上限：params="+fmt.Sprint(parsed.SortedFlags())+
						", deny_args="+strings.Join(parsed.DenyArgs, ",")+"）"), nil
				}
			}
			if req.Contract != nil {
				raw, merr := json.Marshal(req.Contract)
				if merr != nil {
					return actool.Errorf("契约序列化失败"), nil
				}
				if err := pg.SetToolAssetContract(req.AssetID, raw); err != nil {
					return actool.Errorf(err.Error()), nil
				}
			}
			if err := pg.UpdateToolAssetMeta(req.AssetID, req.Category, req.RiskLevel,
				req.Summary, req.UsageExample, req.ProbeOutput, "tool_keeper"); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			// The asset is now managed: publish it into the tool catalogue so
			// agents can see and call it.
			s.syncToolAssetsToCatalog()
			msg := fmt.Sprintf("已登记 %s（分类=%s 风险=%s）。契约只会作为平台上限的子集生效。",
				asset.Name, req.Category, req.RiskLevel)
			return actool.Result{Content: []llm.ContentBlock{llm.TextBlock(msg)}}, nil
		})
}


// wrToolCtx is wrTool with the ToolContext passed through, for tools that
// need the working dir / agent id (e.g. audit attribution).
func wrToolCtx(name, desc string, schema map[string]any, run func(context.Context, json.RawMessage, *actool.ToolContext) (actool.Result, error)) actool.CoreTool {
	return actool.Build(actool.Spec{
		Name: name, Description: desc, Schema: schema,
		Permissions: func(context.Context, json.RawMessage, permission.Context) permission.Decision {
			return permission.Allowed()
		},
		Run: func(ctx context.Context, in json.RawMessage, tc *actool.ToolContext) (actool.Result, error) {
			return run(ctx, in, tc)
		},
	})
}

// toolKeeperTools is the tool set bound to the internal tool_keeper agent.
func (s *Server) toolKeeperTools() []actool.CoreTool {
	return []actool.CoreTool{
		s.toolListToolAssets(),
		s.toolProbeTool(),
		s.toolClaimTool(),
	}
}
