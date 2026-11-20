// tool_contract_test.go：外部工具契约的信任边界测试。
//
// 这组测试守的是整个工具管控体系最关键的一条：agent 认领工具时产出的
// 契约只能收紧权限，不能放宽。放宽权限的权利必须留在平台代码手里。
package db

import "testing"

// limits 是平台为某工具定义的最大权限（管理员/平台侧写）。
func testLimits() *ToolContract {
	return &ToolContract{
		Params: []ParamLimit{
			{Flag: "p", Allowed: []string{"1-1024", "1-65535", "80", "443"}, Risk: "caution",
				Note: "端口范围，只允许这几档"},
			{Flag: "t", Pattern: `^[a-z0-9_-]+$`, Risk: "caution"},
			{Flag: "script", Deny: true, Risk: "dangerous"},
			{Flag: "exec", Deny: true, Risk: "dangerous"},
		},
		Target:                &TargetSpec{Kind: "host", RequireScope: true, MaxTargets: 20},
		AllowTargetsInScopeOnly: true,
		DenyArgs:              []string{"--script", "-exec"},
		MaxTargets:            20,
		TimeoutSeconds:        300,
	}
}

func TestContractCannotWidenParamWhitelist(t *testing.T) {
	limits := testLimits()
	bad := &ToolContract{
		Params: []ParamLimit{{Flag: "p", Allowed: []string{"1-1024", "1-65535", "1-100000"}}},
	}
	if err := ValidateContractAgainstLimits(bad, limits); err == nil {
		t.Fatal("contract widened the port whitelist but was accepted")
	}
}

func TestContractCannotInventUnsanctionedFlag(t *testing.T) {
	limits := testLimits()
	bad := &ToolContract{
		Params: []ParamLimit{{Flag: "brand-new-flag"}},
	}
	if err := ValidateContractAgainstLimits(bad, limits); err == nil {
		t.Fatal("contract invented a flag the platform never sanctioned")
	}
}

func TestContractCannotIncludeDeniedFlag(t *testing.T) {
	limits := testLimits()
	bad := &ToolContract{
		Params: []ParamLimit{{Flag: "script"}},
	}
	if err := ValidateContractAgainstLimits(bad, limits); err == nil {
		t.Fatal("contract re-enabled a flag the platform denied")
	}
}

func TestContractCannotDropScopeRequirement(t *testing.T) {
	limits := testLimits()
	bad := &ToolContract{
		Target: &TargetSpec{Kind: "host", RequireScope: false},
	}
	if err := ValidateContractAgainstLimits(bad, limits); err == nil {
		t.Fatal("contract dropped the in-scope-only requirement")
	}
}

func TestContractCannotRaiseTargetCount(t *testing.T) {
	limits := testLimits()
	bad := &ToolContract{
		Target:     &TargetSpec{Kind: "host", RequireScope: true},
		MaxTargets: 500,
	}
	if err := ValidateContractAgainstLimits(bad, limits); err == nil {
		t.Fatal("contract raised the max target count")
	}
}

// TestContractNeedNotRepeatPlatformDenyArgs documents the inheritance rule:
// the agent's contract does NOT have to restate the platform's deny list —
// the platform enforces its own list at execution time. What the agent must
// not do is WIDEN it (i.e. declare a deny fragment that isn't in the list,
// which would be a claim of permission the platform never granted).
func TestContractNeedNotRepeatPlatformDenyArgs(t *testing.T) {
	limits := testLimits()
	// Not restating the platform's deny list is fine.
	ok := &ToolContract{DenyArgs: []string{}}
	if err := ValidateContractAgainstLimits(ok, limits); err != nil {
		t.Fatalf("contract omitting the platform deny list was rejected: %v", err)
	}
	// Declaring a deny fragment the platform never had is also fine (it can
	// only narrow), but a contract that claims a param outside the limits
	// must still fail — that is the real widening vector.
	bad := &ToolContract{Params: []ParamLimit{{Flag: "brand-new"}}}
	if err := ValidateContractAgainstLimits(bad, limits); err == nil {
		t.Fatal("contract invented a param the platform never sanctioned")
	}
}

// TestContractCannotNarrowScopeToNoScope confirms the explicit "opt out of
// scope" case is still rejected — the relaxed inheritance rule must not
// become a way to turn scope enforcement off.
func TestContractCannotNarrowScopeToNoScope(t *testing.T) {
	limits := testLimits()
	bad := &ToolContract{
		Target:                &TargetSpec{Kind: "host", RequireScope: false},
		AllowTargetsInScopeOnly: false,
	}
	if err := ValidateContractAgainstLimits(bad, limits); err == nil {
		t.Fatal("contract explicitly opted out of the in-scope requirement")
	}
}

func TestContractCannotRaiseTimeout(t *testing.T) {
	limits := testLimits()
	bad := &ToolContract{TimeoutSeconds: 3600}
	if err := ValidateContractAgainstLimits(bad, limits); err == nil {
		t.Fatal("contract raised the timeout ceiling")
	}
}

// TestContractMayTighten 确认"收紧"是允许的——否则 agent 认领毫无意义。
func TestContractMayTighten(t *testing.T) {
	limits := testLimits()
	ok := &ToolContract{
		Params: []ParamLimit{
			{Flag: "p", Allowed: []string{"1-1024"}}, // 更窄的白名单
			{Flag: "t", Pattern: `^[a-z0-9_-]+$`},
		},
		Target:                   &TargetSpec{Kind: "host", RequireScope: true, MaxTargets: 5},
		AllowTargetsInScopeOnly:  true,
		MaxTargets:               5, // 更小的目标数
		DenyArgs:                 []string{"--script", "-exec"},
		TimeoutSeconds:           60, // 更短的超时
	}
	if err := ValidateContractAgainstLimits(ok, limits); err != nil {
		t.Fatalf("a strictly tighter contract was rejected: %v", err)
	}
}

func TestCheckArgvEnforcesContract(t *testing.T) {
	c := &ToolContract{
		Params: []ParamLimit{
			{Flag: "p", Allowed: []string{"1-65535", "80"}},
			{Flag: "t", Pattern: `^[a-z0-9_-]+$`},
		},
		Passthrough: []string{"u", "l"},
		DenyArgs:    []string{"--script"},
	}
	cases := []struct {
		name    string
		argv    []string
		wantErr bool
	}{
		{"ok", []string{"gogo", "-p", "1-65535", "10.0.0.1"}, false},
		{"bad port value", []string{"gogo", "-p", "1-100000", "10.0.0.1"}, true},
		{"denied fragment", []string{"gogo", "--script", "evil.sh"}, true},
		{"unknown flag", []string{"gogo", "--pwn", "x"}, true},
		{"bad template value", []string{"nuclei", "-t", "../../etc/passwd"}, true},
		{"passthrough flag allowed", []string{"nuclei", "-u", "http://x", "-l", "hosts.txt"}, false},
		{"non-passthrough unknown flag", []string{"nuclei", "-zzz", "x"}, true},
		{"ok template", []string{"nuclei", "-t", "cves-2021", "-u", "http://x"}, false},
	}
	for _, tc := range cases {
		got := c.CheckArgv(tc.argv)
		if tc.wantErr && got == "" {
			t.Errorf("%s: expected rejection, got none", tc.name)
		}
		if !tc.wantErr && got != "" {
			t.Errorf("%s: expected allow, got %q", tc.name, got)
		}
	}
}

func TestCheckArgvRequiredParam(t *testing.T) {
	c := &ToolContract{Params: []ParamLimit{{Flag: "u", Required: true}}}
	if got := c.CheckArgv([]string{"nuclei", "-t", "x"}); got == "" {
		t.Fatal("missing required param was accepted")
	}
}
