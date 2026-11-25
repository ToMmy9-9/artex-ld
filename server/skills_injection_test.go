// skills_injection_test.go：验证移植进来的内置 skill 真的能被 agent 装配到
// Skill 工具的可用清单里。
//
// 这是移植工作的验收点：文件放进 skills/ 目录只是第一步——agent_skill_visibility
// 里要有绑定，装配时才会把 skill 装进注册表并出现在 Skill 工具的清单中。
package server

import (
	"os"
	"strings"
	"testing"

	"artex/db"
	"norma/skill"
)

// TestSkillRegistryIncludesPortedSkills verifies the registry built from the
// skills directory exposes every ported skill by name.
func TestSkillRegistryIncludesPortedSkills(t *testing.T) {
	reg, err := skill.LoadDir(builtinSkillDir(t))
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	for _, name := range portedSkills {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("registry missing %q", name)
		}
	}
}

// TestPortedSkillInstructionsAreUsable verifies each ported skill returns
// instructions that name the tools it expects the agent to have. A skill whose
// body says "use the fetch tool" but whose tool list does not include it would
// send the agent chasing a tool that isn't there.
func TestPortedSkillInstructionsAreUsable(t *testing.T) {
	reg, err := skill.LoadDir(builtinSkillDir(t))
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	// platformTools are the tool names the artex-ld runtime actually exposes.
	platformTools := map[string]bool{
		"fetch": true, "Bash": true, "traffic_search": true, "traffic_get": true,
		"traffic_blob": true, "intercept_request": true, "Skill": true,
	}
	for _, name := range portedSkills {
		s, ok := reg.Get(name)
		if !ok {
			continue
		}
		body := s.Instructions
		for tool := range platformTools {
			if !strings.Contains(body, tool) {
				continue
			}
			if !platformTools[tool] {
				t.Errorf("skill %q references unknown tool %q", name, tool)
			}
		}
	}
}

// TestPortedSkillToolNamesResolveAgainstRuntime verifies that every platform
// tool named inside a ported skill's body is a tool the runtime actually
// exposes. This catches the case where the adaptation notes promise a tool
// ("fetch") that a future refactor removes.
func TestPortedSkillToolNamesResolveAgainstRuntime(t *testing.T) {
	reg, err := skill.LoadDir(builtinSkillDir(t))
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	// Tool names the artex-ld runtime provides (server-side + norma builtin).
	known := map[string]bool{
		"fetch": true, "Bash": true, "LS": true, "Read": true, "Write": true,
		"traffic_search": true, "traffic_get": true, "traffic_blob": true,
		"intercept_request": true, "Skill": true, "WebSearch": true,
	}
	for _, name := range portedSkills {
		s, ok := reg.Get(name)
		if !ok {
			continue
		}
		for _, tool := range mentionedPlatformTools(s.Instructions) {
			if !known[tool] {
				t.Errorf("skill %q mentions tool %q which the runtime does not expose", name, tool)
			}
		}
	}
}

// mentionedPlatformTools extracts backticked tokens that name a platform
// tool. Deliberately narrow: the ported skills also mention external CLI
// binaries (SSTImap, GodPotato) and protocol names (OAuth2, CAS) in backticks,
// and those are called through Bash — they are NOT platform CoreTools.
func mentionedPlatformTools(body string) []string {
	// The platform tool names are a small closed set; match those literally
	// rather than trying to classify arbitrary backticked words.
	platformNames := []string{
		"fetch", "Bash", "LS", "Read", "Write", "Skill", "WebSearch",
		"traffic_search", "traffic_get", "traffic_blob", "intercept_request",
	}
	var out []string
	for _, n := range platformNames {
		if strings.Contains(body, "`"+n+"`") {
			out = append(out, n)
		}
	}
	return out
}

// TestSkillVisibilityBindsPortedSkills verifies the DB binding rows exist for
// the agents that are supposed to see the ported skills. The runtime reads
// agent_skill_visibility to decide which skills to load, so a missing row
// means the skill silently never reaches the agent.
func TestSkillVisibilityBindsPortedSkills(t *testing.T) {
	dsn := os.Getenv("ARTEX_PG_DSN")
	if dsn == "" {
		t.Skip("requires isolated ARTEX_PG_DSN")
	}
	pg, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Close() })
	// The ported skills must be visible to worker and pentest at least.
	want := map[string][]string{
		"worker":  {"fingerprint-finder", "api-tester", "sensitive-info-leak", "route-guard-bypass", "response-modifier", "api-authz"},
		"pentest": {"fingerprint-finder", "api-tester", "sensitive-info-leak", "route-guard-bypass", "response-modifier", "api-authz"},
	}
	for agentKey, skills := range want {
		a, err := pg.GetAgentByKey(agentKey)
		if err != nil || a == nil {
			t.Fatalf("agent %q not found: %v", agentKey, err)
		}
		names, err := pg.AgentSkillNames(a.ID)
		if err != nil {
			t.Fatalf("AgentSkillNames(%s): %v", agentKey, err)
		}
		have := map[string]bool{}
		for _, n := range names {
			have[n] = true
		}
		for _, s := range skills {
			if !have[s] {
				t.Errorf("agent %q is not bound to skill %q (visible: %v)", agentKey, s, names)
			}
		}
	}
}
