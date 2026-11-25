// asset_neighbors_tools_test.go：钉住 asset_neighbors 在三个 agent 工具集里都存在。
//
// 背景：planner 提示词早就写了「需要细节可调 asset_neighbors」，但工具此前根本
// 不存在——模型按提示调一个不存在的工具，只会得到 "unknown tool"。这条用例防止
// 提示词与工具集再次脱节。
package agent

import (
	"testing"

	actool "norma/tool"
)

func TestAssetNeighborsInAllToolSets(t *testing.T) {
	ts := NewToolSet(nil, "planner")
	for _, tc := range []struct {
		label string
		tools []string
	}{
		{"planner", toolNames(ts.PlannerTools())},
		{"worker", toolNames(ts.WorkerTools())},
		{"mainagent", toolNames(ts.MainAgentTools())},
	} {
		found := false
		for _, n := range tc.tools {
			if n == "asset_neighbors" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s tool set is missing asset_neighbors", tc.label)
		}
	}
}

func toolNames(in []actool.CoreTool) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		out = append(out, t.Name())
	}
	return out
}
