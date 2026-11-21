package reportx

// MD 交付稿信息头的字段完整度。
//
// docx 封面有「客户名称 / 项目名称 / 报告版本 / 报告日期」四行（见
// pybuilder/report_builder.py:604 的 info 表）。而 md 信息头最初只写了出具方/日期/
// 版本/漏洞统计——客户名与项目名双双缺失。后果：同一份报告，走 docx 能看出受测方是
// 谁，走 md 就看不出来；偏偏 md 是本机没有 python-docx 时的唯一可用导出格式
// （docx 会 501），于是「缺依赖的机器上导出的报告一定缺客户名」。
//
// 这里直接钉 markdownHeader 这个纯函数（BuildMarkdown 会触发 LLM 叙述，不适合做单测
// 的边界）。渲染正文的其余部分不在这里覆盖。

import (
	"strings"
	"testing"
)

func testMeta(loc string) docMeta {
	return docMeta{
		CompanyName: "安全服务团队",
		ClientName:  "某某银行股份有限公司",
		ProjectName: "核心业务系统",
		ReportTitle: "核心业务系统 渗透测试报告",
		Version:     "V1.0",
		Date:        "2026年11月22日",
		Locale:      loc,
	}
}

// TestMarkdownHeaderHasClientAndProject md 信息头必须带客户名与项目名。
func TestMarkdownHeaderHasClientAndProject(t *testing.T) {
	h := markdownHeader(testMeta("zh-CN"), mdCounts{})

	if !strings.Contains(h, "某某银行股份有限公司") {
		t.Errorf("md 信息头缺少客户名称。实际：\n%s", h)
	}
	if !strings.Contains(h, "核心业务系统") {
		t.Errorf("md 信息头缺少项目名称。实际：\n%s", h)
	}
}

// TestMarkdownHeaderLabels 客户名/项目名要带标签——只出现孤立字符串，读者无法判断含义。
func TestMarkdownHeaderLabels(t *testing.T) {
	h := markdownHeader(testMeta("zh-CN"), mdCounts{})

	if !strings.Contains(h, "客户名称") {
		t.Errorf("缺少「客户名称」标签，客户名成了无上下文的裸值。实际：\n%s", h)
	}
	if !strings.Contains(h, "项目名称") {
		t.Errorf("缺少「项目名称」标签。实际：\n%s", h)
	}
}

// TestMarkdownHeaderEnglish 英文 locale 下标签要跟着走，不能硬编码中文。
func TestMarkdownHeaderEnglish(t *testing.T) {
	m := testMeta("en-US")
	m.CompanyName, m.ClientName, m.ProjectName = "Security Services Team", "Acme Bank", "Core Banking"
	m.ReportTitle = "Core Banking Penetration Test Report"

	h := markdownHeader(m, mdCounts{})

	if !strings.Contains(h, "Acme Bank") || !strings.Contains(h, "Client") {
		t.Errorf("英文报告应含 Client 标签与客户名。实际：\n%s", h)
	}
	if strings.Contains(h, "客户名称") || strings.Contains(h, "项目名称") {
		t.Errorf("英文报告不应出现中文标签。实际：\n%s", h)
	}
}

// TestMarkdownHeaderOmitsEmptyClient 客户名为空时不写「客户名称：」空行。
func TestMarkdownHeaderOmitsEmptyClient(t *testing.T) {
	m := testMeta("zh-CN")
	m.ClientName = ""

	if h := markdownHeader(m, mdCounts{}); strings.Contains(h, "客户名称") {
		t.Errorf("客户名为空时不应输出该行。实际：\n%s", h)
	}
}

// TestMarkdownHeaderCounts 统计行要把四档计数都带上，方便一眼看出规模。
func TestMarkdownHeaderCounts(t *testing.T) {
	h := markdownHeader(testMeta("zh-CN"), mdCounts{Crit: 1, High: 2, Med: 3, Low: 4})

	for _, want := range []string{"严重 1", "高危 2", "中危 3", "低危 4", "共 10"} {
		if !strings.Contains(h, want) {
			t.Errorf("统计行缺少 %q。实际：\n%s", want, h)
		}
	}
}
