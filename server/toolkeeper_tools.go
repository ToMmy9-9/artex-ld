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
			// agents can see and call it, and tell the running tasks about it
			// so a worker mid-run learns about it without waiting for its next
			// tool-set re-assembly.
			s.syncToolAssetsToCatalog()
			if claimed, cerr := pg.ToolAssetByID(req.AssetID); cerr == nil {
				s.notifyToolAvailable(claimed)
			}
			msg := fmt.Sprintf("已登记 %s（分类=%s 风险=%s）。契约只会作为平台上限的子集生效。",
				asset.Name, req.Category, req.RiskLevel)
			return actool.Result{Content: []llm.ContentBlock{llm.TextBlock(msg)}}, nil
		})
}


// toolPrepareEnv prepares a tool's runtime environment by downloading a
// release archive and extracting it into the managed directory.
//
// 为什么给 keeper 而不是给 worker：环境准备是"认领"这件事的一部分——一个
// 上传的 exe 缺依赖、缺字典、缺模板时，是 keeper 在探测阶段发现它跑不起来，
// 也只有 keeper 有能力判断该补哪个包。worker 不该有往宿主磁盘写文件的权限。
//
// 安全边界写在 toolenv.go 顶部：只下载与解压，永不执行。
func (s *Server) toolPrepareEnv() actool.CoreTool {
	return wrTool("prepare_tool_env",
		"为一个外部工具准备运行环境：从给定 URL 下载发布包（zip/tar.gz/tar/gz/裸二进制），"+
			"解压到平台的受管工具目录，并自动登记成工具资产（随后走正常认领流程）。\n\n"+
			"用途：上传的工具只是个 exe，但缺少依赖 DLL、字典、POC 模板、配置文件时，"+
			"用这个把完整发布包装进来。\n\n"+
			"重要限制：\n"+
			"- **只下载与解压，绝不执行任何内容**（不跑 install/setup/postinst/.bat），"+
			"所以带安装脚本的包不会自动安装，只会把文件放好。\n"+
			"- 只会写入平台的受管工具目录，不会写到系统目录或用户目录。\n"+
			"- 下载地址不做白名单，但每次准备都会完整记录（URL、哈希、文件数、谁要求的），可追溯。",
		objSchema(map[string]any{
			"tool_name": map[string]any{"type": "string",
				"description": "工具名（小写字母数字与 . _ -）。会成为受管目录下的子目录名与工具资产名。"},
			"url": map[string]any{"type": "string",
				"description": "发布包的 http/https 下载地址。支持 zip / tar.gz / tar / gz / 裸二进制。"},
			"entry_bin": map[string]any{"type": "string",
				"description": "可选。包内主可执行文件的相对路径（如 gogo.exe 或 bin/fscan）。留空则平台按名字自动识别。"},
			"claim": map[string]any{"type": "boolean",
				"description": "可选。准备成功后是否立即唤醒工具管理员认领。默认 true。"},
		}, "tool_name", "url"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var req toolEnvPrepRequest
			if err := json.Unmarshal(in, &req); err != nil {
				return actool.Errorf("参数格式错误: " + err.Error()), nil
			}
			// 默认自动认领：环境准备的目的是让工具可用，停在 uploaded
			// 还需要人再点一次，等于没省事。用指针区分"没传"与"传了 false"。
			claim := true
			if v, ok := boolField(in, "claim"); ok {
				claim = v
			}
			req.Claim = claim
			res, err := s.toolEnvPrep(ctx, req, toolKeeperAgentKey)
			if err != nil {
				return actool.Errorf("环境准备失败: " + err.Error()), nil
			}
			b, _ := json.Marshal(res)
			return actool.Result{Content: []llm.ContentBlock{llm.TextBlock(string(b))}}, nil
		})
}

// boolField reports whether key was present as a JSON boolean, and its value.
func boolField(in json.RawMessage, key string) (val, ok bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal(in, &m) != nil {
		return false, false
	}
	raw, exists := m[key]
	if !exists {
		return false, false
	}
	if json.Unmarshal(raw, &val) != nil {
		return false, false
	}
	return val, true
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
		s.toolPrepareEnv(),
	}
}
