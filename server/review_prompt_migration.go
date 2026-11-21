// review_prompt_migration.go：把「自动处置省略即永久留人」的新契约同步到存量 review 提示词。
//
// 背景：agent/promptcatalog.go 的 reviewDefaultTmpl 是代码兜底值，而
// seedPrompts 走 SeedPromptIfEmpty（首插 only）——库里已有 review 提示词的
// 部署永远不会拿到新正文。自动处置判据改成「研判过就不再重复研判」之后，
// 模型必须知道「省略该 id」的代价，否则仍会大范围省略、让意图静默堆在池里。
//
// 与 finding_prompt_migration.go 同模式：按 setting 标记只跑一次；
// 只升级「与上一版内置正文一致」的提示词，用户改过的不动。
// 二开日期：2026-11-21

package server

import (
	"log"
	"strings"

	"artex/agent"
)

// reviewAutoContractSetting 迁移完成标记。
const reviewAutoContractSetting = "review_auto_contract_v1"

// reviewPromptPrevFrag 是契约之前的自动处置段原文片段。
// 只比对这一小段而非整篇：整篇随时可能因别的原因变动，窄片段更不容易误伤用户编辑。
const reviewPromptPrevFrag = "- 只对能明确判断的 id 输出 approve / reject；拿不准就不输出该 id，留给人工。\n- 自动轮同样只有只读工具"

// upgradeReviewAutoContract 把新增的「省略代价」说明补进存量 review 提示词。
func (s *Server) upgradeReviewAutoContract() {
	pg := s.m.pg
	if pg == nil {
		return
	}
	if value, _, _ := pg.GetSetting(reviewAutoContractSetting); value == "true" {
		return
	}
	a, err := pg.GetAgentByKey("review")
	if err != nil || a == nil {
		// review agent 尚未初始化（或本部署不需要审核）：标记留空，下次启动再试。
		log.Printf("[prompts] review 自动处置契约迁移跳过: review agent 不可用 (%v)", err)
		return
	}
	cur, err := pg.CurrentPrompt(a.ID)
	if err != nil {
		log.Printf("[prompts] review 自动处置契约迁移失败: 读取当前提示词 %v", err)
		return
	}
	seed := agent.BuiltinPromptSeeds()["review"]
	if cur != seed {
		if !strings.Contains(cur, reviewPromptPrevFrag) {
			// 用户改过这段（或已是自定义正文）——尊重人的编辑，不覆盖。
			log.Printf("[prompts] review 提示词已被自定义，跳过自动处置契约迁移（如需新契约请手动同步）")
		} else if _, err := pg.ResetPromptToDefault(a.ID, seed); err != nil {
			log.Printf("[prompts] review 自动处置契约迁移失败: 写入 %v", err)
			return
		} else {
			log.Printf("[prompts] review 提示词已升级：自动处置新增「省略该 id 将被视为放弃处置」的代价说明")
		}
	}
	if err := pg.SetSetting(reviewAutoContractSetting, "true"); err != nil {
		log.Printf("[prompts] review 契约迁移标记写入失败: %v", err)
	}
}
