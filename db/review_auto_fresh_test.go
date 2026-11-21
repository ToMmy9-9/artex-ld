package db

// ═══ [审核流 · 自动处置 fresh 判据回归测试] ═══
// 场景：任务先开意图审核（人工审，预审轮只写建议），再打开「自动处置」。
// 预审轮会把 reviewed_version 写成审到的 content_version，而 content_version 不变；
// 于是 FreshPendingIntents 的「reviewed_version < content_version」判据判定它们不再 fresh，
// 自动处置轮的 FreshIDs 为空 —— 这些意图永远不会被自动放行/驳回，只能在面板上堆着。
//
// 本测试锁定该行为，防止「先把建议写满，再开自动处置」时静默空转。
// 二开日期：2026-11-21

import (
	"testing"
)

// TestPrescreenMakesIntentsInvisibleToAutoDecide 验证：人工预审写过建议后，
// 同一批意图不再出现在 fresh 集合里 —— 即打开自动处置也不会被处置。
func TestPrescreenMakesIntentsInvisibleToAutoDecide(t *testing.T) {
	_, es := setupReviewFlowExploration(t)

	id := mustAddPendingReview(t, es, "被预审过就该仍待自动处置", "worker")
	n0, err := es.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if n0.ContentVersion != 0 {
		t.Fatalf("新建意图 content_version 应为 0，实际 %d", n0.ContentVersion)
	}

	// 人工全审模式下预审轮 pass：只写建议 + reviewed_version，不批准。
	ok, err := es.SetPrescreenPassNote(id, "与目标相关，建议放行", n0.ContentVersion)
	if err != nil || !ok {
		t.Fatalf("SetPrescreenPassNote: ok=%v err=%v", ok, err)
	}

	n1, err := es.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode after prescreen: %v", err)
	}
	if n1.ContentVersion != n0.ContentVersion {
		t.Fatalf("预审属审核元数据，不应推 content_version：before=%d after=%d",
			n0.ContentVersion, n1.ContentVersion)
	}
	if n1.State != "proposed" {
		t.Fatalf("预审后意图必须仍是 proposed，实际 %s", n1.State)
	}

	// 关键断言：预审建议写完，这条意图对「自动处置轮」不再 fresh。
	fresh, err := es.FreshPendingIntents(50)
	if err != nil {
		t.Fatalf("FreshPendingIntents: %v", err)
	}
	for _, n := range fresh {
		if n.ID == id {
			t.Fatalf("意图 #%d 被预审过后仍算 fresh —— 与本测试的前提不符，"+
				"说明预审落账没有写 reviewed_version，请复查 SetPrescreenPassNote", id)
		}
	}

	// 意图确实还在待审队列里（没被丢掉），所以「堆叠」是可观察的用户现象。
	pend, err := es.ProposedIntents(50)
	if err != nil {
		t.Fatalf("ProposedIntents: %v", err)
	}
	found := false
	for _, n := range pend {
		if n.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("意图 #%d 应仍在待审队列中（ProposedIntents），否则不构成堆叠", id)
	}
}

// TestFreshPendingIntentsRejectsSameVersion 直接锁定 fresh 判据的边界条件：
// reviewed_version == content_version 时不算 fresh（这是上面场景的根因公式）。
func TestFreshPendingIntentsRejectsSameVersion(t *testing.T) {
	_, es := setupReviewFlowExploration(t)

	id := mustAddPendingReview(t, es, "fresh 判据边界", "")
	n, err := es.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	// revise 落账同样写 reviewed_version=审到的版本。
	if ok, err := es.SetPrescreenVerdict(id, "建议补验证步骤", n.ContentVersion); err != nil || !ok {
		t.Fatalf("SetPrescreenVerdict: ok=%v err=%v", ok, err)
	}

	fresh, err := es.FreshPendingIntents(50)
	if err != nil {
		t.Fatalf("FreshPendingIntents: %v", err)
	}
	for _, f := range fresh {
		if f.ID == id {
			t.Fatalf("reviewed_version == content_version 时不应算 fresh（意图 #%d）", id)
		}
	}
}
