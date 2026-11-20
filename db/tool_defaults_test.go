// tool_defaults_test.go：默认权限上限的行为约束。
//
// 默认上限存在的意义：没有它，agent 产出的任何参数都会被判"平台未授权"
// 而拒绝，工具就永远停在"已上传但不可用"。所以默认上限必须足够宽松，
// 但仍然保留硬边界（禁止脚本执行、禁止任意本地路径、目标必须受约束）。
package db

import "testing"

func TestDefaultContractAllowsAgentToProduceValidContract(t *testing.T) {
	for _, cat := range []string{"", "utility", "scanner", "brute", "poc", "proxy", "recon", "crawler"} {
		limits := DefaultContractForCategory(cat)
		// agent 产出：只声明一个 limits 里已有的 flag
		contract := &ToolContract{
			Params: []ParamLimit{{Flag: limits.SortedFlags()[0]}},
		}
		if err := ValidateContractAgainstLimits(contract, limits); err != nil {
			t.Errorf("category %q: 默认上限拒绝了合法的收紧契约: %v", cat, err)
		}
	}
}

func TestDefaultContractForbidsScriptExecution(t *testing.T) {
	for _, cat := range []string{"", "utility", "scanner", "brute", "poc", "proxy", "recon", "crawler"} {
		limits := DefaultContractForCategory(cat)
		joined := ""
		for _, d := range limits.DenyArgs {
			joined += d + " "
		}
		for _, bad := range []string{"--script", "-exec", "--exec"} {
			found := false
			for _, d := range limits.DenyArgs {
				if d == bad {
					found = true
				}
			}
			if !found {
				t.Errorf("category %q: 默认上限未禁止 %q（deny_args=%q）", cat, bad, joined)
			}
		}
	}
}

func TestDefaultContractScopesNetworkToolsToTaskScope(t *testing.T) {
	// 会发网络请求的类别必须强制目标在授权范围内。
	for _, cat := range []string{"scanner", "brute", "poc", "proxy", "recon", "crawler"} {
		limits := DefaultContractForCategory(cat)
		if !limits.AllowTargetsInScopeOnly {
			t.Errorf("category %q: 默认上限未强制目标在授权范围内", cat)
		}
		if limits.Target == nil || !limits.Target.RequireScope {
			t.Errorf("category %q: 默认上限的 target 未要求 require_scope", cat)
		}
	}
}

func TestDefaultContractPocForbidsArbitraryTemplatePaths(t *testing.T) {
	// POC 框架的模板参数必须限制成"名字"而不是任意路径 —— 否则模板可以
	// 指向本地任意文件并把它作为请求体发出去（数据外泄路径）。
	limits := DefaultContractForCategory("poc")
	idx := limits.ParamIndex()
	tpl, ok := idx["t"]
	if !ok {
		t.Fatal("poc 默认上限未声明 t 参数")
	}
	if tpl.Pattern == "" {
		t.Fatal("poc 的 t 参数没有取值约束")
	}
	// 一个明显的本地路径必须被拒
	c := &ToolContract{Params: []ParamLimit{{Flag: "t", Pattern: tpl.Pattern}}}
	if got := c.CheckArgv([]string{"nuclei", "-t", "C:\\Windows\\System32\\config"}); got == "" {
		t.Fatal("poc 模板参数接受了明显的本地绝对路径")
	}
}

func TestDefaultContractUtilityRejectsArbitraryFileFlag(t *testing.T) {
	limits := DefaultContractForCategory("utility")
	idx := limits.ParamIndex()
	f, ok := idx["f"]
	if !ok {
		t.Fatal("utility 默认上限未声明 f 参数")
	}
	c := &ToolContract{Params: []ParamLimit{{Flag: "f", Pattern: f.Pattern}}}
	if got := c.CheckArgv([]string{"jq", "-f", "C:/Windows/System32/config/SAM"}); got == "" {
		t.Fatal("utility 的 f 参数接受了任意本地绝对路径")
	}
}
