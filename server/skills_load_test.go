package server

import (
	"testing"

	"norma/skill"
)

// TestBundledSkillsAllLoad 验证仓库内置的全部 skill 都能被加载器解析：
// 名字/描述/正文齐全，且不与已有的重复。
func TestBundledSkillsAllLoad(t *testing.T) {
	reg, err := skill.LoadDir("../skills")
	if err != nil {
		t.Fatal(err)
	}
	all := reg.List()
	t.Logf("loaded %d skills", len(all))
	for _, s := range all {
		if s.Name == "" {
			t.Errorf("skill with empty name: %+v", s)
			continue
		}
		if s.Description == "" {
			t.Errorf("skill %s has empty description", s.Name)
		}
		if len(s.Instructions) == 0 {
			t.Errorf("skill %s has empty instructions", s.Name)
		}
		t.Logf("  %-32s %q (%dB)", s.Name, s.Description, len(s.Instructions))
	}
	if len(all) < 20 {
		t.Errorf("loaded %d skills, want >= 20", len(all))
	}
}
