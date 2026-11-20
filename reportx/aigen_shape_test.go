package reportx

import "testing"

// TestDecodeLLMJSONToleratesModelShapeDrift 验证：模型返回的字段形状与结构体不一致时，
// 内容仍要被完整取回，而不是整段丢弃。
//
// 实测（MiniMax-M3）：把 `"description": ["…"]`（我们要求的数组）返回成
// `"description": "…"`（单个字符串），并把 risk_analysis 写成 impact。
// Go 的 json.Unmarshal 遇到类型不符会记下错误但继续解后面的字段，随后返回该错误；
// 原实现 `_ = json.Unmarshal(...)` 把错误整个丢掉 —— 表现为"AI 分析永远失败"
// 却没有任何线索，且 title / risk 等已解出的字段也一并作废。
func TestDecodeLLMJSONToleratesModelShapeDrift(t *testing.T) {
	raw := `{"title":"未授权接口泄露员工邮箱","severity":"medium",
	"description":"应用对相关接口的访问控制不严格，未授权访问者可直接获取员工邮箱等敏感信息。",
	"impact":"攻击者无需登录即可批量收集员工邮箱。",
	"remediation":["对接口实施身份验证和授权控制。","移除响应中的敏感字段。"]}`

	var out Enrichment
	if err := decodeLLMJSON(raw, &out); err != nil {
		t.Fatalf("decode should tolerate the shape drift, got: %v", err)
	}
	if out.Title != "未授权接口泄露员工邮箱" {
		t.Errorf("title = %q", out.Title)
	}
	if len(out.Description) != 1 || out.Description[0] == "" {
		t.Errorf("description = %v, want the string form wrapped into one element", out.Description)
	}
	if len(out.RiskAnalysis) != 1 || out.RiskAnalysis[0] == "" {
		t.Errorf("risk_analysis = %v, want the `impact` alias mapped in", out.RiskAnalysis)
	}
	if len(out.Remediation) != 2 {
		t.Errorf("remediation = %v, want 2 entries", out.Remediation)
	}
}

// TestDecodeLLMJSONKeepsCompliantShapeUnchanged 验证：模型本来就照 schema 返回时，
// 结果与直接 json.Unmarshal 一致（容错不改变正常路径）。
func TestDecodeLLMJSONKeepsCompliantShapeUnchanged(t *testing.T) {
	raw := `{"title":"标题","description":["第一段","第二段"],"risk_analysis":["风险"],"remediation":["修复"]}`
	var out Enrichment
	if err := decodeLLMJSON(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Description) != 2 || out.Description[0] != "第一段" {
		t.Errorf("description = %v", out.Description)
	}
	if len(out.RiskAnalysis) != 1 || out.RiskAnalysis[0] != "风险" {
		t.Errorf("risk_analysis = %v", out.RiskAnalysis)
	}
}

// TestDecodeLLMJSONReportsUndecodableInput 验证：完全不是 JSON 时必须返回错误，
// 不能假装成功（否则调用方会把"空富化"当成"模型没写内容"）。
func TestDecodeLLMJSONReportsUndecodableInput(t *testing.T) {
	var out Enrichment
	if err := decodeLLMJSON("这不是 JSON", &out); err == nil {
		t.Error("non-JSON input must report an error")
	}
}
