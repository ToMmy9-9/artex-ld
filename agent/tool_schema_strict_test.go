package agent

import (
	"encoding/json"
	"testing"

	actool "norma/tool"
)

// TestToolSchemasHaveConcreteArrayItems 验证：所有工具的 JSON Schema 里，
// 数组的 items 都写清了具体字段（properties），不能只写一个空的 {"type":"object"}。
//
// 为什么这是硬要求：部分供应商（MiniMax / Anthropic 兼容端点）对 schema 做**严格校验**
// ——items 只写 {"type":"object"} 而不带 properties 时，整个请求被直接判 400
// invalid params，模型一个 token 都拿不到。宽松一点的实现则退化成"模型自己猜字段"，
// 实测出现过工具调用参数为空（{}）→ 工具报"summary 不能为空"→ 模型重试仍传空参
// → 整轮规划作废（planner 一轮的推理全部丢弃，任务原地踏步）。
//
// 严格校验的供应商直接拒绝；宽松的供应商让模型瞎猜 —— 两边都是坏结果。
// 所以数组元素的字段必须在 schema 里写死。
func TestToolSchemasHaveConcreteArrayItems(t *testing.T) {
	ts := NewToolSet(nil, "worker")
	all := collectToolSchemas(ts)
	if len(all) == 0 {
		t.Fatal("no tool schemas collected — the enumeration itself is broken")
	}
	for _, entry := range all {
		walkSchemaForEmptyItems(t, entry.name, entry.schema)
	}
}

// schemaEntry is one tool's name + input schema.
type schemaEntry struct {
	name   string
	schema map[string]any
}

// collectToolSchemas enumerates every tool the agent can expose to a model.
func collectToolSchemas(ts *ToolSet) []schemaEntry {
	var out []schemaEntry
	seen := map[string]bool{}
	add := func(list []actool.CoreTool) {
		for _, tool := range list {
			if tool == nil {
				continue
			}
			name := tool.Name()
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, schemaEntry{name: name, schema: tool.InputSchema()})
		}
	}
	add(ts.PlannerTools())
	add(ts.WorkerTools())
	return out
}

// walkSchemaForEmptyItems recursively checks every array in the schema.
func walkSchemaForEmptyItems(t *testing.T, path string, node any) {
	t.Helper()
	switch v := node.(type) {
	case map[string]any:
		if t2, ok := v["type"].(string); ok && t2 == "array" {
			items, has := v["items"]
			if !has {
				t.Errorf("%s: array without items", path)
			} else if im, ok := items.(map[string]any); ok {
				// 只有「元素是对象」的数组才需要 properties；标量数组
				// （{"type":"integer"}）不需要，那是完全合法的 schema。
				if it, isObj := im["type"].(string); isObj && it == "object" {
					if _, hasProps := im["properties"]; !hasProps {
						t.Errorf("%s: array items has no properties — strict providers reject the whole request (400 invalid params), lenient ones make the model guess (empty tool args)", path)
					}
				}
			}
		}
		for k, child := range v {
			walkSchemaForEmptyItems(t, path+"."+k, child)
		}
	case []any:
		for _, child := range v {
			walkSchemaForEmptyItems(t, path+"[]", child)
		}
	}
}

// TestBatchItemsObjAlwaysCarriesProperties 验证 batchItemsObj 的行为：
// 一定带 properties；给了必填项就带 required。
func TestBatchItemsObjAlwaysCarriesProperties(t *testing.T) {
	raw, err := json.Marshal(batchItemsObj(map[string]any{
		"summary": str("一句话描述"),
	}, "summary"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["properties"]; !ok {
		t.Error("batchItemsObj must always carry properties")
	}
	req, ok := got["required"].([]any)
	if !ok || len(req) != 1 || req[0] != "summary" {
		t.Errorf("required = %v, want [summary]", got["required"])
	}
}
