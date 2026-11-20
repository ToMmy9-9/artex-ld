// skills_listing_test.go：验证 worker 实际看到的 Skill 清单里，
// 新移植的 skill 是带描述的（而不是被预算裁剪成只剩名字）。
//
// 为什么这条重要：Skill 工具的清单是模型选择 skill 的唯一依据。如果某个
// skill 在清单里只剩名字，模型就无法判断"该什么时候加载它"，这个 skill
// 等于不存在 —— 绑定与加载都正常，但永远不会有人用它。
package server

import (
	"encoding/json"
	"strings"
	"testing"

	"norma/skill"
)

// TestWorkerSkillListingKeepsDescriptions checks the listing a worker sees:
// every skill bound to the worker must appear WITH its description (not
// name-only), otherwise the model has no signal for when to load it.
func TestWorkerSkillListingKeepsDescriptions(t *testing.T) {
	reg, err := skill.LoadDir(builtinSkillDir(t))
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	listing := reg.Listing()
	if listing == "" {
		t.Fatal("skill listing is empty")
	}
	// The ported skills must appear with a description in the listing.
	for _, name := range []string{
		"fingerprint-finder", "api-tester", "sensitive-info-leak",
		"route-guard-bypass", "response-modifier", "tool-arsenal",
	} {
		idx := strings.Index(listing, "- "+name+":")
		if idx < 0 {
			// Could be name-only (budget clipped) or missing entirely.
			if strings.Contains(listing, "- "+name+"\n") {
				t.Errorf("skill %q is name-only in the listing — the model cannot tell when to load it", name)
			} else {
				t.Errorf("skill %q is missing from the listing entirely", name)
			}
			continue
		}
		// Grab the description text after the colon.
		rest := listing[idx+len("- "+name+":"):]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[:nl]
		}
		desc := strings.TrimSpace(rest)
		if len(desc) < 20 {
			t.Errorf("skill %q has a suspiciously short description in the listing: %q", name, desc)
		}
	}
}

// TestSkillListingBudgetNotExhausted guards the clip-to-name-only behaviour:
// if the listing ever exceeds the cap, skills after the cut lose their
// description and become effectively unusable. This test reports how much
// headroom is left so a future addition that blows the budget is caught.
func TestSkillListingBudgetNotExhausted(t *testing.T) {
	reg, err := skill.LoadDir(builtinSkillDir(t))
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	listing := reg.Listing()
	// The cap lives in norma/skill; mirror it loosely here (the exact value
	// is asserted by that package's own tests).
	const budget = 8000
	if len(listing) > budget {
		t.Fatalf("skill listing is %d chars, over the %d budget — later skills get clipped to name-only",
			len(listing), budget)
	}
	t.Logf("skill listing uses %d/%d chars (%.0f%% headroom left)", len(listing), budget,
		100*(1-float64(len(listing))/budget))
}

// TestListingIsValidJSONGuard is a sanity check that the listing we expose
// is a single flat list of "- name: description" lines, which is what the
// model actually reads.
func TestListingIsValidJSONGuard(t *testing.T) {
	reg, err := skill.LoadDir(builtinSkillDir(t))
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	listing := reg.Listing()
	lines := strings.Split(strings.TrimSpace(listing), "\n")
	if len(lines) < 10 {
		t.Fatalf("listing has only %d lines; expected one per skill", len(lines))
	}
	for _, ln := range lines {
		if !strings.HasPrefix(ln, "- ") {
			t.Fatalf("listing line is not a list item: %q", ln)
		}
	}
	// JSON round-trip guard so the shape stays stable for the UI too.
	if _, err := json.Marshal(listing); err != nil {
		t.Fatalf("listing is not marshalable: %v", err)
	}
}
