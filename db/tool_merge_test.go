// tool_merge_test.go：执行期契约合并的边界测试。
//
// MergeContracts 是真正被执行的那一层：agent 的契约与平台的上限合并，
// 结果是实际生效的权限。合并必须是「取更严的一方」，任何一侧都能收紧，
// 但任何一侧都不能放宽另一侧。
package db

import "testing"

func TestMergeKeepsPlatformDenyArgs(t *testing.T) {
	limits := &ToolContract{DenyArgs: []string{"--script"}}
	contract := &ToolContract{DenyArgs: []string{}}
	got := MergeContracts(contract, limits)
	if len(got.DenyArgs) == 0 || got.DenyArgs[0] != "--script" {
		t.Fatalf("platform deny list lost after merge: %v", got.DenyArgs)
	}
}

func TestMergeKeepsPlatformDeniedParam(t *testing.T) {
	limits := &ToolContract{Params: []ParamLimit{{Flag: "script", Deny: true}}}
	contract := &ToolContract{Params: []ParamLimit{{Flag: "script", Risk: "benign"}}}
	got := MergeContracts(contract, limits)
	idx := got.ParamIndex()
	p, ok := idx["script"]
	if !ok || !p.Deny {
		t.Fatalf("platform-denied param was revived by the agent contract: %+v", p)
	}
}

func TestMergeNarrowsAllowedList(t *testing.T) {
	limits := &ToolContract{Params: []ParamLimit{{Flag: "p", Allowed: []string{"1-1024", "80"}}}}
	contract := &ToolContract{Params: []ParamLimit{{Flag: "p", Allowed: []string{"80"}}}}
	got := MergeContracts(contract, limits)
	idx := got.ParamIndex()
	p := idx["p"]
	if len(p.Allowed) != 1 || p.Allowed[0] != "80" {
		t.Fatalf("merged allowed list is not the narrower one: %v", p.Allowed)
	}
}

func TestMergeDoesNotWidenAllowedList(t *testing.T) {
	limits := &ToolContract{Params: []ParamLimit{{Flag: "p", Allowed: []string{"80"}}}}
	contract := &ToolContract{Params: []ParamLimit{{Flag: "p", Allowed: []string{"80", "1-65535"}}}}
	got := MergeContracts(contract, limits)
	idx := got.ParamIndex()
	for _, v := range idx["p"].Allowed {
		if v == "1-65535" {
			t.Fatalf("merge widened the allowed list: %v", idx["p"].Allowed)
		}
	}
}

func TestMergeKeepsScopeRequirement(t *testing.T) {
	limits := &ToolContract{
		Target:                &TargetSpec{Kind: "any", RequireScope: true, MaxTargets: 50},
		AllowTargetsInScopeOnly: true,
		MaxTargets:             50,
	}
	// agent tries to opt out
	contract := &ToolContract{
		Target:                  &TargetSpec{Kind: "any", RequireScope: false},
		AllowTargetsInScopeOnly: false,
	}
	got := MergeContracts(contract, limits)
	if got.Target == nil || !got.Target.RequireScope {
		t.Fatal("merge let the agent drop the scope requirement")
	}
	if !got.AllowTargetsInScopeOnly {
		t.Fatal("merge let the agent drop allow_targets_in_scope_only")
	}
}

func TestMergeTakesSmallerMaxTargets(t *testing.T) {
	limits := &ToolContract{MaxTargets: 50, TimeoutSeconds: 900}
	contract := &ToolContract{MaxTargets: 5, TimeoutSeconds: 60}
	got := MergeContracts(contract, limits)
	if got.MaxTargets != 5 {
		t.Fatalf("max targets should be the smaller one (5), got %d", got.MaxTargets)
	}
	if got.TimeoutSeconds != 60 {
		t.Fatalf("timeout should be the smaller one (60), got %d", got.TimeoutSeconds)
	}
}

func TestMergeDoesNotLetAgentRaiseLimits(t *testing.T) {
	limits := &ToolContract{MaxTargets: 5, TimeoutSeconds: 60}
	contract := &ToolContract{MaxTargets: 500, TimeoutSeconds: 3600}
	got := MergeContracts(contract, limits)
	if got.MaxTargets != 5 {
		t.Fatalf("agent raised max targets to %d", got.MaxTargets)
	}
	if got.TimeoutSeconds != 60 {
		t.Fatalf("agent raised timeout to %d", got.TimeoutSeconds)
	}
}

func TestMergeIntersectsPassthrough(t *testing.T) {
	limits := &ToolContract{Passthrough: []string{"u", "l"}}
	contract := &ToolContract{Passthrough: []string{"u", "evil"}}
	got := MergeContracts(contract, limits)
	for _, f := range got.Passthrough {
		if f == "evil" {
			t.Fatalf("merge let an unsanctioned flag through: %v", got.Passthrough)
		}
	}
}

func TestMergedContractIsEnforceable(t *testing.T) {
	// 端到端：平台上限禁止 --script，agent 契约没提它 —— 合并后仍必须拒绝。
	limits := DefaultContractForCategory("poc")
	contract := &ToolContract{Params: []ParamLimit{{Flag: "t", Pattern: `^[A-Za-z0-9_./-]+$`}}}
	eff := MergeContracts(contract, limits)
	if got := eff.CheckArgv([]string{"nuclei", "--script", "evil.sh"}); got == "" {
		t.Fatal("merged contract accepted a platform-denied argument")
	}
	if got := eff.CheckArgv([]string{"nuclei", "-t", "cves-2021"}); got != "" {
		t.Fatalf("merged contract rejected a legitimate call: %q", got)
	}
}
