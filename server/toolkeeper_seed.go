// toolkeeper_seed.go：内置「工具管理员」agent（tool_keeper）的种子注册。
//
// 为什么要有这个 agent：用户上传外部工具后不应该还要自己去填一堆参数规则。
// 上传即触发这个 agent 认领工具——它读用法、判断类型与风险、登记调用示例，
// 并产出权限契约。平台负责强制执行，agent 只负责"理解"。
//
// 信任边界：agent 产出的契约只能是平台权限上限的**子集**（见
// db.ValidateContractAgainstLimits），放宽权限的权利留在平台代码里。
package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"

	"artex/agent"
)

const (
	toolKeeperAgentKey = "tool_keeper"
	toolKeeperDesc     = "内部工具管理员：认领用户上传的外部工具，判定其能力与风险，登记受管调用契约。"
	toolKeeperSeedFlag = "tool_keeper_seeded_v1"
)

// seedToolKeeper registers the internal tool-keeper agent, its default prompt,
// and the binding of its three dedicated tools.
//
// Idempotent: a settings flag short-circuits repeat seeding, and every insert
// is ON CONFLICT-safe so a user-defined agent that already owns the key is
// never overwritten.
func (s *Server) seedToolKeeper() {
	pg := s.m.pg
	if pg == nil {
		return
	}
	tx, err := pg.Begin()
	if err != nil {
		log.Printf("[tool-keeper] seed 事务开启失败: %v", err)
		return
	}
	defer tx.Rollback()

	// Serialize concurrent seeds (server restart storm) on one advisory lock.
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(7734201101)`); err != nil {
		log.Printf("[tool-keeper] seed 加锁失败: %v", err)
		return
	}
	var done string
	err = tx.QueryRow(`SELECT value FROM settings WHERE key=$1`, toolKeeperSeedFlag).Scan(&done)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("[tool-keeper] seed 读取标记失败: %v", err)
		return
	}
	if done == "true" {
		return
	}

	// 1) the agent itself
	var agentID int64
	err = tx.QueryRow(`INSERT INTO agents(key,name,description,role,builtin,enabled)
		VALUES ($1,'工具管理员',$2,'assistant',false,true)
		ON CONFLICT (key) DO NOTHING RETURNING id`,
		toolKeeperAgentKey, toolKeeperDesc).Scan(&agentID)
	if errors.Is(err, sql.ErrNoRows) {
		// A user-defined agent already owns the key — leave it alone but still
		// make sure the tools are bound, so the feature works either way.
		if err = tx.QueryRow(`SELECT id FROM agents WHERE key=$1`, toolKeeperAgentKey).Scan(&agentID); err != nil {
			log.Printf("[tool-keeper] 取已存在 agent 失败: %v", err)
			return
		}
	}
	if err != nil {
		log.Printf("[tool-keeper] 建 agent 失败: %v", err)
		return
	}

	// 2) its default prompt
	var promptID int64
	if err = tx.QueryRow(`SELECT current_prompt_id FROM agents WHERE id=$1`, agentID).Scan(&promptID); err != nil ||
		promptID == 0 {
		if err = tx.QueryRow(`INSERT INTO agent_prompts(agent_id,version,template_text,note,updated_by)
			VALUES ($1,1,$2,'内置默认','system') RETURNING id`,
			agentID, agent.ToolKeeperDefaultPrompt).Scan(&promptID); err != nil {
			log.Printf("[tool-keeper] seed prompt 失败: %v", err)
			return
		}
		if _, err = tx.Exec(`UPDATE agents SET current_prompt_id=$1,
			trigger_run_mode='serial', trigger_merge_mode='by_task', trigger_max_parallel=0
			WHERE id=$2`, promptID, agentID); err != nil {
			log.Printf("[tool-keeper] 配置 agent 失败: %v", err)
			return
		}
	}

	// 3) bind its three tools to it ONLY
	for _, toolKey := range []string{"list_tool_assets", "probe_tool", "claim_tool"} {
		if _, err = tx.Exec(`UPDATE tools SET agents=agents || to_jsonb($2::text)
			WHERE key=$1 AND NOT (agents ? $2)`, toolKey, toolKeeperAgentKey); err != nil {
			log.Printf("[tool-keeper] 绑定工具 %s 失败: %v", toolKey, err)
			return
		}
	}

	if _, err = tx.Exec(`INSERT INTO settings(key,value) VALUES ($1,'true')
		ON CONFLICT (key) DO UPDATE SET value='true'`, toolKeeperSeedFlag); err != nil {
		log.Printf("[tool-keeper] 写标记失败: %v", err)
		return
	}
	if err = tx.Commit(); err != nil {
		log.Printf("[tool-keeper] seed 提交失败: %v", err)
		return
	}
	log.Printf("[tool-keeper] 内置工具管理员已就绪")
}

// seedToolKeeperTools makes sure the three keeper tools exist in the tools
// table with their schemas and the tool_keeper binding.
//
// SeedTool uses ON CONFLICT DO NOTHING, so a description change would not
// propagate on an existing DB — we upsert the schema/description explicitly.
func (s *Server) seedToolKeeperTools() {
	pg := s.m.pg
	if pg == nil {
		return
	}
	tools := s.toolKeeperTools()
	for _, t := range tools {
		key := t.Name()
		schema, err := json.Marshal(t.InputSchema())
		if err != nil {
			log.Printf("[tool-keeper] 工具 %s schema 取值失败: %v", key, err)
			continue
		}
		if err := pg.UpsertSystemToolExclusive(key, t.Description(), schema, nil, "builtin",
			[]string{toolKeeperAgentKey}); err != nil {
			log.Printf("[tool-keeper] 工具 %s 入库失败: %v", key, err)
		}
	}
}


// seedPlannerSkillVisibility binds a small set of method skills to the planner.
//
// The planner is the decision maker: it reads the exploration graph and
// decides what to dispatch next. Without any skill in its tool set it cannot
// even load a skill, so it has no idea the platform ships method skills like
// fingerprint-finder or api-tester — and therefore never dispatches work
// that would use them.
//
// Only a few are bound: the planner's context budget goes to the graph, not
// to methodology. The worker keeps the full set.
func (s *Server) seedPlannerSkillVisibility() {
	pg := s.m.pg
	if pg == nil {
		return
	}
	planner, err := pg.GetAgentByKey("planner")
	if err != nil || planner == nil {
		return
	}
	// Append, never replace: SetAgentSkillVisibility is a full replace and
	// would wipe whatever the user configured for the planner in the UI.
	// ToggleSkillVisibility only flips one row, so the user's own picks
	// survive.
	for _, name := range []string{"fingerprint-finder", "api-tester", "sensitive-info-leak",
		"route-guard-bypass", "tool-arsenal"} {
		if err := pg.ToggleSkillVisibility(planner.ID, name, true); err != nil {
			log.Printf("[planner-skill] 绑定 %s 失败: %v", name, err)
		}
	}
}

// toolKeeperSeedAll is the one-call entry used at server startup.
func (s *Server) toolKeeperSeedAll() {
	s.seedToolKeeperTools()
	s.seedToolKeeper()
	s.seedPlannerSkillVisibility()
}
