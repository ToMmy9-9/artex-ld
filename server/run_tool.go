// run_tool.go：受管执行入口 —— 外部工具真正跑起来的唯一通道。
//
// 为什么需要它：契约模型（平台上限 + agent 收紧）只有作用在"实际执行的
// argv"上才成立。如果 agent 还能用 Bash 自由文本拼命令行，平台就看不到
// 它到底执行了什么，契约只是一份展示品。
//
// run_tool 的流程：
//
//	结构化参数 → ①查工具资产 → ②合并契约（平台上限 ∩ agent 契约）
//	          → ③校验参数取值 → ④校验目标在授权范围内
//	          → ⑤平台拼 argv（agent 不能自己写命令行）
//	          → ⑥执行 → ⑦写审计
//
// 与 Bash 的分工：Bash 仍然可以跑通用命令（ls/cat/curl 等），但外部攻防
// 工具（gogo/fscan/nuclei 等已上传资产）走 run_tool —— 因为只有这里平台
// 才知道"哪个工具、什么参数、打什么目标"。
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"artex/agent"
	"artex/db"
	"norma/llm"
	actool "norma/tool"
)

// maxToolOutput caps how much stdout/stderr a managed run returns.
const maxToolOutput = 256 << 10 // 256 KiB

// maxToolTargets caps how many targets one managed call may carry.
const maxToolTargets = 500

// runToolArgv is the structured input of run_tool.
//
// The agent supplies named arguments, never a raw command line. The platform
// composes the argv from the tool's contract, so the agent cannot smuggle in
// a flag the contract does not sanction.
type runToolArgv struct {
	// Tool is the external tool's name (e.g. "gogo").
	Tool string `json:"tool"`
	// Args are the named arguments: {"p": "1-65535", "u": "10.0.0.1"}.
	// Keys are flag names WITHOUT the leading dash ("p", not "-p").
	Args map[string]string `json:"args,omitempty"`
	// Targets are the hosts/IPs/URLs this call acts on. The platform checks
	// each against the task's authorized scope.
	Targets []string `json:"targets,omitempty"`
	// TimeoutSeconds overrides the contract's timeout (never above it).
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// toolRunRequest is the internal form carrying the resolved task context.
type toolRunRequest struct {
	runToolArgv
	TaskID   int64
	AgentKey string
}

// toolRunTool builds the run_tool core tool.
//
// It is bound to agents that are allowed to use uploaded tools (the same
// binding that decides whether they see the tool catalogue at all), so a
// worker without tool access cannot invoke it.
func (s *Server) toolRunTool() actool.CoreTool {
	return wrToolCtx("run_tool",
		"以受管方式执行一个已上传的外部工具（gogo/fscan/nuclei 等）。\n\n"+
			"与 Bash 的区别：外部工具必须走这里，平台才会校验参数与目标；"+
			"Bash 跑的是通用命令，平台不保证其参数受工具契约约束。\n\n"+
			"参数：\n"+
			"- tool: 工具名（先用 list_tool_assets 查看可用的已认领工具）\n"+
			"- args: 参数字典，键是参数名且不带前导横线（如 {\"p\":\"1-65535\"}）\n"+
			"- targets: 本次要作用的目标列表（主机/IP/URL），必须在任务授权范围内\n\n"+
			"平台会：按工具契约校验每个参数取值 → 校验每个目标在授权范围内 → "+
			"拼装命令行执行 → 记录审计。越界的参数或目标会被直接拒绝。",
		objSchema(map[string]any{
			"tool": map[string]any{"type": "string", "description": "外部工具名。"},
			"args": map[string]any{
				"type":        "object",
				"description": "参数字典，键为参数名（不带前导横线）。",
				"additionalProperties": map[string]any{"type": "string"},
			},
			"targets": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "本次作用的目标（主机/IP/URL），必须在任务授权范围内。",
			},
			"timeout_seconds": map[string]any{"type": "integer", "description": "超时秒数（不得超过契约上限）。"},
		}, "tool"),
		func(ctx context.Context, in json.RawMessage, tc *actool.ToolContext) (actool.Result, error) {
			return s.runToolExec(ctx, in, tc)
		})
}

// runToolExec performs the managed execution.
func (s *Server) runToolExec(ctx context.Context, in json.RawMessage, tc *actool.ToolContext) (actool.Result, error) {
	pg := s.m.pg
	if pg == nil {
		return actool.Errorf("数据库不可用"), nil
	}
	var req runToolArgv
	if err := json.Unmarshal(in, &req); err != nil {
		return actool.Errorf("参数格式错误: "+err.Error()), nil
	}
	toolName := strings.TrimSpace(req.Tool)
	if toolName == "" {
		return actool.Errorf("缺少 tool 参数"), nil
	}
	info := agent.RunInfoFrom(ctx)
	taskID := info.TaskID
	agentKey := ""
	if tc != nil {
		agentKey = tc.AgentID
	}

	asset, err := pg.ToolAssetByName(toolName, "")
	if err != nil {
		return actool.Errorf(fmt.Sprintf("工具 %q 不存在或尚未上传", toolName)), nil
	}
	if asset.State != "managed" {
		return actool.Errorf(fmt.Sprintf("工具 %q 尚未被认领（当前状态 %s），无法调用", toolName, asset.State)), nil
	}

	// ① 合并契约：平台上限 ∩ agent 契约（取更严的一方）
	limits, err := db.ParseContract(asset.Limits)
	if err != nil {
		return actool.Errorf("平台权限上限解析失败: "+err.Error()), nil
	}
	contract, err := db.ParseContract(asset.Contract)
	if err != nil {
		return actool.Errorf("工具契约解析失败: "+err.Error()), nil
	}
	eff := db.MergeContracts(contract, limits)

	// ② 校验参数取值（用合并后的契约）
	argv, err := s.buildToolArgv(asset, eff, req)
	if err != nil {
		s.auditToolRejection(pg, asset, req, nil, taskID, agentKey, err.Error())
		return actool.Errorf(err.Error()), nil
	}

	// ③ 校验目标在授权范围内
	//
	// 被拒绝的调用同样要审计：一次「越界目标被拦下」恰恰是最需要留痕的事件，
	// 它说明要么 agent 判断错了，要么有人在试探边界。
	targets := req.Targets
	if len(targets) > maxToolTargets {
		s.auditToolRejection(pg, asset, req, argv, taskID, agentKey,
			fmt.Sprintf("目标数量超过上限 %d", maxToolTargets))
		return actool.Errorf(fmt.Sprintf("目标数量超过上限 %d", maxToolTargets)), nil
	}
	if eff.AllowTargetsInScopeOnly || (eff.Target != nil && eff.Target.RequireScope) {
		matcher, merr := s.taskScopeMatcher(taskID)
		if merr != nil {
			s.auditToolRejection(pg, asset, req, argv, taskID, agentKey,
				"范围校验失败: "+merr.Error())
			return actool.Errorf("范围校验失败: "+merr.Error()), nil
		}
		for _, t := range targets {
			if !matcher.InScope(t) {
				reason := fmt.Sprintf("目标 %q 不在本任务的授权范围内，已拒绝执行", t)
				s.auditToolRejection(pg, asset, req, argv, taskID, agentKey, reason)
				return actool.Errorf(reason), nil
			}
		}
	}
	if eff.MaxTargets > 0 && len(targets) > eff.MaxTargets {
		return actool.Errorf(fmt.Sprintf("目标数量 %d 超过契约上限 %d", len(targets), eff.MaxTargets)), nil
	}

	// ④ 执行
	timeout := eff.TimeoutSeconds
	if timeout <= 0 {
		timeout = 300
	}
	if req.TimeoutSeconds > 0 && req.TimeoutSeconds < timeout {
		timeout = req.TimeoutSeconds
	}
	out, exitCode, runErr := s.execManagedTool(ctx, asset, argv, timeout)

	// ⑤ 审计（无论成功失败都记）
	argvJSON, _ := json.Marshal(argv)
	targetsJSON, _ := json.Marshal(targets)
	_, _ = pg.InsertToolAssetRun(&db.ToolAssetRun{
		AssetID:  asset.ID,
		TaskID:   taskID,
		AgentKey: agentKey,
		ToolName: asset.Name,
		Argv:     argvJSON,
		Targets:  targetsJSON,
		ExitCode: exitCode,
	})

	if runErr != nil {
		return actool.Errorf(fmt.Sprintf("执行失败（exit=%d）：%v\n输出：%s", exitCode, runErr, out)), nil
	}
	msg := fmt.Sprintf("[%s] exit=%d\n%s", strings.Join(argv, " "), exitCode, out)
	return actool.Result{Content: []llm.ContentBlock{llm.TextBlock(msg)}}, nil
}

// buildToolArgv composes the command line from structured args.
//
// The agent never supplies a raw command line: it supplies a map of flag →
// value, and the platform decides the flag order and the shape. Flags that the
// contract does not sanction are rejected rather than silently dropped.
func (s *Server) buildToolArgv(asset *db.ToolAsset, eff *db.ToolContract, req runToolArgv) ([]string, error) {
	idx := eff.ParamIndex()
	pass := map[string]bool{}
	for _, f := range eff.Passthrough {
		pass[f] = true
	}

	// Validate every supplied argument against the contract.
	keys := make([]string, 0, len(req.Args))
	for k := range req.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := req.Args[k]
		lim, ok := idx[k]
		if !ok {
			if pass[k] {
				continue
			}
			return nil, fmt.Errorf("参数 %q 不在 %s 的契约允许范围内（契约只授权了这些参数：%v）",
				k, asset.Name, eff.SortedFlags())
		}
		if lim.Deny {
			return nil, fmt.Errorf("参数 %q 被平台禁止", k)
		}
		if len(lim.Allowed) > 0 && !matchAllowedList(lim.Allowed, v) {
			return nil, fmt.Errorf("参数 %q 的取值 %q 不在允许范围内（允许：%v）", k, v, lim.Allowed)
		}
		if lim.Pattern != "" {
			if re, err := regexp.Compile(lim.Pattern); err != nil {
				return nil, fmt.Errorf("参数 %q 的取值规则无效", k)
			} else if !re.MatchString(v) {
				return nil, fmt.Errorf("参数 %q 的取值 %q 不符合允许的格式", k, v)
			}
		}
	}

	// Compose argv: binary + sorted flags + targets.
	path, perr := s.toolAssetPath(asset)
	if perr != nil {
		return nil, perr
	}
	argv := []string{path}
	for _, k := range keys {
		v := req.Args[k]
		if v == "" {
			argv = append(argv, "-"+k)
			continue
		}
		argv = append(argv, "-"+k, v)
	}
	argv = append(argv, req.Targets...)

	// Final gate: the composed command line must itself satisfy the
	// contract. This catches anything the per-argument check missed (e.g.
	// a deny fragment hidden in a target string).
	if reason := eff.CheckArgv(argv); reason != "" {
		return nil, fmt.Errorf("命令未通过契约校验: %s", reason)
	}
	return argv, nil
}

// execManagedTool runs the composed argv with a timeout and bounded output.
func (s *Server) execManagedTool(ctx context.Context, asset *db.ToolAsset, argv []string, timeout int) (string, int, error) {
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = os.TempDir()
	// Managed runs inherit the proxy environment so HTTP-speaking tools route
	// through the platform's capture proxy (and therefore through the
	// traffic interception rules). Native-socket tools ignore these — that
	// limitation is documented, not silently pretended away.
	cmd.Env = os.Environ()

	var buf strings.Builder
	lim := &limitedWriter{w: &buf, limit: maxToolOutput}
	cmd.Stdout = lim
	cmd.Stderr = lim

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return buf.String(), exitCode, err
}

// taskScopeMatcher builds the authorized-scope matcher for a task.
func (s *Server) taskScopeMatcher(taskID int64) (*db.ScopeMatcher, error) {
	if taskID <= 0 {
		return db.EmptyScopeMatcher(), nil
	}
	pg := s.m.pg
	if pg == nil {
		return db.EmptyScopeMatcher(), fmt.Errorf("数据库不可用")
	}
	as := pg.Assets()
	if as == nil {
		return db.EmptyScopeMatcher(), fmt.Errorf("资产库不可用")
	}
	rows, err := as.ListTaskScope(taskID)
	if err != nil {
		return nil, err
	}
	// 关联公司的范围也计入授权边界：任务范围里 kind='company' 的行只是
	// 引用，真正的 root_domain/ip/cidr 在 company_scope 表里。
	companyScopes := map[int64][]db.TaskScope{}
	cs := pg.Companies()
	if cs != nil {
		for _, r := range rows {
			if r.CompanyID == nil || r.Kind != "company" {
				continue
			}
			if _, ok := companyScopes[*r.CompanyID]; ok {
				continue
			}
			rules, cerr := cs.GetScope(*r.CompanyID)
			if cerr != nil {
				continue
			}
			companyScopes[*r.CompanyID] = scopeRulesToTaskScope(rules)
		}
	}
	return db.BuildScopeMatcher(rows, companyScopes), nil
}

// matchAllowedList mirrors db's whitelist matching for server-side use.
func matchAllowedList(allowed []string, v string) bool {
	lower := strings.ToLower(v)
	for _, a := range allowed {
		a = strings.ToLower(a)
		if a == "*" {
			return true
		}
		if strings.HasSuffix(a, "*") && strings.HasPrefix(lower, strings.TrimSuffix(a, "*")) {
			return true
		}
		if a == lower {
			return true
		}
	}
	return false
}

// toolAssetDirName is the display name of the managed directory.
func (s *Server) toolAssetDirName() string { return filepath.Base(s.toolAssetDir()) }

// scopeRulesToTaskScope converts company scope rules into the TaskScope shape
// the matcher consumes, so both layers are judged by the same code.
func scopeRulesToTaskScope(rules []db.ScopeRule) []db.TaskScope {
	out := make([]db.TaskScope, 0, len(rules))
	for _, r := range rules {
		out = append(out, db.TaskScope{
			Kind:   r.Kind,
			Domain: r.Domain,
			Net:    r.Net,
			Value:  r.Value,
		})
	}
	return out
}

// auditToolRejection records a call that was refused before execution.
//
// The rejected argv is stored alongside the reason so the trail shows both
// what the agent tried and why the platform stopped it.
func (s *Server) auditToolRejection(pg *db.DB, asset *db.ToolAsset, req runToolArgv,
	argv []string, taskID int64, agentKey, reason string) {
	if pg == nil || asset == nil {
		return
	}
	stored := argv
	if len(stored) == 0 {
		// The argv was never composed (the rejection happened while building
		// it); record the requested args instead so the attempt is visible.
		stored = []string{asset.Name}
		keys := make([]string, 0, len(req.Args))
		for k := range req.Args {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			stored = append(stored, "-"+k, req.Args[k])
		}
	}
	argvJSON, _ := json.Marshal(stored)
	targetsJSON, _ := json.Marshal(req.Targets)
	_, _ = pg.InsertToolAssetRun(&db.ToolAssetRun{
		AssetID:  asset.ID,
		TaskID:   taskID,
		AgentKey: agentKey,
		ToolName: asset.Name,
		Argv:     argvJSON,
		Targets:  targetsJSON,
		ExitCode: -1,
	})
	// The reason goes into the argv trail as a trailing marker so a plain
	// argv dump still shows why the run stopped.
	_, _ = pg.Exec(`UPDATE tool_asset_runs SET argv = argv || $2::jsonb
		WHERE id = (SELECT max(id) FROM tool_asset_runs WHERE asset_id=$1)`,
		asset.ID, []byte(`["__rejected__:`+reason+`"]`))
}
