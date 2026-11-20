// toolenv_pg_test.go：环境准备的完整编排测试（落库 + 落盘）。
//
// 与 toolenv_test.go（纯函数）和 toolenv_download_test.go（网络链路）不同，
// 这里验证的是 toolEnvPrep 这一步到位的编排：
//
//	HTTP 下载 → 解压到受管目录 → 登记 tool_assets（state=uploaded）→ 写审计
//
// 重点断言三件事：
//  1. 资产真的登记了，且 rel_path 指向"子目录/入口程序"（不是光一个文件名）。
//  2. 审计真的留痕，成功与失败都有记录——失败静默是最糟的形态。
//  3. 同一工具重复准备 = 覆盖同一条资产、清掉旧目录，不产生第二份。
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"artex/db"
)

// prepEntryName 返回当前平台上会被识别为可执行的入口文件名。
func prepEntryName() string {
	if runtime.GOOS == "windows" {
		return "gogo.exe"
	}
	return "gogo"
}

// makeReleaseZip 构造一个"带依赖"的发布包。
func makeReleaseZip(t *testing.T) []byte {
	t.Helper()
	return zipWith(t, map[string]string{
		prepEntryName():   "MZ\x90\x00fake-pe",
		"cfg/config.yaml": "scan:\n  ports: 1-65535\n",
		"dict/pass.txt":   "123456\npassword\n",
	})
}

func TestToolEnvPrepEndToEndPG(t *testing.T) {
	s, pg := newTestServer(t)
	body := makeReleaseZip(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	const toolName = "testtoolenvgogo"
	t.Cleanup(func() { cleanupPrepArtifacts(s, pg, toolName) })

	res, err := s.toolEnvPrep(context.Background(), toolEnvPrepRequest{
		ToolName: toolName,
		URL:      srv.URL + "/gogo-release.zip",
	}, "test")
	if err != nil {
		t.Fatalf("环境准备失败: %v", err)
	}

	// —— 返回值 ——
	if ok, _ := res["ok"].(bool); !ok {
		t.Fatalf("结果标记为失败: %v", res)
	}
	if got, _ := res["entry_bin"].(string); got != prepEntryName() {
		t.Errorf("入口识别 = %q，期望 %q", got, prepEntryName())
	}
	assetID, _ := res["asset_id"].(int64)
	if assetID == 0 {
		t.Fatalf("未登记工具资产: %v", res)
	}

	// —— 磁盘 ——
	dir := filepath.Join(s.toolAssetDir(), toolName)
	for _, rel := range []string{prepEntryName(), "cfg/config.yaml", "dict/pass.txt"} {
		if _, serr := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); serr != nil {
			t.Errorf("期望落盘的文件不存在: %s", rel)
		}
	}

	// —— 数据库 ——
	asset, aerr := pg.ToolAssetByID(assetID)
	if aerr != nil {
		t.Fatalf("查不到刚登记的资产: %v", aerr)
	}
	if asset.Name != toolName {
		t.Errorf("资产名 = %q", asset.Name)
	}
	if asset.State != "uploaded" {
		t.Errorf("新准备的资产应为 uploaded（等认领），实得 %q", asset.State)
	}
	// rel_path 必须是 "子目录/入口" 的两段形态：只存文件名的话，
	// run_tool 的 toolAssetPath 会去受管目录根下找，找不到。
	want := toolName + "/" + prepEntryName()
	if filepath.ToSlash(asset.RelPath) != want {
		t.Errorf("rel_path = %q，期望 %q", asset.RelPath, want)
	}
	// 而且这个路径必须真的能解析出来。
	if _, perr := s.toolAssetPath(asset); perr != nil {
		t.Errorf("rel_path 无法解析成真实文件: %v", perr)
	}
	if asset.SHA256 == "" {
		t.Error("资产应记录发布包哈希")
	}

	// —— 审计 ——
	preps, lerr := pg.ListToolEnvPrepsByName(toolName, 10)
	if lerr != nil {
		t.Fatalf("查审计失败: %v", lerr)
	}
	if len(preps) != 1 {
		t.Fatalf("应有 1 条审计记录，实得 %d", len(preps))
	}
	p := preps[0]
	if p.State != "ok" {
		t.Errorf("审计状态 = %q，期望 ok（错误: %s）", p.State, p.Error)
	}
	if p.AssetID != assetID {
		t.Errorf("审计里的 asset_id = %d，期望 %d", p.AssetID, assetID)
	}
	if p.FileCount != 3 {
		t.Errorf("文件数 = %d，期望 3", p.FileCount)
	}
	if p.EntryBin != prepEntryName() {
		t.Errorf("审计入口 = %q", p.EntryBin)
	}
	if p.SHA256 == "" || p.URL == "" {
		t.Error("审计应记录 URL 与哈希")
	}
	if p.By != "test" {
		t.Errorf("requested_by = %q，期望 test", p.By)
	}
}

// TestToolEnvPrepRecordsFailure 确认失败也留痕。没有这条，"我明明让它装过"
// 就无法追溯。
func TestToolEnvPrepRecordsFailure(t *testing.T) {
	s, pg := newTestServer(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()

	const toolName = "testtoolenvfail"
	t.Cleanup(func() { cleanupPrepArtifacts(s, pg, toolName) })

	_, err := s.toolEnvPrep(context.Background(), toolEnvPrepRequest{
		ToolName: toolName,
		URL:      srv.URL + "/missing.zip",
	}, "test")
	if err == nil {
		t.Fatal("404 应当导致失败")
	}

	preps, lerr := pg.ListToolEnvPrepsByName(toolName, 10)
	if lerr != nil {
		t.Fatalf("查审计失败: %v", lerr)
	}
	if len(preps) != 1 {
		t.Fatalf("失败也应留 1 条审计记录，实得 %d", len(preps))
	}
	if preps[0].State != "failed" {
		t.Errorf("审计状态 = %q，期望 failed", preps[0].State)
	}
	if preps[0].Error == "" {
		t.Error("失败记录应带错误原因")
	}
	// 失败时不能留下半成品目录。
	if _, serr := os.Stat(filepath.Join(s.toolAssetDir(), toolName)); serr == nil {
		t.Error("失败后不应留下目标目录")
	}
}

// TestToolEnvPrepReplacesCleanly 确认重复准备 = 覆盖而不是累加。
//
// 这条很关键：旧版本残留的 DLL/配置会让新版本起不来，属于极难排查的问题。
func TestToolEnvPrepReplacesCleanly(t *testing.T) {
	s, pg := newTestServer(t)
	const toolName = "testtoolenvreplace"
	t.Cleanup(func() { cleanupPrepArtifacts(s, pg, toolName) })

	// 第一次：带一个旧文件
	v1 := zipWith(t, map[string]string{
		prepEntryName(): "MZ v1",
		"old_dep.dll":   "stale dependency",
	})
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(v1)
	}))
	defer srv1.Close()

	res1, err := s.toolEnvPrep(context.Background(), toolEnvPrepRequest{
		ToolName: toolName, URL: srv1.URL + "/v1.zip",
	}, "test")
	if err != nil {
		t.Fatalf("第一次准备失败: %v", err)
	}
	id1, _ := res1["asset_id"].(int64)

	// 第二次：同名，但没有 old_dep.dll
	v2 := zipWith(t, map[string]string{
		prepEntryName(): "MZ v2",
		"new_dep.dll":   "fresh dependency",
	})
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(v2)
	}))
	defer srv2.Close()

	res2, err := s.toolEnvPrep(context.Background(), toolEnvPrepRequest{
		ToolName: toolName, URL: srv2.URL + "/v2.zip",
	}, "test")
	if err != nil {
		t.Fatalf("第二次准备失败: %v", err)
	}
	id2, _ := res2["asset_id"].(int64)
	if replaced, _ := res2["replaced"].(bool); !replaced {
		t.Error("第二次应标记为替换")
	}
	if id1 != id2 {
		t.Errorf("重复准备应覆盖同一条资产（%d → %d），否则会重复登记", id1, id2)
	}

	dir := filepath.Join(s.toolAssetDir(), toolName)
	if _, serr := os.Stat(filepath.Join(dir, "old_dep.dll")); serr == nil {
		t.Error("旧版本的残留文件应被清掉（会导致新版本起不来）")
	}
	if _, serr := os.Stat(filepath.Join(dir, "new_dep.dll")); serr != nil {
		t.Error("新版本的文件应当存在")
	}
	got, rerr := os.ReadFile(filepath.Join(dir, prepEntryName()))
	if rerr != nil || string(got) != "MZ v2" {
		t.Errorf("主程序应被更新为 v2，实得 %q (err=%v)", string(got), rerr)
	}
}

// TestToolEnvPrepRejectsBadToolName 确认工具名污染不能穿透到目录名。
func TestToolEnvPrepRejectsBadToolName(t *testing.T) {
	s, pg := newTestServer(t)
	for _, bad := range []string{"", "../escape", "a/b", "..", repeatStr("a", 100)} {
		if _, err := s.toolEnvPrep(context.Background(), toolEnvPrepRequest{
			ToolName: bad, URL: "http://127.0.0.1:1/x.zip",
		}, "test"); err == nil {
			t.Errorf("工具名 %q 应当被拒绝", bad)
		}
	}
	_ = pg
}

// TestToolEnvPrepEntryHint 确认调用方可以手动指定入口程序。
func TestToolEnvPrepEntryHint(t *testing.T) {
	s, pg := newTestServer(t)
	const toolName = "testtoolenvhint"
	t.Cleanup(func() { cleanupPrepArtifacts(s, pg, toolName) })

	// 包里有两个可执行候选，自动识别会挑与工具名同名的；
	// 但调用方明确指定 bin/real.exe 时应尊重它。
	realEntry := "bin/real.exe"
	if runtime.GOOS != "windows" {
		realEntry = "bin/real"
	}
	body := zipWith(t, map[string]string{
		prepEntryName(): "decoy",
		realEntry:       "the real one",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	res, err := s.toolEnvPrep(context.Background(), toolEnvPrepRequest{
		ToolName:  toolName,
		URL:       srv.URL + "/pkg.zip",
		EntryHint: realEntry,
	}, "test")
	if err != nil {
		t.Fatalf("环境准备失败: %v", err)
	}
	if got, _ := res["entry_bin"].(string); got != realEntry {
		t.Errorf("入口应尊重 hint %q，实得 %q", realEntry, got)
	}
	assetID, _ := res["asset_id"].(int64)
	asset, aerr := pg.ToolAssetByID(assetID)
	if aerr != nil {
		t.Fatalf("查资产失败: %v", aerr)
	}
	want := toolName + "/" + realEntry
	if filepath.ToSlash(asset.RelPath) != want {
		t.Errorf("rel_path = %q，期望 %q", asset.RelPath, want)
	}
}

// TestToolEnvPrepNoExecutableStillSucceeds 确认"只有字典/模板"的包不被当成失败。
func TestToolEnvPrepNoExecutableStillSucceeds(t *testing.T) {
	s, pg := newTestServer(t)
	const toolName = "testtoolenvdict"
	t.Cleanup(func() { cleanupPrepArtifacts(s, pg, toolName) })

	body := zipWith(t, map[string]string{
		"dict/pass.txt":    "123456\n",
		"dict/users.txt":   "admin\n",
		"templates/a.yaml": "id: a\n",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	res, err := s.toolEnvPrep(context.Background(), toolEnvPrepRequest{
		ToolName: toolName, URL: srv.URL + "/dict.zip",
	}, "test")
	if err != nil {
		t.Fatalf("纯资源包应当成功: %v", err)
	}
	if entry, _ := res["entry_bin"].(string); entry != "" {
		t.Errorf("纯资源包不应识别出入口程序，实得 %q", entry)
	}
	if note, _ := res["note"].(string); note == "" {
		t.Error("应提示『包内未识别出可执行文件』")
	}
	// 文件还是要落地。
	if _, serr := os.Stat(filepath.Join(s.toolAssetDir(), toolName, "dict", "pass.txt")); serr != nil {
		t.Error("字典文件应当落盘")
	}
}

// cleanupPrepArtifacts removes one test tool's DB rows and its managed
// directory. It's called via t.Cleanup (after the test body), and the DB
// itself is closed by newTestServer's own cleanup registered earlier — Go runs
// Later-registered cleanups First, so this one runs while the DB is still open.
func cleanupPrepArtifacts(s *Server, pg *db.DB, toolName string) {
	_, _ = pg.Exec(`DELETE FROM tool_env_preps WHERE tool_name = $1`, toolName)
	_, _ = pg.Exec(`DELETE FROM tool_asset_runs WHERE asset_id IN
		(SELECT id FROM tool_assets WHERE name = $1)`, toolName)
	_, _ = pg.Exec(`DELETE FROM tool_assets WHERE name = $1`, toolName)
	_, _ = pg.Exec(`DELETE FROM tools WHERE key = $1`, extToolKey(toolName))
	_ = os.RemoveAll(filepath.Join(s.toolAssetDir(), toolName))
}

// repeatStr 拼一个重复串。
func repeatStr(ch string, n int) string {
	out := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, ch...)
	}
	return string(out)
}
