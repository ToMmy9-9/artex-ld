// name_test.go：回归 —— 漏洞的可读名称必须进入交付报告。
//
// 背景：report_finding 工具一直有 name 参数（漏洞名称），RecordFindingTx 也把它
// 写进了探索节点 payload（"name"）与 findings.name 列；但报告侧只解析了
// vulnclass，FindingInput 也没有 Name 字段。结果交付给客户的报告里，
// 「漏洞名称」一栏显示的是机器口径的类别代码（sqli / rce），而不是人写的
// 名称（「SQL 注入」）。这个缺陷来自 AntiFox 上游（其 report/ 同样缺 Name），
// 合并后仍未修。
//
// 这里锁死回退链：AI 富化标题 > 漏洞可读名 > vulnclass > 摘要。
package reportx

import "testing"

func TestFindingTitlePrefersReadableName(t *testing.T) {
	cases := []struct {
		name string
		in   FindingInput
		enr  Enrichment
		want string
	}{
		{
			name: "AI 富化标题优先",
			in:   FindingInput{VulnClass: "sqli", Name: "SQL 注入", Summary: "订单查询接口存在注入"},
			enr:  Enrichment{Title: "订单查询接口 SQL 注入"},
			want: "订单查询接口 SQL 注入",
		},
		{
			name: "无富化时用可读名称",
			in:   FindingInput{VulnClass: "sqli", Name: "SQL 注入", Summary: "订单查询接口存在注入"},
			want: "SQL 注入",
		},
		{
			name: "既无富化也无名称时回退 vulnclass",
			in:   FindingInput{VulnClass: "sqli", Summary: "订单查询接口存在注入"},
			want: "sqli",
		},
		{
			name: "全空时回退摘要",
			in:   FindingInput{Summary: "订单查询接口存在注入"},
			want: "订单查询接口存在注入",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanTitle(nz(tc.enr.Title, nz(tc.in.Name, nz(tc.in.VulnClass, tc.in.Summary))))
			if got != tc.want {
				t.Fatalf("标题 = %q，期望 %q", got, tc.want)
			}
		})
	}
}
