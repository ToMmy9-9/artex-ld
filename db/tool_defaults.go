// tool_defaults.go：外部工具的默认权限上限。
//
// 为什么需要它：契约模型是「平台定上限、agent 只能收紧」。如果平台不给出
// 任何上限（空契约），agent 产出的任何参数都会被判为"平台未授权"而拒绝——
// 工具就永远处于「已上传但不可用」的状态，默认上限让上传即用。
//
// 默认上限的原则：**按能力类别给出宽松但有边界的权限**。
//   - 通用小工具（utility）：几乎不限制，但仍然禁止执行脚本/读写任意路径；
//   - 扫描器（scanner）：允许指定目标与端口，但目标必须落在授权范围内；
//   - 爆破器（brute）：允许字典，但限制并发/速率相关的开关；
//   - POC 框架（poc）：只允许已知模板名，禁止本地路径与脚本执行；
//   - 代理（proxy）：限制监听端口与上游地址。
//
// 管理员可以在 UI 上随时收紧（POST /api/tool-assets/{id}/limits）。
package db

import "encoding/json"

// DefaultContractForCategory returns the platform's default permission ceiling
// for a tool of the given category. The agent may narrow any of it.
//
// category 为空时按 utility 处理（最宽松但仍有边界）。
func DefaultContractForCategory(category string) *ToolContract {
	switch category {
	case "scanner":
		return &ToolContract{
			// 默认强制范围管控。原生 socket 工具（gogo/fscan）平台无法强制
			// 拦截，需要管理员显式降级为 advisory —— 降级必须是一个知情
			// 的决定，不能是默认状态。
			ScopeEnforcement: ScopeEnforced,
			Params: []ParamLimit{
				{Flag: "p", Note: "端口或端口范围", Risk: "caution"},
				{Flag: "n", Note: "并发数", Risk: "caution"},
				{Flag: "rate", Note: "速率", Risk: "caution"},
				{Flag: "t", Note: "扫描类型", Risk: "caution"},
				{Flag: "o", Note: "输出文件", Risk: "benign"},
			},
			Passthrough: []string{"u", "l", "target", "host", "ip", "cidr", "url", "d", "domain"},
			Target: &TargetSpec{
				Kind: "any", RequireScope: true, MaxTargets: 50,
			},
			AllowTargetsInScopeOnly: true,
			DenyArgs:                 []string{"--script", "-exec", "--exec", "--upload", "--shell"},
			MaxTargets:               50,
			TimeoutSeconds:           900,
		}
	case "brute":
		return &ToolContract{
			// 默认强制范围管控。原生 socket 工具（gogo/fscan）平台无法强制
			// 拦截，需要管理员显式降级为 advisory —— 降级必须是一个知情
			// 的决定，不能是默认状态。
			ScopeEnforcement: ScopeEnforced,
			Params: []ParamLimit{
				{Flag: "u", Note: "目标 URL", Risk: "caution"},
				{Flag: "p", Note: "端口", Risk: "caution"},
				{Flag: "d", Note: "字典文件", Risk: "caution"},
				{Flag: "P", Note: "内置字典选择", Risk: "caution"},
				{Flag: "c", Note: "用户名字典", Risk: "caution"},
				{Flag: "t", Note: "并发数", Risk: "caution"},
				{Flag: "l", Note: "输出文件", Risk: "benign"},
			},
			Target: &TargetSpec{Kind: "any", RequireScope: true, MaxTargets: 20},
			AllowTargetsInScopeOnly: true,
			DenyArgs:                 []string{"--script", "-exec", "--exec", "--shell", "--upload"},
			MaxTargets:               20,
			TimeoutSeconds:           1800,
		}
	case "poc":
		return &ToolContract{
			// 默认强制范围管控。原生 socket 工具（gogo/fscan）平台无法强制
			// 拦截，需要管理员显式降级为 advisory —— 降级必须是一个知情
			// 的决定，不能是默认状态。
			ScopeEnforcement: ScopeEnforced,
			Params: []ParamLimit{
				// 模板名只允许已知模板标识，不允许路径 —— 否则模板可以指向
				// 本地任意文件并把它作为请求体发出去（数据外泄路径）。
				{Flag: "t", Pattern: `^[A-Za-z0-9_./-]+$`, Note: "模板或模板集名", Risk: "caution"},
				{Flag: "u", Note: "目标 URL", Risk: "caution"},
				{Flag: "l", Note: "目标文件", Risk: "caution"},
				{Flag: "severity", Note: "严重级别过滤", Risk: "benign"},
				{Flag: "rl", Note: "速率限制", Risk: "caution"},
				{Flag: "c", Note: "并发数", Risk: "caution"},
			},
			Target: &TargetSpec{Kind: "any", RequireScope: true, MaxTargets: 100},
			AllowTargetsInScopeOnly: true,
			DenyArgs:                 []string{"--script", "-exec", "--exec", "--shell", "--code", "-code", "--interactsh-server"},
			MaxTargets:               100,
			TimeoutSeconds:           1200,
		}
	case "proxy":
		return &ToolContract{
			// 默认强制范围管控。原生 socket 工具（gogo/fscan）平台无法强制
			// 拦截，需要管理员显式降级为 advisory —— 降级必须是一个知情
			// 的决定，不能是默认状态。
			ScopeEnforcement: ScopeEnforced,
			Params: []ParamLimit{
				{Flag: "p", Note: "监听端口", Risk: "caution"},
				{Flag: "l", Note: "上游代理地址", Risk: "caution"},
				{Flag: "i", Note: "透传规则", Risk: "caution"},
			},
			Target: &TargetSpec{Kind: "any", RequireScope: true, MaxTargets: 10},
			AllowTargetsInScopeOnly: true,
			DenyArgs:                 []string{"--script", "-exec", "--exec"},
			MaxTargets:               10,
			TimeoutSeconds:           600,
		}
	case "recon", "crawler":
		return &ToolContract{
			Params: []ParamLimit{
				{Flag: "d", Note: "字典", Risk: "caution"},
				{Flag: "w", Note: "字典集", Risk: "caution"},
				{Flag: "o", Note: "输出文件", Risk: "benign"},
				{Flag: "s", Note: "并发", Risk: "caution"},
				{Flag: "deep", Note: "深度", Risk: "benign"},
			},
			Passthrough: []string{"u", "l", "target", "host", "ip", "cidr", "url", "domain", "list"},
			Target: &TargetSpec{Kind: "any", RequireScope: true, MaxTargets: 50},
			AllowTargetsInScopeOnly: true,
			DenyArgs:                 []string{"--script", "-exec", "--exec", "--shell", "--upload"},
			MaxTargets:               50,
			TimeoutSeconds:           900,
		}
	default: // utility
		return &ToolContract{
			// 通用小工具（jq 这类）不作用于网络目标，没有范围概念。
			ScopeEnforcement: ScopeNone,
			Params: []ParamLimit{
				{Flag: "f", Pattern: `^[A-Za-z0-9_./\\-]+$`, Note: "输入文件（限制为普通文件名，不允许任意路径）", Risk: "caution"},
				{Flag: "r", Note: "原始输出", Risk: "benign"},
				{Flag: "j", Note: "紧凑输出", Risk: "benign"},
				{Flag: "s", Note: "排序", Risk: "benign"},
				{Flag: "o", Note: "输出文件", Risk: "benign"},
				{Flag: "c", Note: "颜色", Risk: "benign"},
			},
			Passthrough: []string{"a", "b", "e", "i", "n", "s", "v", "y", "z", "h", "V", "h"},
			Target: &TargetSpec{Kind: "any", RequireScope: false, MaxTargets: 0},
			AllowTargetsInScopeOnly: false,
			DenyArgs:                 []string{"--script", "-exec", "--exec", "--shell", "--eval", "-e"},
			MaxTargets:               0,
			TimeoutSeconds:           300,
		}
	}
}

// DefaultContractJSON returns the default ceiling as stored JSON.
func DefaultContractJSON(category string) json.RawMessage {
	raw, err := json.Marshal(DefaultContractForCategory(category))
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

// NativeSocketCategories are the tool categories whose implementations
// typically bypass the platform's HTTP proxy (Go programs opening raw
// sockets). For these, scope enforcement CANNOT be done by the platform — the
// traffic never passes through it.
//
// This list exists so the UI can tell the administrator which tools should
// probably be downgraded to advisory, rather than leaving them to guess.
var NativeSocketCategories = map[string]bool{
	"scanner": true,
	"brute":   true,
}

// SuggestScopeEnforcement returns the enforcement mode that suits a tool of
// the given category and whether the category is a known native-socket one.
//
// The first return value is only a suggestion: the platform's default stays
// "enforced" because a silent downgrade would be a fake guarantee.
func SuggestScopeEnforcement(category string) (mode string, nativeSocket bool) {
	if category == "utility" {
		return ScopeNone, false
	}
	if NativeSocketCategories[category] {
		return ScopeAdvisory, true
	}
	return ScopeEnforced, false
}
