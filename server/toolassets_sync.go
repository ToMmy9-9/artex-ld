// toolassets_sync.go：把已认领的外部工具接入 agent 的可调用集合。
//
// 缺口：工具上传并被认领后，只存在于 tool_assets 表里；agent 的工具目录
// （tools 表）并不知道它存在，于是既看不见也用不上 —— "管理起来了但用不了"。
//
// 这里把「已认领的工具资产」同步成 tools 表里的 shell 类工具：
//   - key  = ext_<工具名>
//   - exec = {dir: <受管目录>}，复用现成的目录扫描注入机制，把该目录下的
//            可执行文件清单注入 Bash 描述
//   - agents = 默认绑定到 worker / pentest（与现有 arsenal 工具一致）
//
// 同时注册 run_tool（受管执行入口）并绑定同样的 agent：外部工具应当走
// run_tool 而不是 Bash 自由文本，只有这样平台才知道实际执行了什么。
//
// 同步是幂等的：每次上传/认领后调用一次，已存在的行只更新描述与绑定。
package server

import (
	"encoding/json"
	"fmt"
	"log"

	"artex/db"
)

// extToolAgents are the agents that get access to uploaded external tools.
//
// worker and pentest mirror the existing arsenal binding: they are the
// agents that actually run tools. `auto` and `mainagent` cover the
// conversational surfaces, so a human can drive an uploaded tool by hand from
// the chat view or the task page. The planner deliberately does NOT get them
// — it dispatches intent, it does not execute.
var extToolAgents = []string{"worker", "pentest", "auto", "mainagent"}

// extToolKey builds the tools-table key for an external asset.
func extToolKey(name string) string { return "ext_" + name }

// extToolCatalogKey is the single shared entry that scans the managed
// directory and lists the binaries in it.
const extToolCatalogKey = "ext_catalog"

// syncToolAssetsToCatalog mirrors the managed-tool state into the tools
// catalog so agents can see and use the uploaded binaries.
//
// Only assets in state=managed are exposed: an unclaimed binary is not
// understood yet, and exposing it would let an agent run a tool whose
// capabilities nobody has looked at.
func (s *Server) syncToolAssetsToCatalog() {
	pg := s.m.pg
	if pg == nil {
		return
	}
	assets, err := pg.ListToolAssets()
	if err != nil {
		log.Printf("[tool-assets] 同步工具目录失败: %v", err)
		return
	}
	live := map[string]bool{}
	for _, a := range assets {
		if a.State != "managed" {
			continue
		}
		live[extToolKey(a.Name)] = true
		desc := a.Summary
		if desc == "" {
			desc = fmt.Sprintf("外部工具 %s（%s）", a.Name, a.Platform)
		}
		usage := ""
		if a.UsageExample != "" {
			usage = "\n\n示例：" + a.UsageExample
		}
		risk := ""
		if a.RiskLevel != "" {
			risk = "\n\n风险等级：" + a.RiskLevel
		}
		desc = desc + risk + usage +
			"\n\n这是已上传的外部工具。优先用 run_tool 以受管方式调用" +
			"（平台会校验参数与目标范围）；确需直接执行时用 Bash，并遵守上述示例。"

		// 范围执行模式必须让 agent 看见：advisory 的工具平台拦不住越界目标，
		// 如果不告诉 agent，它会以为调用完全受管控而放心地扩大扫描范围。
		mode := effectiveScopeEnforcement(a)
		switch mode {
		case db.ScopeAdvisory:
			desc += "\n\n⚠ 范围管控说明：该工具不经过平台流量代理，**平台无法拦截它访问授权范围之外的目标**。" +
				"调用它扫描目标时必须自己严格限定在任务授权范围内；越界不会被阻止，只会记入审计。"
		case db.ScopeNone:
			desc += "\n\n范围管控说明：该工具不作用于网络目标，无授权范围概念。"
		default:
			desc += "\n\n范围管控说明：平台会强制校验目标是否在任务授权范围内，越界将被拒绝。"
		}
		// Each asset gets its own catalogue entry carrying ITS description.
		// The exec.dir directory scan lives on a single shared entry (see
		// extToolCatalogKey) — pointing every entry at the same directory
		// would repeat the whole listing once per tool in the Bash prompt.
		if err := pg.UpsertSystemToolExclusive(extToolKey(a.Name), desc, json.RawMessage("{}"),
			nil, "shell", extToolAgents); err != nil {
			log.Printf("[tool-assets] 同步工具 %s 失败: %v", a.Name, err)
		}
	}
	// One shared directory-scan entry: it tells the model which binaries the
	// managed directory holds, so a freshly dropped file is visible without
	// touching the DB.
	catDesc := fmt.Sprintf("受管外部工具目录：%s（其中的可执行文件可直接用 Bash 调用；"+
		"但外部工具应优先用 run_tool 以受管方式执行）", s.toolAssetDir())
	catExec, _ := json.Marshal(map[string]any{"dir": s.toolAssetDir()})
	if err := pg.UpsertSystemToolExclusive(extToolCatalogKey, catDesc, json.RawMessage("{}"),
		catExec, "shell", extToolAgents); err != nil {
		log.Printf("[tool-assets] 同步工具目录清单失败: %v", err)
	}
	live[extToolCatalogKey] = true

	// Drop catalog entries for assets that are no longer managed (deleted,
	// quarantined, or reset to uploaded) so the catalogue never advertises a
	// tool that cannot be called.
	if err := pg.RemoveSystemToolsNotIn(live); err != nil {
		log.Printf("[tool-assets] 清理失效工具条目失败: %v", err)
	}
}

// ensureRunToolRegistered makes sure the managed-execution entry point exists
// in the tools catalog and is bound to the same agents as the external tools.
//
// Without this binding the tool would be dropped by ToolResolve (which keeps
// only tools bound to the calling agent), so run_tool would never reach an
// agent even though it is in the host tool set.
func (s *Server) ensureRunToolRegistered() {
	pg := s.m.pg
	if pg == nil {
		return
	}
	t := s.toolRunTool()
	schema, err := json.Marshal(t.InputSchema())
	if err != nil {
		log.Printf("[tool-assets] run_tool schema 序列化失败: %v", err)
		return
	}
	// run_tool is a callable platform tool, NOT a shell hint: its kind must be
	// builtin, otherwise it would also be appended to the Bash description as
	// if it were an installed command-line program.
	if err := pg.UpsertSystemToolExclusive("run_tool", t.Description(), schema,
		nil, "builtin", extToolAgents); err != nil {
		log.Printf("[tool-assets] 注册 run_tool 失败: %v", err)
	}
}

// toolAssetsSyncAll is the startup entry point.
func (s *Server) toolAssetsSyncAll() {
	s.ensureRunToolRegistered()
	s.syncToolAssetsToCatalog()
}

// assetDirectoryHint renders the managed directory listing for the Bash
// description of the ext_* tools. Kept separate so the same scan result can
// be reused by several catalogue entries.
func (s *Server) assetDirectoryHint() string {
	return s.toolAssetDir()
}

var _ = db.DefaultContractForCategory
