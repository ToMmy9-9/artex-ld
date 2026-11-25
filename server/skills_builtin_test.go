// skills_builtin_test.go：内置 skill 目录的加载与适配约束测试。
//
// 这批 skill 移植自 Claude Code 工程（见 docs/HOST-ADAPTATION.txt）。
// 方法论内容可以随上游更新，但"宿主适配"部分有硬约束：skill 正文里不能
// 残留 Claude Code 专属的 MCP 工具名或路径，否则 agent 会去找不存在的工具。
package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"norma/skill"
)

// builtinSkillDir 是内置 skill 的根目录（相对于 server 包）。
func builtinSkillDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "skills"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return dir
}

// portedSkills 是本次移植进 artex-ld 的 skill。
var portedSkills = []string{
	"fingerprint-finder",
	"api-tester",
	"sensitive-info-leak",
	"route-guard-bypass",
	"response-modifier",
	"tool-arsenal",
	// api-authz：越权双身份差分方法论（BOLA/BFLA），不在上游 PTS 集合里，
	// 由本仓库新增，仍按内置 skill 的加载/绑定约束走同一套测试。
	"api-authz",
}

// TestSkillsDirHasNoStrayMarkdown guards against stray .md files in the skills
// root. LoadDir registers EVERY .md file as a skill, so a note like
// "HOST-ADAPTATION.md" would show up in the agent's skill listing as an
// invokable skill with an empty description — noise the model must reason
// about on every turn.
func TestSkillsDirHasNoStrayMarkdown(t *testing.T) {
	root := builtinSkillDir(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read skills dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".md") {
			t.Errorf("stray markdown in skills root: %s (it would be registered as a skill)", e.Name())
		}
	}
}

// TestBuiltinSkillsLoad 验证所有内置 skill 都能被 LoadDir 正确解析：
// 有 name、description、非空指令正文。解析失败时 agent 根本看不到这个 skill。
func TestBuiltinSkillsLoad(t *testing.T) {
	reg, err := skill.LoadDir(builtinSkillDir(t))
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	byName := map[string]skill.Skill{}
	for _, s := range reg.List() {
		byName[s.Name] = s
	}
	for _, name := range portedSkills {
		s, ok := byName[name]
		if !ok {
			t.Errorf("skill %q not loaded from %s", name, builtinSkillDir(t))
			continue
		}
		if s.Description == "" {
			t.Errorf("skill %q has an empty description (agents cannot tell when to use it)", name)
		}
		if strings.TrimSpace(s.Instructions) == "" {
			t.Errorf("skill %q has empty instructions", name)
		}
		if s.Dir == "" {
			t.Errorf("skill %q has no Dir (references would be unreachable)", name)
		}
	}
}

// TestPortedSkillsHaveNoClaudeCodeRemnants 验证移植后的 skill 正文里
// 不残留 Claude Code 专属的 MCP 工具名与路径。
//
// 残留的后果不是"不好看"，而是 agent 会去调用这些不存在的工具
// （mcp__yakit__* / mcp__js-reverse-mcp__*），浪费轮次或直接失败。
func TestPortedSkillsHaveNoClaudeCodeRemnants(t *testing.T) {
	banned := []string{
		"mcp__yakit__",
		"mcp__js-reverse-mcp__",
		"mcp__memory-search-mcp__",
		".claude/skills/",
		".claude/mcp/",
		"CLAUDE.md",
	}
	root := builtinSkillDir(t)
	for _, name := range portedSkills {
		dir := filepath.Join(root, name)
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if info.Name() == "__pycache__" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".md") {
				return nil
			}
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			body := string(data)
			for _, b := range banned {
				if strings.Contains(body, b) {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s still references %q — the agent would call a tool that does not exist", rel, b)
				}
			}
			return nil
		})
		if err != nil {
			t.Errorf("walk %s: %v", dir, err)
		}
	}
}

// TestPortedSkillReferencesResolve 验证 skill 正文里引用的 references 文件
// 都真实存在。移植时若漏拷 references 或改错相对路径，agent 读到的
// 指引会指向不存在的文件。
func TestPortedSkillReferencesResolve(t *testing.T) {
	root := builtinSkillDir(t)
	for _, name := range portedSkills {
		dir := filepath.Join(root, name)
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			base := filepath.Dir(path)
			for _, ref := range extractMDRefs(string(data)) {
				target := filepath.Join(base, filepath.FromSlash(ref))
				if _, statErr := os.Stat(target); statErr != nil {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s references %q which does not exist", rel, ref)
				}
			}
			return nil
		})
		if err != nil {
			t.Errorf("walk %s: %v", dir, err)
		}
	}
}

// extractMDRefs pulls markdown link targets that point at .md files.
func extractMDRefs(body string) []string {
	var out []string
	rest := body
	for {
		i := strings.Index(rest, "](")
		if i < 0 {
			return out
		}
		rest = rest[i+2:]
		j := strings.Index(rest, ")")
		if j < 0 {
			return out
		}
		target := rest[:j]
		rest = rest[j+1:]
		// strip an optional anchor and title
		if k := strings.IndexByte(target, '#'); k >= 0 {
			target = target[:k]
		}
		if k := strings.IndexAny(target, " \t"); k >= 0 {
			target = target[:k]
		}
		if target == "" || strings.Contains(target, "://") || !strings.HasSuffix(target, ".md") {
			continue
		}
		out = append(out, target)
	}
}
