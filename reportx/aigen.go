package reportx

// AI narrative layer: turns thin findings ({vulnclass, severity, summary, poc})
// into professional Chinese report prose (description / test process / risk /
// remediation + CVSS) via the platform's active LLM profile. Report-level and
// per-finding calls run concurrently. All AI output is JSON; parse failures fall
// back to the raw finding fields (assembly in build.go), so generation never
// hard-fails on a bad LLM response.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"artex/agent"
	"artex/db"
	"norma/llm"
)

// complete runs one system+user → text completion on the report-role LLM profile.
// Returns an error when no profile is configured (caller falls back to skeleton).
//
// 【合并说明】AntiFox 原实现走其自研 engine/agentcore.Run；本仓库保留 ARTEX 原生的
// planner-worker 双图架构（无 engine/ 目录），故这里改用 artex-ld 的
// agent.Config → llm.Provider → Complete 单轮调用。语义等价：一次 system + 一次 user、
// 无工具、只取正文，且**不依赖任何被重写的 agent 内核**。
func complete(ctx context.Context, pg *db.DB, system, user string, maxTokens int) (string, error) {
	// 报告/富化角色可绑定到指定 LLM(角色分工)——未绑定则用全局激活。让富化能走更强的模型
	// (如 Claude)而不动渗透 agent 的模型。
	p, err := pg.ReportRoleProfile()
	if err != nil || p == nil || strings.TrimSpace(p.APIKey) == "" {
		return "", fmt.Errorf("no active LLM profile")
	}
	cfg := agent.ConfigFrom(p.Format, p.Model, p.BaseURL, p.APIKey, p.Proxy)
	cfg.RatePerSecond, cfg.RatePerMinute = p.RatePerSecond, p.RatePerMinute
	cfg.ReasoningEffort = p.ReasoningEffort
	prov, err := cfg.NewProvider()
	if err != nil {
		return "", err
	}
	cctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	msg, _, _, err := prov.Complete(cctx, llm.CompletionRequest{
		System:    []string{system},
		Messages:  []llm.Message{llm.UserText(user)},
		MaxTokens: maxTokens,
		Thinking:  "disabled",
	})
	if err != nil {
		return "", err
	}
	return cleanJSON(msg.Text()), nil
}

// ---------------------------------------------------------------------------
// Report-level narrative (background / service result / remediation / checklist)
// ---------------------------------------------------------------------------

type reportLevelOut struct {
	Background            []string `json:"background"`
	ServiceResult         string   `json:"service_result"`
	RemediationConclusion string   `json:"remediation_conclusion"`
	ChecklistHits         []string `json:"checklist_hits"`
}

const sysReportLevel = "你是%s的资深渗透测试报告撰写专家，擅长撰写专业、客户可读、技术严谨的中文渗透测试报告。严格只输出 JSON，不要 markdown 代码块。"

const userReportLevel = `根据以下项目与漏洞清单撰写报告综述内容。

项目名称：%s
客户名称：%s
漏洞清单：
%s
要求（专业交付口吻，不要出现"本次测试于今天"之类措辞）：
- background：字符串数组，2 段。第一段说明客户委托%s开展渗透测试及测试目标；第二段说明测试范围与人工+工具结合的方式。每段 60-130 字。
- service_result：1 段，概述测试了哪些系统、发现漏洞的数量与严重度分布。
- remediation_conclusion：1 段整改结论（首轮渗透默认全部待修复，建议尽快整改并复测）。
- checklist_hits：字符串数组，根据漏洞类型推断命中的标准测试项名称（例如 "SQL 注入"、"未授权访问"、"登录认证绕过"、"敏感信息泄露"、"命令注入"、"跨站脚本攻击（XSS）"、"垂直越权访问" 等标准名称）。

只输出 JSON：{"background":["...","..."],"service_result":"...","remediation_conclusion":"...","checklist_hits":["...","..."]}`

// reportLevelPromptData feeds the editable report-level prompt template.
type reportLevelPromptData struct {
	Company, Project, Client, FindingsList, LangDirective string
}

// vulnPromptData feeds the editable per-vuln prompt template.
type vulnPromptData struct {
	Company, VulnClass, Name, Severity, Summary, PoC, LangDirective string
}

func aiReportLevel(ctx context.Context, pg *db.DB, in *GenInput, sk *skillContent) reportLevelOut {
	var out reportLevelOut
	loc := normLocale(in.Locale)
	var sb strings.Builder
	// 把完整测试范围/目标系统作为上下文喂给叙述(经 FindingsList 通道，skill/兜底两条 prompt
	// 路径都会带上)，这样综述能点名具体系统——同时封面项目名保持短(ProjectName)不受影响。
	if sc := strings.TrimSpace(in.Scope); sc != "" {
		fmt.Fprintf(&sb, "【测试范围/目标系统】%s\n\n【漏洞清单】\n", truncateRune(sc, 1500))
	}
	for _, f := range in.Findings {
		fmt.Fprintf(&sb, "- [%s] %s：%s\n", severityName(loc, f.Severity), nz(f.VulnClass, "-"), truncateRune(f.Summary, 80))
	}
	dir := outputLangDirective(loc)
	data := reportLevelPromptData{Company: in.CompanyName, Project: in.ProjectName, Client: in.ClientName, FindingsList: sb.String(), LangDirective: dir}
	sys := renderPrompt(sk.reportLevelSys, data)
	if sys == "" {
		sys = fmt.Sprintf(sysReportLevel, in.CompanyName)
	}
	user := renderPrompt(sk.reportLevelUser, data)
	if user == "" {
		user = fmt.Sprintf(userReportLevel, in.ProjectName, in.ClientName, sb.String(), in.CompanyName) +
			"\n\n输出语言要求：" + dir
	}
	raw, err := complete(ctx, pg, sys, user, 4000)
	if err != nil {
		return out
	}
	if err := decodeLLMJSON(raw, &out); err != nil {
		// 模型没给出可解析的 JSON：记一笔，别让"AI 分析永远失败"变成无迹可寻。
		log.Printf("[report] 报告级叙述 JSON 解析失败: %v (原始输出前 200 字: %s)", err, clipText(raw, 200))
	}
	return out
}

// ---------------------------------------------------------------------------
// Per-finding narrative (description / test process / risk / remediation / CVSS)
// ---------------------------------------------------------------------------

type Enrichment struct {
	Title        string            `json:"title"`  // concise professional vuln name
	Target       string            `json:"target"` // affected host / URL, extracted from evidence
	Description  []string          `json:"description"`
	Module       string            `json:"module"`
	TestProcess  []procBlock       `json:"test_process"`
	RiskAnalysis []string          `json:"risk_analysis"`
	Remediation  []string          `json:"remediation"`
	CVSSScore    float64           `json:"cvss_score"`
	CVSSVector   string            `json:"cvss_vector"`
	CVSSMetrics  map[string]string `json:"cvss_metrics"`
}

const sysVuln = "你是%s的资深渗透测试报告撰写专家。把漏洞原始记录改写为【交付级】报告正文——像资深顾问亲笔写给客户，精炼、专业、就事论事。严格只输出 JSON，不要 markdown 代码块，不要客套话、免责声明或套话。HTTP 请求/命令/响应等原文必须真实保留，不得编造。"

const userVuln = `把下面这条漏洞改写成报告正文。核心要求：【精炼、具体、克制】——宁缺毋滥，不凑条数、不注水、不说正确的废话；每一句都要是针对本目标本漏洞的真信息。

漏洞类型/原始名称：%s
严重程度：%s
原始摘要：
%s
原始复现过程/证据：
%s

字段要求：
- title：规范、专业的漏洞名称（10-26 字）。原始名已规范（含 CVE 等）则保留或微调，不要照抄摘要。
- target：受影响主机或 URL，从摘要/证据中提取；无法确定留空字符串。
- description：1 段 2-3 句，只说清这是什么漏洞、出在哪个点、本质成因。不堆 OWASP 分类、不复述修复、不空泛铺垫。
- module：所属功能模块（简短，如"外部账户登录 / 未授权接口"）。
- risk_analysis：2-4 条，每条一句，必须是针对本目标本漏洞的具体后果（谁、能做什么、拿到什么数据或权限），不要套话。
- remediation：2-4 条，每条一句，具体可落地（改哪个点、怎么改、校验什么），不要空话。
- CVSS 3.1：cvss_score(数字)、cvss_vector(如 "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H")、cvss_metrics（八项英文取值）。

不要输出 test_process——复现过程由平台用真实证据直接生成，返回 "test_process":[] 即可。

只输出 JSON：{"title":"...","target":"...","description":["..."],"module":"...","test_process":[],"risk_analysis":["..."],"remediation":["..."],"cvss_score":0,"cvss_vector":"...","cvss_metrics":{...}}`

func aiVulnNarrative(ctx context.Context, pg *db.DB, locale string, f FindingInput, companyName string, sk *skillContent) Enrichment {
	var out Enrichment
	loc := normLocale(locale)
	dir := outputLangDirective(loc)
	// Name 是可读漏洞名（report_finding 的 name）。AI 改写标题时优先参考它，
	// 否则模型只能看到 "sqli" 这类机器口径的类别代码。
	data := vulnPromptData{Company: companyName, VulnClass: nz(f.VulnClass, "-"), Name: nz(f.Name, ""), Severity: severityName(loc, f.Severity), Summary: nz(f.Summary, "-"), PoC: nz(f.PoC, "-"), LangDirective: dir}
	sys := renderPrompt(sk.vulnSys, data)
	if sys == "" {
		sys = fmt.Sprintf(sysVuln, companyName)
	}
	user := renderPrompt(sk.vulnUser, data)
	if user == "" {
		user = fmt.Sprintf(userVuln,
			firstNonEmpty(f.Name, nz(f.VulnClass, "-")), severityName(loc, f.Severity), nz(f.Summary, "-"), nz(f.PoC, "-")) +
			"\n\n输出语言要求：" + dir
	}
	// Ground the PROSE (description / risk / remediation) in the ACTUAL work transcript
	// (real tool calls / requests / responses / analysis recorded when the finding was
	// made) so it reflects what actually happened, not an expansion of the one-liner.
	if tr := strings.TrimSpace(f.Transcript); tr != "" {
		user += "\n\n=== 发现该漏洞时的【真实工作转录】(工具调用/请求/响应/分析) ===\n" +
			"description/risk_analysis 依据转录中的真实现象来写，不得编造或臆测转录中没有的内容。\n\n" + tr
	}
	// PROSE-ONLY, ALWAYS. We never ask the model to reconstruct a verbatim test_process:
	// that large generation is slow and unreliable (many models time out / return nothing
	// on it) AND risks fabricating a PoC (the user's key pain). Instead test_process is
	// filled DETERMINISTICALLY below from REAL evidence — the agent's in-situ 复现过程 if
	// present, else the raw PoC. The model only writes prose (title/desc/module/risk/
	// remediation/CVSS), which is small, fast, and reliable on any model.
	if len(f.Reproduction) > 2 { // ""/"{}" carry nothing
		user += "\n\n=== 复现过程已由测试引擎就地记录 ===\n" +
			"本漏洞的入口/前置/步骤/来源已在测试时【就地记录】，将直接用于报告的『测试过程』。你【无需也不要】重建 test_process，直接返回 \"test_process\":[]。"
	} else {
		user += "\n\n=== test_process 由真实证据直接提供 ===\n" +
			"『测试过程』将由平台用【真实 PoC/证据原文】直接生成，你【无需也不要】重建或编造 test_process，直接返回 \"test_process\":[]。"
	}
	user += "请集中把 title / description / module / risk_analysis / remediation 与 CVSS 写扎实、专业，依据上方摘要与转录里的真实现象，不得编造转录中没有的内容。"
	raw, err := complete(ctx, pg, sys, user, 4000)
	if err != nil {
		return out
	}
	if err := decodeLLMJSON(raw, &out); err != nil {
		// 解析失败仍保留已解出的字段（title/risk/remediation 等）—— 比整段丢弃强。
		log.Printf("[report] 漏洞叙述 JSON 解析失败: %v (原始输出前 200 字: %s)", err, clipText(raw, 200))
	}
	// Fill test_process from REAL evidence, never the model's reconstruction: the agent's
	// in-situ 复现过程 wins; else the raw PoC (deterministic). This makes every consumer
	// (persisted enrichment → findings page UI, and the report) show real, non-fabricated
	// steps — and keeps the generation prose-only (small, reliable).
	if b := reproToBlocks(f.Reproduction, loc, nil); len(b) > 0 {
		out.TestProcess = b
	} else if len(out.TestProcess) == 0 && strings.TrimSpace(f.PoC) != "" {
		out.TestProcess = proofToBlocks(f.PoC)
	}
	return out
}

// EnrichFinding is the exported per-finding AI enrichment, shared by the report
// pipeline and the findings-management "AI 分析" endpoint. locale controls the
// output language (zh-CN | zh-TW | en-US); skillDir enables the editable
// report-generation prompt (falls back to the built-in prompt when unset/missing).
// Returns a zero Enrichment (not an error) when no LLM profile is configured —
// callers keep whatever raw fields the finding already has.
func EnrichFinding(ctx context.Context, pg *db.DB, skillDir, locale string, f FindingInput, companyName string) Enrichment {
	if strings.TrimSpace(companyName) == "" {
		companyName = pick(locale, "安全服务团队", "安全服務團隊", "Security Services Team")
	}
	sk := loadSkillContent(skillDir, locale)
	return aiVulnNarrative(ctx, pg, locale, f, companyName, sk)
}

// cleanJSON strips markdown fences and surrounding prose from an LLM response.
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1] // strip leading AND trailing prose around the JSON object
		}
	}
	return s
}

// decodeLLMJSON 把模型的 JSON 响应解进 out，并对**字段形状不一致**做容错。
//
// 为什么需要容错：模型并不总是照 schema 返回。实测 MiniMax-M3 会把
// `"description": ["…"]`（我们要求的数组）返回成 `"description": "…"`（单个字符串），
// 还可能把数组字段返回成对象、把字符串字段返回成数组。Go 的 json.Unmarshal 遇到
// 类型不符会**记下第一个类型错误、但继续解后面的字段**，随后返回那个错误。
//
// 原来的写法是 `_ = json.Unmarshal(...)` —— 错误被整个丢掉，调用方只看得到
// "description 是空的"，完全看不出是模型形状不对还是真没内容；更糟的是 title /
// risk_analysis 等字段虽然解出来了，却因为整段被视为失败而写不进库，表现为
// "AI 分析永远失败"却没有任何日志线索。
//
// 这里：先按原样解一次（拿到的字段都保留），再对形状不符的字段做逐个纠正
// （字符串 → 单元素数组；数组 → 取首个非空字符串拼成一段；对象 → 取常见文本键），
// 并把纠正动作记进日志，方便回头调 prompt。
func decodeLLMJSON(raw string, out any) error {
	firstErr := json.Unmarshal([]byte(raw), out)
	if firstErr == nil {
		return nil
	}
	// 逐字段纠正后重解一次。纠正必须**按目标类型**做：把 title 之类本该是字符串的
	// 字段也压成数组，同样解不出来（只是把一个类型错误换成另一个）。
	var generic map[string]json.RawMessage
	if jerr := json.Unmarshal([]byte(raw), &generic); jerr != nil {
		return firstErr // 连对象都不是，无法纠正
	}
	// 别名归位：模型偶尔用同义键名（risk_analysis → impact / risk，
	// description → desc / detail）而不是我们要求的键名。这类偏差不纠正，
	// 报告里对应的段落就是空的。
	for alias, canonical := range llmFieldAliases {
		if _, exists := generic[canonical]; exists {
			continue
		}
		if v, ok := generic[alias]; ok {
			generic[canonical] = v
		}
	}
	targets := llmFieldTypes(out)
	fixed := false
	for key, val := range generic {
		want, known := targets[key]
		if !known {
			continue // 不在结构体里的字段（模型多吐的）保持原样
		}
		norm := normalizeLLMField(val, want)
		if norm == nil {
			continue
		}
		generic[key] = *norm
		fixed = true
	}
	if !fixed {
		return firstErr
	}
	patched, merr := json.Marshal(generic)
	if merr != nil {
		return firstErr
	}
	if jerr := json.Unmarshal(patched, out); jerr != nil {
		return firstErr
	}
	return nil
}

// llmFieldAliases 是模型常用的同义键名 → 本仓库期望的键名。
// 模型不总按 prompt 里给的键名返回（实测 MiniMax-M3 把 risk_analysis 写成
// impact、description 写成 desc），不归位就等于丢掉整段内容。
var llmFieldAliases = map[string]string{
	"impact":       "risk_analysis",
	"risk":         "risk_analysis",
	"risks":        "risk_analysis",
	"desc":         "description",
	"detail":       "description",
	"details":      "description",
	"fix":          "remediation",
	"fixes":        "remediation",
	"advice":       "remediation",
	"suggestion":   "remediation",
	"recommend":    "remediation",
	"module_name":  "module",
	"vuln_class":   "title",
	"vuln_name":    "title",
	"name":         "title",
	"cvss_vector":  "cvss_vector",
	"cvss_metrics": "cvss_metrics",
}

// llmFieldTypes 列出目标结构体每个 JSON 字段的目标类型（"string" / "array"），
// 供纠正模型返回的形状时按类型归位。
func llmFieldTypes(out any) map[string]string {
	outT := reflect.TypeOf(out)
	if outT == nil || outT.Kind() != reflect.Pointer || outT.Elem().Kind() != reflect.Struct {
		return nil
	}
	res := map[string]string{}
	for i := 0; i < outT.Elem().NumField(); i++ {
		f := outT.Elem().Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			name = f.Name
		}
		switch f.Type.Kind() {
		case reflect.Slice:
			res[name] = "array"
		case reflect.String:
			res[name] = "string"
		}
	}
	return res
}

// normalizeLLMField 把"形状不对"的字段纠正成该平台约定的形状：
// 数组字段（[]string）→ 字符串/对象时压成单元素或拼接数组；
// 字符串字段 ← 数组时取首个非空元素；← 对象时取常见的文本键。
// 返回 nil 表示这个字段本来就合规，无需纠正。
func normalizeLLMField(val json.RawMessage, want string) *json.RawMessage {
	trimmed := strings.TrimSpace(string(val))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var arr []any
	if err := json.Unmarshal(val, &arr); err == nil {
		// 已经是数组。
		if want == "string" {
			// 目标是字符串：取首个非空元素。
			for _, item := range arr {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					out, merr := json.Marshal(s)
					if merr != nil {
						return nil
					}
					raw := json.RawMessage(out)
					return &raw
				}
			}
			return nil
		}
		// 目标是数组：每个元素转成字符串（对象取文本）。
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			switch v := item.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					parts = append(parts, v)
				}
			case map[string]any:
				if s := textFromMap(v); s != "" {
					parts = append(parts, s)
				}
			}
		}
		out, err := json.Marshal(parts)
		if err != nil {
			return nil
		}
		raw := json.RawMessage(out)
		return &raw
	}
	var obj map[string]any
	if err := json.Unmarshal(val, &obj); err == nil {
		// 是对象：目标是字符串就取常见的文本键；目标是数组则取不到正文，退回单元素。
		if s := textFromMap(obj); s != "" {
			if want == "array" {
				out, merr := json.Marshal([]string{s})
				if merr != nil {
					return nil
				}
				raw := json.RawMessage(out)
				return &raw
			}
			out, merr := json.Marshal(s)
			if merr != nil {
				return nil
			}
			raw := json.RawMessage(out)
			return &raw
		}
		return nil
	}
	// 是标量字符串：目标是数组时包成单元素数组。
	if want == "array" {
		var s string
		if err := json.Unmarshal(val, &s); err == nil && strings.TrimSpace(s) != "" {
			out, merr := json.Marshal([]string{s})
			if merr != nil {
				return nil
			}
			raw := json.RawMessage(out)
			return &raw
		}
	}
	return nil // 本来就合规
}

// textFromMap 从一个对象里挑出最像正文的文本值（按常见键名优先）。
func textFromMap(m map[string]any) string {
	for _, key := range []string{"text", "content", "description", "detail", "value", "summary", "body"} {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

// clipText 截断长文本用于日志（避免把整段模型输出灌进日志）。
func clipText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// firstNonEmpty 返回第一个非空（去空白后）的字符串。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
