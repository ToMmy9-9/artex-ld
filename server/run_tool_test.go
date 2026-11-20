// run_tool_test.go：受管执行入口的闭环测试。
//
// 这些用例不走 LLM，直接验证「结构化参数 → 契约校验 → 范围校验 → 执行 →
// 审计」这条链路，以及三条必须拒绝的越权路径：参数越界、目标越界、契约
// 外的参数夹带。
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"artex/db"
	actool "norma/tool"
)


// newTestServer builds a Server backed by the test database, with the managed
// tool directory pointed at a per-test temp dir so the test never touches the
// real tool library.
func newTestServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	dsn, _, err := db.DSN()
	if err != nil {
		t.Skipf("no database config (%v) — skipping", err)
	}
	pg, err := db.Open(dsn)
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	// A Server with just a manager is enough for the tool-level paths we
	// exercise here (no engine, no scheduler).
	m := &Manager{dir: t.TempDir(), pg: pg}
	s := &Server{m: m, dataDir: filepath.Join(t.TempDir(), "data"), baseDir: t.TempDir()}

	// The test assets live in the real database (there is only one PG), so
	// they must be removed afterwards — otherwise every test run would leave
	// junk rows behind that later show up in the tool catalogue.
	//
	// The cleanup is registered BEFORE the caller closes the DB: Go runs
	// t.Cleanup callbacks after the test function returns, but the caller's
	// `defer pg.Close()` fires first — so closing must happen inside the
	// cleanup too, and the DELETE has to run before it.
	t.Cleanup(func() {
		_, _ = pg.Exec(`DELETE FROM tool_asset_runs WHERE asset_id IN
			(SELECT id FROM tool_assets WHERE name LIKE 'testtool%')`)
		_, _ = pg.Exec(`DELETE FROM tool_assets WHERE name LIKE 'testtool%'`)
		_, _ = pg.Exec(`DELETE FROM tools WHERE key LIKE 'ext\_testtool%'`)
		pg.Close()
	})
	return s, pg
}

// newTestToolAsset writes a tiny executable into the managed directory and
// registers it as a managed asset with the given contract.
//
// The "executable" is a shell/batch shim that echoes its argv, so the test
// can assert on the exact command line the platform composed.
func newTestToolAsset(t *testing.T, s *Server, name, scriptBody string, limits, contract *db.ToolContract) *db.ToolAsset {
	t.Helper()
	// The (name, platform) unique index means a test asset name must be
	// unique per run; suffix it so repeated runs never collide.
	name = name + "_" + shortRand()
	dir := s.toolAssetDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ext, body := ".bat", scriptBody
	if runtime.GOOS != "windows" {
		ext, body = ".sh", echoShimSh
	}
	path := filepath.Join(dir, name+ext)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	limJSON, _ := json.Marshal(limits)
	conJSON, _ := json.Marshal(contract)
	asset := &db.ToolAsset{
		Name: name, DisplayName: name + ext, SHA256: "test", Platform: "windows",
		RelPath: name + ext, State: "managed", Limits: limJSON, Contract: conJSON,
	}
	id, err := s.m.pg.InsertToolAsset(asset)
	if err != nil {
		t.Fatalf("insert asset: %v", err)
	}
	asset.ID = id
	return asset
}

// echoShim prints its own arguments so the test can assert the composed argv.
//
// On Windows a .bat is executed by the shell; elsewhere a shell script with a
// shebang works. The asset's rel_path keeps the matching extension so the
// platform runs the right thing.
const echoShim = "@echo off\r\necho ARGV:%*\r\necho done\r\n"
const echoShimSh = "#!/bin/sh\necho ARGV:$*\necho done\n"

// callRunTool invokes the run_tool core tool with the given JSON input.
func callRunTool(t *testing.T, s *Server, input map[string]any) (string, error) {
	t.Helper()
	in, _ := json.Marshal(input)
	tool := s.toolRunTool()
	res, err := tool.Call(context.Background(), in, &actool.ToolContext{AgentID: "worker"})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, b := range res.Content {
		if b.Text != "" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String(), nil
}

func TestRunToolComposesArgvAndAudits(t *testing.T) {
	s, pg := newTestServer(t)

	limits := &db.ToolContract{
		Params: []db.ParamLimit{
			{Flag: "p", Allowed: []string{"1-1024", "80"}},
			{Flag: "c", Allowed: []string{"-c"}},
		},
		DenyArgs:      []string{"--script"},
		MaxTargets:    10,
		TimeoutSeconds: 30,
	}
	contract := &db.ToolContract{
		Params: []db.ParamLimit{
			{Flag: "p", Allowed: []string{"1-1024"}},
			{Flag: "c", Allowed: []string{"-c"}},
		},
		DenyArgs:      []string{"--script"},
		MaxTargets:    10,
		TimeoutSeconds: 30,
	}
	asset := newTestToolAsset(t, s, "testtool1", echoShim, limits, contract)

	out, err := callRunTool(t, s, map[string]any{
		"tool": asset.Name,
		"args": map[string]string{"p": "1-1024", "c": "-c"},
	})
	if err != nil {
		t.Fatalf("run_tool: %v", err)
	}
	if !strings.Contains(out, "exit=0") {
		t.Errorf("expected a successful run, got: %s", out)
	}
	// The composed argv must contain the flags and the values.
	if !strings.Contains(out, "-p") || !strings.Contains(out, "1-1024") {
		t.Errorf("composed argv missing the flag/value: %s", out)
	}

	// The audit row must exist with the composed argv.
	runs, err := pg.ListToolAssetRuns(asset.ID, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected exactly 1 audit row, got %d", len(runs))
	}
	if !strings.Contains(string(runs[0].Argv), "-p") {
		t.Errorf("audit argv missing the flag: %s", runs[0].Argv)
	}
	if runs[0].AgentKey != "worker" {
		t.Errorf("audit agent key = %q, want worker", runs[0].AgentKey)
	}
}

func TestRunToolRejectsArgOutsideContract(t *testing.T) {
	s, _ := newTestServer(t)

	limits := &db.ToolContract{Params: []db.ParamLimit{{Flag: "p", Allowed: []string{"80"}}}}
	contract := &db.ToolContract{Params: []db.ParamLimit{{Flag: "p", Allowed: []string{"80"}}}}
	asset := newTestToolAsset(t, s, "testtool2", echoShim, limits, contract)

	out, err := callRunTool(t, s, map[string]any{
		"tool": asset.Name,
		"args": map[string]string{"p": "1-65535"},
	})
	if err != nil {
		t.Fatalf("run_tool: %v", err)
	}
	if !strings.Contains(out, "允许范围") {
		t.Errorf("a port value outside the contract should be rejected, got: %s", out)
	}
}

func TestRunToolRejectsFlagOutsideContract(t *testing.T) {
	s, _ := newTestServer(t)

	limits := &db.ToolContract{Params: []db.ParamLimit{{Flag: "p", Allowed: []string{"80"}}}}
	contract := &db.ToolContract{Params: []db.ParamLimit{{Flag: "p", Allowed: []string{"80"}}}}
	asset := newTestToolAsset(t, s, "testtool3", echoShim, limits, contract)

	out, err := callRunTool(t, s, map[string]any{
		"tool": asset.Name,
		"args": map[string]string{"evil": "x"},
	})
	if err != nil {
		t.Fatalf("run_tool: %v", err)
	}
	if !strings.Contains(out, "契约允许范围") {
		t.Errorf("an unsanctioned flag should be rejected, got: %s", out)
	}
}

func TestRunToolRejectsOutOfScopeTarget(t *testing.T) {
	s, pg := newTestServer(t)

	limits := &db.ToolContract{
		Params:                []db.ParamLimit{{Flag: "p", Allowed: []string{"80"}}},
		AllowTargetsInScopeOnly: true,
		Target:                &db.TargetSpec{Kind: "host", RequireScope: true, MaxTargets: 10},
		DenyArgs:              []string{"--script"},
		MaxTargets:            10,
		TimeoutSeconds:        30,
	}
	contract := &db.ToolContract{
		Params:                []db.ParamLimit{{Flag: "p", Allowed: []string{"80"}}},
		AllowTargetsInScopeOnly: true,
		Target:                &db.TargetSpec{Kind: "host", RequireScope: true, MaxTargets: 10},
		DenyArgs:              []string{"--script"},
		MaxTargets:            10,
		TimeoutSeconds:        30,
	}
	asset := newTestToolAsset(t, s, "testtool4", echoShim, limits, contract)

	// No task scope is attached to this test run, so every target is out of
	// scope. This is the key property: an unauthorized target must never
	// reach the process.
	out, err := callRunTool(t, s, map[string]any{
		"tool":    asset.Name,
		"args":    map[string]string{"p": "80"},
		"targets": []string{"8.8.8.8"},
	})
	if err != nil {
		t.Fatalf("run_tool: %v", err)
	}
	if !strings.Contains(out, "授权范围") {
		t.Errorf("an out-of-scope target should be rejected, got: %s", out)
	}
	// The rejection must be audited (exit=-1 marks "refused before running"),
	// and the refused target must never appear in the composed argv.
	runs, _ := pg.ListToolAssetRuns(asset.ID, 10)
	if len(runs) != 1 {
		t.Fatalf("expected 1 audit row (the rejection), got %d", len(runs))
	}
	if runs[0].ExitCode != -1 {
		t.Errorf("a refused call should be audited with exit=-1, got %d", runs[0].ExitCode)
	}
	// The audit MUST contain the refused target — that is the point of
	// recording a rejection: it shows what was attempted. What must NOT
	// happen is the process running.
	if !strings.Contains(string(runs[0].Argv), "8.8.8.8") {
		t.Errorf("the refused target should be recorded in the audit trail: %s", runs[0].Argv)
	}
	if strings.Contains(out, "ARGV:") {
		t.Errorf("the tool actually ran despite the out-of-scope target: %s", out)
	}
}

func TestRunToolRejectsUnclaimedTool(t *testing.T) {
	s, pg := newTestServer(t)

	limits := &db.ToolContract{Params: []db.ParamLimit{{Flag: "p"}}}
	nc := newTestToolAsset(t, s, "testtoolNC", echoShim, limits, &db.ToolContract{})
	// Force the state back to uploaded.
	if _, err := pg.Exec(`UPDATE tool_assets SET state='uploaded' WHERE id=$1`, nc.ID); err != nil {
		t.Fatalf("reset state: %v", err)
	}
	out, err := callRunTool(t, s, map[string]any{"tool": nc.Name, "args": map[string]string{}})
	if err != nil {
		t.Fatalf("run_tool: %v", err)
	}
	if !strings.Contains(out, "尚未被认领") {
		t.Errorf("an unclaimed tool must not be callable, got: %s", out)
	}
}

func TestRunToolRejectsUnknownTool(t *testing.T) {
	s, _ := newTestServer(t)
	out, err := callRunTool(t, s, map[string]any{"tool": "does-not-exist"})
	if err != nil {
		t.Fatalf("run_tool: %v", err)
	}
	if !strings.Contains(out, "不存在") {
		t.Errorf("an unknown tool should be rejected, got: %s", out)
	}
}

// shortRand returns a short random suffix for unique test asset names.
func shortRand() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "x"
	}
	return hex.EncodeToString(b)
}

// TestRunToolAdvisoryModeAllowsButAuditsOutOfScope covers the explicit
// downgrade path for tools the platform cannot constrain (native sockets).
//
// The contract here says scope_enforcement=advisory, i.e. the platform KNOWS
// it cannot stop this tool from reaching out-of-scope targets. In that mode
// the call must proceed — but the audit must record the bypass, and the agent
// must be told.
func TestRunToolAdvisoryModeAllowsButAuditsOutOfScope(t *testing.T) {
	s, pg := newTestServer(t)

	limits := &db.ToolContract{
		Params:                []db.ParamLimit{{Flag: "p", Allowed: []string{"80"}}},
		AllowTargetsInScopeOnly: true,
		Target:                &db.TargetSpec{Kind: "host", RequireScope: true, MaxTargets: 10},
		ScopeEnforcement:      db.ScopeAdvisory,
		MaxTargets:            10,
		TimeoutSeconds:        30,
	}
	contract := &db.ToolContract{
		Params:                []db.ParamLimit{{Flag: "p", Allowed: []string{"80"}}},
		AllowTargetsInScopeOnly: true,
		Target:                &db.TargetSpec{Kind: "host", RequireScope: true, MaxTargets: 10},
		ScopeEnforcement:      db.ScopeAdvisory,
		MaxTargets:            10,
		TimeoutSeconds:        30,
	}
	asset := newTestToolAsset(t, s, "testtoolAdv", echoShim, limits, contract)

	out, err := callRunTool(t, s, map[string]any{
		"tool":    asset.Name,
		"args":    map[string]string{"p": "80"},
		"targets": []string{"8.8.8.8"},
	})
	if err != nil {
		t.Fatalf("run_tool: %v", err)
	}
	// The call must proceed in advisory mode.
	if !strings.Contains(out, "exit=0") {
		t.Errorf("advisory mode should let the call proceed, got: %s", out)
	}
	// And the agent must be told the target was out of scope.
	if !strings.Contains(out, "越界目标已放行") {
		t.Errorf("advisory mode must warn the agent about the bypass, got: %s", out)
	}
	// The audit must be marked as a scope bypass.
	runs, _ := pg.ListToolAssetRuns(asset.ID, 10)
	if len(runs) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(runs))
	}
	if !strings.Contains(string(runs[0].Argv), "scope_bypass") {
		t.Errorf("the audit row should be marked as a scope bypass: %s", runs[0].Argv)
	}
}

// TestRunToolEnforcedStillRefusesWhenScopeIsEmpty guards the default: a tool
// that has NOT been explicitly downgraded must still refuse out-of-scope
// targets even when the contract says nothing about enforcement.
func TestRunToolEnforcedStillRefusesWhenScopeIsEmpty(t *testing.T) {
	s, _ := newTestServer(t)

	limits := &db.ToolContract{
		Params:                []db.ParamLimit{{Flag: "p", Allowed: []string{"80"}}},
		AllowTargetsInScopeOnly: true,
		Target:                &db.TargetSpec{Kind: "host", RequireScope: true, MaxTargets: 10},
		MaxTargets:            10,
		TimeoutSeconds:        30,
	}
	asset := newTestToolAsset(t, s, "testtoolEnf", echoShim, limits, &db.ToolContract{
		Params:                []db.ParamLimit{{Flag: "p", Allowed: []string{"80"}}},
		AllowTargetsInScopeOnly: true,
		Target:                &db.TargetSpec{Kind: "host", RequireScope: true, MaxTargets: 10},
		MaxTargets:            10,
		TimeoutSeconds:        30,
	})

	out, err := callRunTool(t, s, map[string]any{
		"tool":    asset.Name,
		"args":    map[string]string{"p": "80"},
		"targets": []string{"8.8.8.8"},
	})
	if err != nil {
		t.Fatalf("run_tool: %v", err)
	}
	if !strings.Contains(out, "授权范围") {
		t.Errorf("the default (enforced) must refuse out-of-scope targets, got: %s", out)
	}
}
