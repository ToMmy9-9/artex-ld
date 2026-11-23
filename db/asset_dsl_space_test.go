package db

import "testing"

// TestDSLAllowsSpacesAroundOperator 固化「字段表达式允许空格」的解析行为。
//
// 背景：用户按直觉写 `ip = 1.2.3.4`（字段名与运算符之间、运算符与值之间
// 各有一个空格）时，解析器会放弃字段表达式、把 `ip` 当成裸文本，最终报
// 「DSL 语法错误：意外的内容 'BARE'」。同一份查询只因为多打一个空格就失败，
// 属于典型的静默踩雷，必须锁死。
func TestDSLAllowsSpacesAroundOperator(t *testing.T) {
	cases := []struct {
		dsl   string
		field string
		op    string
		value string
	}{
		{`ip=198.51.100.11`, "ip", "=", "198.51.100.11"},
		{`ip = 198.51.100.11`, "ip", "=", "198.51.100.11"},
		{`ip = "198.51.100.11"`, "ip", "=", "198.51.100.11"},
		{`ip="198.51.100.11"`, "ip", "=", "198.51.100.11"},
		{`domain = example.com`, "domain", "=", "example.com"},
		{`domain=example.com`, "domain", "=", "example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.dsl, func(t *testing.T) {
			expr, end, ok := tryParseFieldExpr(tc.dsl, 0)
			if !ok {
				t.Fatalf("解析失败：%q", tc.dsl)
			}
			if expr.Field != tc.field || expr.Op != tc.op || expr.Value != tc.value {
				t.Fatalf("解析结果 %+v，期望 field=%q op=%q value=%q", expr, tc.field, tc.op, tc.value)
			}
			if end != len(tc.dsl) {
				t.Fatalf("消费位置 %d，期望 %d（剩余部分未被解析）", end, len(tc.dsl))
			}
		})
	}
}

// TestDSLParsesWholeExpression 确认带空格的表达式能走完整个 ParseDSL →
// buildDSLWhere 链路，而不只是 tryParseFieldExpr 这一层。
func TestDSLParsesWholeExpression(t *testing.T) {
	for _, dsl := range []string{`ip = 198.51.100.11`, `ip=198.51.100.11`} {
		node, err := ParseDSL(dsl)
		if err != nil {
			t.Fatalf("ParseDSL(%q) 失败：%v", dsl, err)
		}
		where, args, err := buildDSLWhere(node)
		if err != nil {
			t.Fatalf("buildDSLWhere(%q) 失败：%v", dsl, err)
		}
		if where == "" || len(args) == 0 {
			t.Fatalf("ParseDSL(%q) 产出空条件：where=%q args=%v", dsl, where, args)
		}
	}
}
