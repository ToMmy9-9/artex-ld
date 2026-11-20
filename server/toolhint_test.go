// toolhint_test.go：目录扫描型 shell 提示的扫描、渲染与校验测试。
package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"artex/db"
)

func writeHintFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestScanToolFilesGroupsByTopDir verifies executables are grouped by their
// first path segment and hidden dirs are skipped.
func TestScanToolFilesGroupsByTopDir(t *testing.T) {
	root := t.TempDir()
	writeHintFile(t, filepath.Join(root, "domain", "gau.exe"), "x")
	writeHintFile(t, filepath.Join(root, "domain", "katana.exe"), "x")
	writeHintFile(t, filepath.Join(root, "dirscan", "ffuf.exe"), "x")
	writeHintFile(t, filepath.Join(root, "readme.md"), "not a tool")
	writeHintFile(t, filepath.Join(root, ".hidden", "secret.exe"), "x")
	writeHintFile(t, filepath.Join(root, "nested", "deep", "deeper", "tool.exe"), "x")

	allowed := toolExecExtensions("windows", nil)
	files, total := scanToolFiles(root, 3, allowed)
	if total != 4 {
		t.Fatalf("total = %d, want 4 (hidden dir + md must be excluded): %+v", total, files)
	}
	groups := map[string]int{}
	for _, f := range files {
		groups[f.group]++
	}
	if groups["domain"] != 2 {
		t.Errorf("domain group = %d, want 2", groups["domain"])
	}
	if groups["dirscan"] != 1 {
		t.Errorf("dirscan group = %d, want 1", groups["dirscan"])
	}
	if _, ok := groups[".hidden"]; ok {
		t.Errorf("hidden dir leaked into groups: %+v", groups)
	}
}

// TestToolExecExtensionsPlatformFilter verifies the Windows/Linux split: a
// .exe must not be offered on a Linux deployment and vice versa. This is the
// bug that breaks "upload once, reuse everywhere" when the platform differs.
func TestToolExecExtensionsPlatformFilter(t *testing.T) {
	win := toolExecExtensions("windows", nil)
	if !win[".exe"] {
		t.Error("windows must accept .exe")
	}
	if win[".sh"] {
		t.Error("windows must not accept .sh by default")
	}
	lin := toolExecExtensions("linux", nil)
	if lin[".exe"] {
		t.Error("linux must not accept .exe by default")
	}
	if !lin[".sh"] {
		t.Error("linux must accept .sh")
	}
	// include extends the platform set without widening to the other platform
	ext := toolExecExtensions("linux", []string{"exe", ".JAR"})
	if !ext[".exe"] || !ext[".jar"] {
		t.Errorf("include should extend the set, got %+v", ext)
	}
}

// TestRenderShellHintDirectoryScan verifies the rendered hint carries the
// absolute root and the grouped listing, and that a static hint keeps its
// legacy "- key: desc" shape.
func TestRenderShellHintDirectoryScan(t *testing.T) {
	root := t.TempDir()
	writeHintFile(t, filepath.Join(root, "domain", "gau.exe"), "x")
	writeHintFile(t, filepath.Join(root, "vulnscan", "nuclei.exe"), "x")

	exec, _ := json.Marshal(map[string]any{"dir": root, "platform": "windows"})
	row := &db.Tool{Key: "arsenal", Kind: "shell", Exec: exec, Description: "攻防工具集"}
	out := renderShellHint(row, "arsenal", "攻防工具集")

	if !strings.Contains(out, root) {
		t.Errorf("rendered hint must carry the absolute root %q, got:\n%s", root, out)
	}
	if !strings.Contains(out, "domain/") || !strings.Contains(out, "gau.exe") {
		t.Errorf("rendered hint must list the grouped executables, got:\n%s", out)
	}
	if !strings.Contains(out, "2 个") {
		t.Errorf("rendered hint must report the real count, got:\n%s", out)
	}

	// static hint (no dir) keeps the legacy format
	static := &db.Tool{Key: "ffuf", Kind: "shell", Exec: json.RawMessage(`{}`), Description: "目录爆破"}
	if s := renderShellHint(static, "ffuf", "目录爆破"); s != "- ffuf: 目录爆破" {
		t.Errorf("static hint format changed: %q", s)
	}
}

// TestRenderShellHintMissingDir verifies a broken dir degrades to a readable
// line instead of an empty string (an empty hint would silently drop the tool).
func TestRenderShellHintMissingDir(t *testing.T) {
	exec, _ := json.Marshal(map[string]any{"dir": filepath.Join(t.TempDir(), "nope")})
	row := &db.Tool{Key: "arsenal", Kind: "shell", Exec: exec}
	out := renderShellHint(row, "arsenal", "")
	if out == "" {
		t.Fatal("missing dir must still render a non-empty hint")
	}
	if !strings.Contains(out, "不可访问") {
		t.Errorf("missing dir hint should say 不可访问, got:\n%s", out)
	}
}

// TestValidateShellHintDir verifies absolute-path enforcement: a relative dir
// would resolve differently per task working dir, so it must be rejected.
func TestValidateShellHintDir(t *testing.T) {
	mk := func(dir string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"dir": dir})
		return b
	}
	if err := validateShellHintDir("shell", json.RawMessage(`{}`)); err != nil {
		t.Errorf("no dir must pass: %v", err)
	}
	if err := validateShellHintDir("shell", mk("relative/path")); err == nil {
		t.Error("relative dir must be rejected")
	}
	if err := validateShellHintDir("shell", mk(t.TempDir())); err != nil {
		t.Errorf("absolute existing dir must pass: %v", err)
	}
	// a file is not a dir
	f := filepath.Join(t.TempDir(), "a.exe")
	writeHintFile(t, f, "x")
	if err := validateShellHintDir("shell", mk(f)); err == nil {
		t.Error("a file must be rejected as a dir")
	}
	// malformed exec JSON must be rejected, not silently accepted
	if err := validateShellHintDir("shell", json.RawMessage(`{not json`)); err == nil {
		t.Error("malformed exec JSON must be rejected")
	}
	// non-shell kinds are not validated (their exec has a different shape)
	if err := validateShellHintDir("command", json.RawMessage(`{not json`)); err != nil {
		t.Errorf("non-shell kinds must pass through: %v", err)
	}
}

// TestRejectModelDirScan verifies the model-facing create/update tools cannot
// create a directory-scan hint — that would let a model read a file inventory
// of any path on disk into its own context.
func TestRejectModelDirScan(t *testing.T) {
	if err := rejectModelDirScan("shell", json.RawMessage(`{"dir":"C:/Windows"}`)); err == nil {
		t.Error("model must not be able to set exec.dir on a shell tool")
	}
	if err := rejectModelDirScan("shell", json.RawMessage(`{}`)); err != nil {
		t.Errorf("shell without dir must pass: %v", err)
	}
	if err := rejectModelDirScan("command", json.RawMessage(`{"command":"ls"}`)); err != nil {
		t.Errorf("non-shell kinds must pass: %v", err)
	}
}

// TestScanToolFilesDepthLimit verifies max_depth bounds the walk consistently
// for BOTH directories and files: root/a/b/c/x.exe is visible at depth 3 while
// root/a/b/c/d/y.exe is not (the d directory itself is cut). A file must not
// slip past a limit that its containing directory already hit.
func TestScanToolFilesDepthLimit(t *testing.T) {
	root := t.TempDir()
	writeHintFile(t, filepath.Join(root, "a", "b", "c", "deep.exe"), "x")
	writeHintFile(t, filepath.Join(root, "a", "b", "c", "d", "deeper.exe"), "x")
	allowed := toolExecExtensions("windows", nil)

	_, total := scanToolFiles(root, 2, allowed)
	if total != 0 {
		t.Errorf("depth 2 must not reach a/b/c/deep.exe, got %d", total)
	}
	_, total = scanToolFiles(root, 3, allowed)
	if total != 1 {
		t.Errorf("depth 3 must reach a/b/c/deep.exe but not a/b/c/d/deeper.exe, got %d", total)
	}
	_, total = scanToolFiles(root, 4, allowed)
	if total != 2 {
		t.Errorf("depth 4 must reach both files, got %d", total)
	}
}

// TestToolHintCacheKeyDistinguishesParams verifies changing the scan params
// produces a different cache key (a stale listing would hide new tools).
func TestToolHintCacheKeyDistinguishesParams(t *testing.T) {
	a := shellHintSpec{Dir: "/x", MaxDepth: 3, MaxEntries: 120, Platform: "windows"}
	b := shellHintSpec{Dir: "/x", MaxDepth: 3, MaxEntries: 120, Platform: "linux"}
	if toolHintCacheKey(a) == toolHintCacheKey(b) {
		t.Error("platform change must change the cache key")
	}
}

// TestScanToolFilesSkipsNoiseDirs verifies dependency trees, test folders and
// package-init files never reach the listing. Real tool collections carry
// pyenv/site-packages, sqlmap/thirdparty and wafw00f/tests — listing those
// would flood the prompt with hundreds of non-tools.
func TestScanToolFilesSkipsNoiseDirs(t *testing.T) {
	root := t.TempDir()
	writeHintFile(t, filepath.Join(root, "pyenv", "Lib", "site-packages", "six.py"), "x")
	writeHintFile(t, filepath.Join(root, "sqlmap", "thirdparty", "six.py"), "x")
	writeHintFile(t, filepath.Join(root, "wafw00f", "tests", "test_detection.py"), "x")
	writeHintFile(t, filepath.Join(root, "pkg", "__init__.py"), "x")
	writeHintFile(t, filepath.Join(root, "pkg", "real_tool.py"), "x")
	writeHintFile(t, filepath.Join(root, "pkg", "test_thing.py"), "x")
	writeHintFile(t, filepath.Join(root, "pkg", "mod_test.py"), "x")

	allowed := toolExecExtensions("linux", nil)
	files, total := scanToolFiles(root, 3, allowed)
	if total != 1 {
		t.Fatalf("total = %d, want 1 (only pkg/real_tool.py): %+v", total, files)
	}
	if files[0].rel != "pkg/real_tool.py" {
		t.Errorf("kept file = %q, want pkg/real_tool.py", files[0].rel)
	}
}

// TestScanToolFilesPythonEntryOnly verifies python scripts are listed only as
// entry points: a group-level entry (sqlmap.py) and a leaf-dir script
// (linpeas.sh) are callable tools, while library modules nested deeper
// (lib/core/x.py) are not.
func TestScanToolFilesPythonEntryOnly(t *testing.T) {
	root := t.TempDir()
	writeHintFile(t, filepath.Join(root, "sqlmap-1.10", "sqlmap.py"), "x")
	writeHintFile(t, filepath.Join(root, "sqlmap-1.10", "lib", "core", "x.py"), "x")
	writeHintFile(t, filepath.Join(root, "sqlmap-1.10", "extra", "beep", "beep.py"), "x")
	writeHintFile(t, filepath.Join(root, "privesc", "PEASS-ng", "linpeas.sh"), "x")

	allowed := toolExecExtensions("linux", nil)
	files, total := scanToolFiles(root, 3, allowed)
	if total != 2 {
		t.Fatalf("total = %d, want 2 (sqlmap.py + linpeas.sh): %+v", total, files)
	}
	got := map[string]bool{}
	for _, f := range files {
		got[f.rel] = true
	}
	if !got["sqlmap-1.10/sqlmap.py"] {
		t.Error("group-level entry script sqlmap.py must be listed")
	}
	if !got["privesc/PEASS-ng/linpeas.sh"] {
		t.Error("leaf-dir script linpeas.sh must be listed")
	}
	for _, f := range files {
		if strings.Contains(f.rel, "lib/") || strings.Contains(f.rel, "extra/") {
			t.Errorf("nested library module leaked into the listing: %s", f.rel)
		}
	}
}

// TestRenderShellHintCollapsesHugeGroup verifies a group with hundreds of
// executables is folded into a count instead of one enormous line — the Bash
// description is carried on every LLM turn, so its size is a token cost.
func TestRenderShellHintCollapsesHugeGroup(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 60; i++ {
		writeHintFile(t, filepath.Join(root, "big", "tool"+itoa(i)+".exe"), "x")
	}
	exec, _ := json.Marshal(map[string]any{"dir": root, "platform": "windows"})
	row := &db.Tool{Key: "arsenal", Kind: "shell", Exec: exec}
	out := renderShellHint(row, "arsenal", "")

	if !strings.Contains(out, "共 60 个") {
		t.Errorf("huge group must report the real count, got:\n%s", out)
	}
	// the rendered line must stay short: a count + a capped name list
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 400 {
			t.Errorf("rendered line too long (%d chars):\n%s", len(line), line)
		}
	}
	// the group name must still appear so the model knows the directory exists
	if !strings.Contains(out, "big/") {
		t.Errorf("collapsed group must still name the directory, got:\n%s", out)
	}
}

// TestRenderShellHintStatesRealShell verifies the hint states which shell the
// Bash tool actually runs. The tool is named "Bash" but on Windows it is
// PowerShell — without this the model writes `ls -la` / `&&` and every call
// fails.
func TestRenderShellHintStatesRealShell(t *testing.T) {
	root := t.TempDir()
	writeHintFile(t, filepath.Join(root, "util", "jq.exe"), "x")
	exec, _ := json.Marshal(map[string]any{"dir": root, "platform": "windows"})
	row := &db.Tool{Key: "arsenal", Kind: "shell", Exec: exec}
	out := renderShellHint(row, "arsenal", "")

	if !strings.Contains(out, "命令语法") {
		t.Fatalf("hint must state the real shell, got:\n%s", out)
	}
	if runtime.GOOS == "windows" {
		if !strings.Contains(out, "PowerShell") {
			t.Errorf("windows hint must say PowerShell, got:\n%s", out)
		}
	} else if !strings.Contains(out, "bash -c") {
		t.Errorf("posix hint must say bash -c, got:\n%s", out)
	}
}

// TestScanToolFilesCurrentPlatformSanity makes sure the scan works on the
// platform the test actually runs on (guards against an empty-extension rule
// that would make every real-world tool invisible on Linux).
func TestScanToolFilesCurrentPlatformSanity(t *testing.T) {
	root := t.TempDir()
	name := "tool.exe"
	if runtime.GOOS != "windows" {
		name = "tool.sh"
	}
	writeHintFile(t, filepath.Join(root, name), "x")
	allowed := toolExecExtensions("", nil)
	_, total := scanToolFiles(root, 2, allowed)
	if total != 1 {
		t.Errorf("auto platform detection must find %s, got %d", name, total)
	}
}
