package server

// ═══ [审核流 · 自动处置契约提示词迁移测试] ═══
// 覆盖 upgradeReviewAutoContract 的三条分支：
//  1. 库里是旧内置正文 → 升级成新正文并写标记；
//  2. 用户改过那段 → 原样保留、不覆盖，仍写标记（视为已处理，避免每次启动重试）；
//  3. 标记已存在 → 直接返回，不再动库（幂等）。
// 二开日期：2026-11-21

import (
	"strings"
	"testing"

	"artex/agent"
	"artex/db"
)

// reviewPromptAgent 取 review agent；不存在则跳过（该部署没启用审核域）。
func reviewPromptAgent(t *testing.T, pg *db.DB) *db.Agent {
	t.Helper()
	a, err := pg.GetAgentByKey("review")
	if err != nil {
		t.Fatalf("GetAgentByKey(review): %v", err)
	}
	if a == nil {
		t.Skip("review agent 未初始化")
	}
	return a
}

// restoreReviewPrompt 把 review 提示词与迁移标记还原到测试前状态。
func restoreReviewPrompt(t *testing.T, pg *db.DB, agentID int64) {
	t.Helper()
	old, err := pg.CurrentPrompt(agentID)
	if err != nil {
		t.Fatalf("snapshot CurrentPrompt: %v", err)
	}
	flag, hadFlag, err := pg.GetSetting(reviewAutoContractSetting)
	if err != nil {
		t.Fatalf("snapshot flag: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pg.ResetPromptToDefault(agentID, old)
		if hadFlag {
			_ = pg.SetSetting(reviewAutoContractSetting, flag)
		} else {
			_, _ = pg.Exec(`DELETE FROM settings WHERE key=$1`, reviewAutoContractSetting)
		}
	})
}

func TestReviewAutoContractMigration(t *testing.T) {
	s, m := findingMigrationServer(t)
	pg := m.pg
	a := reviewPromptAgent(t, pg)
	restoreReviewPrompt(t, pg, a.ID)

	seed := agent.BuiltinPromptSeeds()["review"]

	t.Run("UpgradesLegacyBody", func(t *testing.T) {
		// 造出「旧正文」：把本次新增的两条说明整行删掉，恢复成契约前的原样。
		// 删掉后 "- 只对能明确判断…" 与 "- 自动轮同样只有只读工具" 重新相邻，
		// 正好等于 reviewPromptPrevFrag —— 迁移就是靠这个窄片段认旧正文的。
		const (
			newLine1 = "- 省略该 id 的后果要知道：这一轮判断过的意图不会再被反复研判（服务端记账，避免空烧令牌），所以「不输出」意味着它这一版内容基本就停在待审池等人了。除非确实拿不准，否则不要省。\n"
			newLine2 = "- 判断偏保守的边界：这条意图如果照字面执行，会不会动业务数据、关审计、留后门、打 DoS？会就 reject（写清怎么改）。纯探测、只读、与目标直接相关的 approve。\n"
		)
		legacy := seed
		for _, line := range []string{newLine1, newLine2} {
			if !strings.Contains(legacy, line) {
				t.Fatalf("新正文里找不到本次新增的说明行，请同步更新本测试：%q", line[:20])
			}
			legacy = strings.Replace(legacy, line, "", 1)
		}
		if strings.Contains(legacy, "省略该 id 的后果") {
			t.Fatalf("构造旧正文失败：仍残留新契约文本")
		}
		if !strings.Contains(legacy, reviewPromptPrevFrag) {
			t.Fatalf("构造出的旧正文不匹配 reviewPromptPrevFrag（%q）——迁移的旧正文识别条件失效", reviewPromptPrevFrag[:30])
		}

		if _, err := pg.ResetPromptToDefault(a.ID, legacy); err != nil {
			t.Fatalf("write legacy prompt: %v", err)
		}
		if _, err := pg.Exec(`DELETE FROM settings WHERE key=$1`, reviewAutoContractSetting); err != nil {
			t.Fatal(err)
		}

		s.upgradeReviewAutoContract()

		got, err := pg.CurrentPrompt(a.ID)
		if err != nil {
			t.Fatalf("CurrentPrompt: %v", err)
		}
		if got != seed {
			t.Fatalf("旧内置正文应被升级为新正文")
		}
		if v, _, _ := pg.GetSetting(reviewAutoContractSetting); v != "true" {
			t.Fatalf("迁移完成应写入标记，实际 %q", v)
		}
	})

	t.Run("RespectsCustomBody", func(t *testing.T) {
		custom := "# 我方自定义审核提示词\n只按公司的白名单裁决。"
		if _, err := pg.ResetPromptToDefault(a.ID, custom); err != nil {
			t.Fatalf("write custom prompt: %v", err)
		}
		if _, err := pg.Exec(`DELETE FROM settings WHERE key=$1`, reviewAutoContractSetting); err != nil {
			t.Fatal(err)
		}

		s.upgradeReviewAutoContract()

		got, err := pg.CurrentPrompt(a.ID)
		if err != nil {
			t.Fatalf("CurrentPrompt: %v", err)
		}
		if got != custom {
			t.Fatalf("自定义正文不得被覆盖，实际变成 %q", got)
		}
		if v, _, _ := pg.GetSetting(reviewAutoContractSetting); v != "true" {
			t.Fatalf("即便跳过覆盖也应收敛标记，避免每次启动重试，实际 %q", v)
		}
	})

	t.Run("IdempotentWhenFlagSet", func(t *testing.T) {
		if _, err := pg.ResetPromptToDefault(a.ID, seed); err != nil {
			t.Fatalf("write seed prompt: %v", err)
		}
		if err := pg.SetSetting(reviewAutoContractSetting, "true"); err != nil {
			t.Fatal(err)
		}
		// 手动改成别的内容，迁移不应因标记存在而覆盖它。
		after := "# 标记已存在后的手工改稿"
		if _, err := pg.ResetPromptToDefault(a.ID, after); err != nil {
			t.Fatalf("write after-flag prompt: %v", err)
		}

		s.upgradeReviewAutoContract()

		got, err := pg.CurrentPrompt(a.ID)
		if err != nil {
			t.Fatalf("CurrentPrompt: %v", err)
		}
		if got != after {
			t.Fatalf("标记已置位时迁移不应动库，实际 %q", got)
		}
	})
}
