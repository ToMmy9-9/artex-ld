// executor_catalog.go：向 planner 告知"本任务可以派哪些执行器"。
//
// 现状的断层：planner 被要求在意图里填 target_executor，checkExecutor 也会
// 校验它填的 key 合法——但 planner 从来没被告知**有哪些 executor 可选**、
// **各自擅长什么**。它只能凭空猜一个 key，猜错就被拒。
//
// 这里把可用的 executor（自定义 agent 且声明了 executor_key 的）渲染成一段
// 清单注入 planner 的 system prompt，让它能按意图类型选人。
//
// 渲染内容：executor_key、agent 名称、能力描述（取 agent 的 description）、
// 以及该 executor 绑定的 skill（如果能查到），让 planner 判断"这条意图该派谁"。
package server

import (
	"fmt"
	"sort"
	"strings"

	"artex/db"
)

// executorCatalog renders the list of dispatchable executors.
//
// Only agents that declare an executor_key are dispatchable by intent —
// built-in agents are forbidden from declaring one (DB constraint), and a
// custom agent without a key has no executor identity.
func (s *Server) executorCatalog() string {
	pg := s.m.pg
	if pg == nil {
		return ""
	}
	agents, err := pg.ListAgents()
	if err != nil {
		return ""
	}
	type entry struct {
		key   string
		name  string
		desc  string
		skills []string
	}
	var entries []entry
	for _, a := range agents {
		if a.ExecutorKey == "" || !a.Enabled {
			continue
		}
		e := entry{key: a.ExecutorKey, name: a.Name, desc: a.Description}
		e.skills = s.skillsForAgent(a.ID)
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })

	var b strings.Builder
	b.WriteString("\n\n【可用执行器（可在意图的 target_executor 里指定）】：\n")
	b.WriteString("以下是本平台可被派遣的执行器。生成意图时，如果该方向适合其中某个执行器的专长，" +
		"就填它的 executor_key；没有合适的就留空（走默认执行者）。\n")
	for _, e := range entries {
		b.WriteString(fmt.Sprintf("- %s（%s）", e.key, e.name))
		if e.desc != "" {
			b.WriteString(": " + e.desc)
		}
		if len(e.skills) > 0 {
			b.WriteString("｜可用技能: " + strings.Join(e.skills, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// skillsForAgent lists the skill names visible to an agent.
//
// Best-effort: a failure here must not break the planner prompt, so an
// unreadable visibility table just yields no skill list.
func (s *Server) skillsForAgent(agentID int64) []string {
	pg := s.m.pg
	if pg == nil || agentID <= 0 {
		return nil
	}
	rows, err := pg.AgentSkillNames(agentID)
	if err != nil {
		return nil
	}
	sort.Strings(rows)
	return rows
}

var _ = db.EmptyScopeMatcher
