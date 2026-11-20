package server

import (
	"strings"
	"testing"

	"artex/db"
)

// TestToolAvailableNoticeCarriesDecisionData pins what the "new tool available"
// notice must contain. A worker that receives this hint has to be able to decide
// whether to reach for the tool WITHOUT re-reading its full description, so the
// notice carries: what it is, how risky it is, how to call it, which dangerous
// switches the contract already removed, and the scope caveat.
func TestToolAvailableNoticeCarriesDecisionData(t *testing.T) {
	asset := &db.ToolAsset{
		Name:         "gogo",
		Category:     "scanner",
		RiskLevel:    "critical",
		Summary:      "Go 编写的端口/服务扫描器",
		UsageExample: "gogo.exe -p top2 -t 50 -o json",
		Contract:     []byte(`{"deny_args":["-e","--exploit"]}`),
		Limits:       []byte(`{"scope_enforcement":"advisory"}`),
	}
	got := toolAvailableNotice(asset)

	for _, want := range []string{
		"外部工具已可用",
		"gogo",
		"scanner",                                              // category: what kind of tool
		"critical",                                             // risk: how dangerous
		"gogo.exe -p top2 -t 50 -o json",                       // usage: how to call it
		"-e",                                                   // disabled capability
		"--exploit",                                            // disabled capability
		"平台无法拦截它访问越界目标", // scope caveat for advisory tools
		"run_tool",                                             // the managed call path
		"不要直接用 Bash",                                            // and why not to bypass it
	} {
		if !strings.Contains(got, want) {
			t.Errorf("notice missing %q\n---\n%s", want, got)
		}
	}
}

// TestToolAvailableNoticeMinimalAsset checks the notice does not emit empty
// scaffolding when keeper recorded little: a bare name must still produce a
// usable hint rather than dangling labels.
func TestToolAvailableNoticeMinimalAsset(t *testing.T) {
	got := toolAvailableNotice(&db.ToolAsset{Name: "nmap"})
	if !strings.Contains(got, "nmap") {
		t.Fatalf("notice lost the tool name: %s", got)
	}
	for _, unwanted := range []string{"（）", "风险等级 \n", "调用示例:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("notice has empty-section scaffolding %q\n---\n%s", unwanted, got)
		}
	}
}
