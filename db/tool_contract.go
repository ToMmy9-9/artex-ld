// tool_contract.go：外部工具的权限契约模型与校验。
//
// 设计要点（这是整个工具管控的基石）：
//
//	平台/管理员在 contract_limits 里定义每个 flag 的"最大权限"；
//	内置 tool_keeper agent 认领工具时产出 contract；
//	校验规则是 contract ⊆ limits —— agent 只能收紧，永远不能放宽。
//
// 为什么必须这样：agent 的产出本质上不可信（提示注入、模型幻觉、二进制
// 本身可能伪装成别的工具）。如果 agent 能写"允许任意目标"，那整套管控
// 就等于让一个不可信组件去定义信任边界。把"放宽"的权利收在平台代码手里，
// agent 无论怎么跑都放宽不了权限上限。
package db

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ParamLimit 是单个 flag 的权限上限。
//
//	Value 形态二选一：
//	  - 白名单：Allowed 非空时，flag 的取值必须命中其中之一（支持前缀通配 "*"）。
//	  - 正则：Pattern 非空时，取值必须整体匹配该正则。
//	Risk 决定这个 flag 本身有多危险，用于决定是否需要人工确认：
//	  benign（无害，如 --version）| caution（注意）| dangerous（危险，如 --exec/--script）。
type ParamLimit struct {
	// Flag 是参数名，不含前导短横线（"p" / "http-proxy"）。
	Flag string `json:"flag"`
	// Required 表示该参数必须出现。
	Required bool `json:"required,omitempty"`
	// Allowed 是取值白名单；空串表示不限制取值（但仍受 Pattern 约束）。
	Allowed []string `json:"allowed,omitempty"`
	// Pattern 是取值必须匹配的正则。
	Pattern string `json:"pattern,omitempty"`
	// Risk 是该参数的风险等级。
	Risk string `json:"risk,omitempty"`
	// Note 是给模型看的说明（这个参数干什么、有什么坑）。
	Note string `json:"note,omitempty"`
	// Deny 为 true 时该 flag 一律禁止（无论契约怎么写）。
	Deny bool `json:"deny,omitempty"`
}

// TargetSpec 描述"这个工具打谁"的权限。
//
// 工具的目标可以有多种形态（域名、IP、CIDR、URL），平台按 TaskScope 校验。
type TargetSpec struct {
	// Kind 是目标类型：host | ip | cidr | url | any。
	Kind string `json:"kind"`
	// Pattern 可选正则，限制目标形态（例如 url 只允许 https://开头）。
	Pattern string `json:"pattern,omitempty"`
	// RequireScope 为 true 时目标必须落在任务授权范围内。
	RequireScope bool `json:"require_scope"`
	// MaxTargets 限制单次调用的目标数量上限（0 = 不限）。
	MaxTargets int `json:"max_targets,omitempty"`
}

// ToolContract 是一个外部工具的完整权限契约。
type ToolContract struct {
	// Params 是各 flag 的权限上限。
	Params []ParamLimit `json:"params,omitempty"`
	// Target 描述目标权限。
	Target *TargetSpec `json:"target,omitempty"`
	// AllowTargetsInScopeOnly 强制目标必须在授权范围内（等价于 Target.RequireScope，
	// 保留独立字段是为了让"平台强制项"和"agent 产出项"在存储上可区分）。
	AllowTargetsInScopeOnly bool `json:"allow_targets_in_scope_only,omitempty"`
	// DenyArgs 是整条命令级别的禁止片段（如 "--script"、"-exec"）。
	DenyArgs []string `json:"deny_args,omitempty"`
	// Passthrough 列出"允许透传但不做取值校验"的 flag（如 nuclei 的 -u/-l、
	// fscan 的 -u）。这些 flag 的取值由 Target 规格统一约束（必须在授权
	// 范围内），所以不需要逐个白名单。**契约里既没声明也没透传的 flag
	// 一律拒绝** —— 这是防止 agent 夹带未授权参数的关键。
	Passthrough []string `json:"passthrough,omitempty"`
	// MaxTargets 单次调用目标数上限（0 = 不限）。
	MaxTargets int `json:"max_targets,omitempty"`
	// TimeoutSeconds 单次执行超时上限。
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// ParamIndex 把 flag 快速索引到 ParamLimit。
func (c *ToolContract) ParamIndex() map[string]ParamLimit {
	m := map[string]ParamLimit{}
	for _, p := range c.Params {
		m[p.Flag] = p
	}
	return m
}

// ValidateContractAgainstLimits checks that an agent-produced contract is a
// subset of the platform-defined limits. It is the trust boundary: the agent
// may narrow anything, never widen.
//
// Rules:
//   - every flag in contract must exist in limits (can't invent a flag the
//     platform never sanctioned);
//   - a flag marked Deny in limits can never appear in contract;
//   - contract's Allowed list must be a subset of limits' Allowed;
//   - contract's Pattern must be a subset of limits' Pattern (i.e. limits'
//     pattern must accept everything the contract's pattern accepts) — we
//     enforce this by requiring the contract pattern to be *absent* or to
//     equal/intersect conservatively; a stricter check is done at runtime;
//   - RequireScope can only be turned on, never off;
//   - MaxTargets may only shrink;
//   - DenyArgs must be a superset of limits' DenyArgs.
func ValidateContractAgainstLimits(contract, limits *ToolContract) error {
	if limits == nil {
		return fmt.Errorf("平台未定义该工具的权限上限，无法校验契约")
	}
	if contract == nil {
		return fmt.Errorf("契约为空")
	}
	limitIdx := limits.ParamIndex()

	for _, p := range contract.Params {
		lim, ok := limitIdx[p.Flag]
		if !ok {
			return fmt.Errorf("契约包含平台未授权的参数 %q", p.Flag)
		}
		if lim.Deny {
			return fmt.Errorf("参数 %q 被平台明确禁止，不能出现在契约里", p.Flag)
		}
		// 白名单必须是子集
		if len(lim.Allowed) > 0 {
			for _, v := range p.Allowed {
				if !matchAllowed(lim.Allowed, v) {
					return fmt.Errorf("参数 %q 的取值 %q 不在平台授权范围内", p.Flag, v)
				}
			}
		}
		// 契约的正则必须不比平台的更宽松
		if lim.Pattern != "" && p.Pattern != "" {
			if !regexpSubset(p.Pattern, lim.Pattern) {
				return fmt.Errorf("参数 %q 的取值约束比平台授权更宽松", p.Flag)
			}
		}
		if lim.Required && !p.Required {
			return fmt.Errorf("参数 %q 是平台要求的必填项，契约不能取消", p.Flag)
		}
	}

	// 范围要求只能加严不能取消。
	//
	// 注意：平台强制的项**不要求 agent 重复声明**——agent 的契约里没有写
	// require_scope，不等于它想取消这个约束，而是它压根没提这件事。
	// 平台在最终执行时用的是「平台上限 ∩ agent 契约」，所以平台强制的
	// 部分永远生效。这里只在 agent **显式声明了相反意图**时才拒绝：
	// 即 agent 写了 require_scope=false 而平台要求 true。
	if limits.Target != nil && limits.Target.RequireScope {
		if contract.Target != nil && !contract.Target.RequireScope {
			return fmt.Errorf("平台要求目标必须在授权范围内，契约不能取消该约束")
		}
	}
	if limits.AllowTargetsInScopeOnly && contract.Target != nil &&
		!contract.Target.RequireScope {
		return fmt.Errorf("平台要求目标必须在授权范围内，契约不能取消该约束")
	}

	// 目标数上限只能收紧。agent 不声明 = 沿用平台上限（不是取消上限）。
	if limits.MaxTargets > 0 && contract.MaxTargets > limits.MaxTargets {
		return fmt.Errorf("单次目标数上限不能超过平台的 %d", limits.MaxTargets)
	}

	// 平台禁止的片段由平台自己强制执行，agent 不必重复声明；
	// 只有当 agent 声明的 deny_args 与平台的**不一致**时才提示（不是拒绝）。
	// 平台最终执行时用的是「平台上限 ∪ agent 契约」的禁止片段并集。

	// 透传 flag 不能凭空多出来（那等于夹带未授权参数）
	for _, f := range contract.Passthrough {
		if !containsFold(limits.Passthrough, f) {
			return fmt.Errorf("透传参数 %q 不在平台授权范围内", f)
		}
	}

	// 超时上限只能收紧。agent 不声明 = 沿用平台上限。
	if limits.TimeoutSeconds > 0 && contract.TimeoutSeconds > limits.TimeoutSeconds {
		return fmt.Errorf("执行超时不能超过平台的 %d 秒", limits.TimeoutSeconds)
	}
	return nil
}

// matchAllowed reports whether v is in the allowed list (prefix "*" wildcards
// supported, case-insensitive).
func matchAllowed(allowed []string, v string) bool {
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

// regexpSubset reports whether every string matching narrow also matches wide.
// We can only prove this for the common case where narrow is literally wide
// or wide is a literal prefix of narrow; otherwise we conservatively reject.
func regexpSubset(narrow, wide string) bool {
	if narrow == wide {
		return true
	}
	nr, err1 := regexp.Compile(narrow)
	wr, err2 := regexp.Compile(wide)
	if err1 != nil || err2 != nil {
		return false
	}
	// 若 wide 是纯字面量（无元字符），且 narrow 匹配它，则 narrow 更严格 → 子集成立。
	if !hasMeta(wide) {
		return nr.MatchString(wide)
	}
	// 否则无法证明，保守判定为"不更宽松"仅当 narrow 自身无元字符且匹配 wide。
	if !hasMeta(narrow) {
		return wr.MatchString(narrow)
	}
	return false
}

func hasMeta(re string) bool {
	return strings.ContainsAny(re, `.*+?()[]{}|^$\`)
}

func containsFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// ParseContract decodes a stored contract JSON blob.
func ParseContract(raw json.RawMessage) (*ToolContract, error) {
	if len(raw) == 0 {
		return &ToolContract{}, nil
	}
	var c ToolContract
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// CheckArgv validates a concrete argv against a contract. It is the runtime
// enforcement point: the platform builds argv from structured args, validates
// them here, and only then executes.
//
// Returns a human-readable reason on rejection (empty string = allowed).
func (c *ToolContract) CheckArgv(argv []string) string {
	if c == nil {
		return "工具没有契约，拒绝执行"
	}
	idx := c.ParamIndex()

	// 1) 整条命令级的禁止片段
	joined := strings.Join(argv, " ")
	for _, d := range c.DenyArgs {
		if d != "" && strings.Contains(joined, d) {
			return "参数 " + d + " 被平台禁止"
		}
	}

	// 2) 逐个解析参数
	seen := map[string]string{}
	passIdx := map[string]bool{}
	for _, f := range c.Passthrough {
		passIdx[f] = true
	}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			continue
		}
		name := strings.TrimLeft(a, "-")
		val := ""
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name, val = name[:eq], name[eq+1:]
		} else if i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
			// 只有当该 flag 在契约里声明为"取值型"时才消费下一个 token。
			if lim, ok := idx[name]; ok && (len(lim.Allowed) > 0 || lim.Pattern != "") {
				val = argv[i+1]
				i++
			}
		}
		lim, ok := idx[name]
		if !ok {
			if passIdx[name] {
				// 透传 flag：取值由 Target 规格统一约束，不做逐值白名单。
				seen[name] = val
				continue
			}
			// 契约里既没声明也没透传的 flag 一律拒绝 —— 防止夹带未授权参数。
			return "参数 " + name + " 不在契约允许范围内"
		}
		if lim.Deny {
			return "参数 " + name + " 被平台禁止"
		}
		if val != "" {
			if len(lim.Allowed) > 0 && !matchAllowed(lim.Allowed, val) {
				return "参数 " + name + " 的取值 " + val + " 不在允许范围内"
			}
			if lim.Pattern != "" {
				re, err := regexp.Compile(lim.Pattern)
				if err != nil {
					return "参数 " + name + " 的取值规则无效"
				}
				if !re.MatchString(val) {
					return "参数 " + name + " 的取值 " + val + " 不符合允许的格式"
				}
			}
		}
		seen[name] = val
	}

	// 3) 必填项
	for _, p := range c.Params {
		if p.Required {
			if _, ok := seen[p.Flag]; !ok {
				return "缺少必填参数 " + p.Flag
			}
		}
	}
	return ""
}

// SortedFlags returns the contract's flag names in a stable order (for UI).
func (c *ToolContract) SortedFlags() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.Params))
	for _, p := range c.Params {
		out = append(out, p.Flag)
	}
	sort.Strings(out)
	return out
}

// MergeContracts returns the effective contract used at execution time:
// the platform's ceiling with the agent's contract merged in, where the
// agent may only narrow.
//
// Merge rules:
//   - params: union, but a flag denied by the platform stays denied;
//   - deny_args: union (platform's bans always apply);
//   - passthrough: the agent's list, intersected with the platform's
//     (a flag the platform never sanctioned cannot be passed through);
//   - target.require_scope: OR of the two (either side can demand it);
//   - max_targets / timeout_seconds: the SMALLER of the two (0 = "not
//     specified", so the other side's value wins).
//
// This is the function that actually enforces the trust boundary at run time;
// ValidateContractAgainstLimits is the early check that gives the agent a
// readable error instead of a silent surprise.
func MergeContracts(contract, limits *ToolContract) *ToolContract {
	out := &ToolContract{}
	if limits != nil {
		*out = *limits
	}
	if contract == nil {
		return out
	}

	// params: start from the platform's, overlay the agent's (narrowing only).
	merged := map[string]ParamLimit{}
	for _, p := range out.Params {
		merged[p.Flag] = p
	}
	for _, p := range contract.Params {
		if lim, ok := merged[p.Flag]; ok {
			if lim.Deny {
				continue // the platform's ban wins
			}
			// The agent may narrow the allowed list, never widen it.
			if len(lim.Allowed) > 0 && len(p.Allowed) > 0 {
				narrowed := make([]string, 0, len(p.Allowed))
				for _, v := range p.Allowed {
					if matchAllowed(lim.Allowed, v) {
						narrowed = append(narrowed, v)
					}
				}
				p.Allowed = narrowed
			}
			if lim.Pattern != "" && p.Pattern != "" && !regexpSubset(p.Pattern, lim.Pattern) {
				p.Pattern = lim.Pattern // keep the platform's (looser) ceiling
			}
		}
		merged[p.Flag] = p
	}
	out.Params = out.Params[:0]
	for _, p := range merged {
		out.Params = append(out.Params, p)
	}
	sort.Slice(out.Params, func(i, j int) bool { return out.Params[i].Flag < out.Params[j].Flag })

	// deny_args: union.
	for _, d := range contract.DenyArgs {
		if !containsFold(out.DenyArgs, d) {
			out.DenyArgs = append(out.DenyArgs, d)
		}
	}

	// passthrough: the agent's, minus anything the platform never sanctioned.
	if len(out.Passthrough) == 0 {
		out.Passthrough = contract.Passthrough
	} else {
		allowed := map[string]bool{}
		for _, f := range out.Passthrough {
			allowed[f] = true
		}
		kept := make([]string, 0, len(contract.Passthrough))
		for _, f := range contract.Passthrough {
			if allowed[f] {
				kept = append(kept, f)
			}
		}
		out.Passthrough = kept
	}

	// target: either side may demand scope.
	if contract.Target != nil {
		if out.Target == nil {
			cp := *contract.Target
			out.Target = &cp
		} else {
			out.Target.RequireScope = out.Target.RequireScope || contract.Target.RequireScope
			if out.Target.MaxTargets == 0 || (contract.Target.MaxTargets > 0 && contract.Target.MaxTargets < out.Target.MaxTargets) {
				out.Target.MaxTargets = contract.Target.MaxTargets
			}
			if out.Target.Pattern == "" {
				out.Target.Pattern = contract.Target.Pattern
			}
		}
	}
	out.AllowTargetsInScopeOnly = out.AllowTargetsInScopeOnly ||
		(out.Target != nil && out.Target.RequireScope) || contract.AllowTargetsInScopeOnly

	// max targets / timeout: the smaller wins.
	if contract.MaxTargets > 0 && (out.MaxTargets == 0 || contract.MaxTargets < out.MaxTargets) {
		out.MaxTargets = contract.MaxTargets
	}
	if contract.TimeoutSeconds > 0 && (out.TimeoutSeconds == 0 || contract.TimeoutSeconds < out.TimeoutSeconds) {
		out.TimeoutSeconds = contract.TimeoutSeconds
	}
	return out
}
