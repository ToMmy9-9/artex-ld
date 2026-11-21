package db

// ═══ [审核流 · 自动处置判据分离回归测试] ═══
// 修复目标：人工预审写满 reviewed_version 后，再打开「自动处置」时，
// 自动轮必须仍能看到这批意图并处置它们，而不是因为沿用预审计次判据
// 而永久空转、让意图堆在待审池。
// 二开日期：2026-11-21

import "testing"

// TestAutoDecideSeesPrescreenedIntents 是本次修复的核心回归：
// 预审过 ≠ 自动处置过。两个判据必须互不影响。
func TestAutoDecideSeesPrescreenedIntents(t *testing.T) {
	_, es := setupReviewFlowExploration(t)

	id := mustAddPendingReview(t, es, "预审过之后自动处置仍应看到我", "worker")
	n, err := es.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	// 阶段一：人工全审模式的预审轮 —— 写建议 + reviewed_version，意图仍 pending。
	if ok, err := es.SetPrescreenPassNote(id, "建议放行", n.ContentVersion); err != nil || !ok {
		t.Fatalf("SetPrescreenPassNote: ok=%v err=%v", ok, err)
	}

	// 预审判据下它已经「审过」，不应再进预审轮（省 LLM）。
	fresh, err := es.FreshPendingIntents(50)
	if err != nil {
		t.Fatalf("FreshPendingIntents: %v", err)
	}
	for _, f := range fresh {
		if f.ID == id {
			t.Fatalf("预审计次判据应已排除 #%d", id)
		}
	}

	// 阶段二：此刻打开「自动处置」。自动处置判据必须仍能看到它 —— 这是修复点。
	auto, err := es.AutoDecidePendingIntents(50)
	if err != nil {
		t.Fatalf("AutoDecidePendingIntents: %v", err)
	}
	found := false
	for _, a := range auto {
		if a.ID == id {
			found = true
			if a.ContentVersion != n.ContentVersion {
				t.Fatalf("自动处置候选携带版本 %d，期望 %d", a.ContentVersion, n.ContentVersion)
			}
			break
		}
	}
	if !found {
		t.Fatalf("意图 #%d 被预审后仍应进入自动处置候选（修复前就是这里空转，导致意图堆叠）", id)
	}
}

// TestMarkAutoDecidedStopsRepeatJudge 验证研判标记生效：
// 标记后同一版本不再进候选（避免 15s 一轮反复烧 LLM），正文变更后重新进候选。
func TestMarkAutoDecidedStopsRepeatJudge(t *testing.T) {
	_, es := setupReviewFlowExploration(t)

	id := mustAddPendingReview(t, es, "标记后不该被反复研判", "")
	n, err := es.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	ok, err := es.MarkAutoDecided(id, n.ContentVersion)
	if err != nil || !ok {
		t.Fatalf("MarkAutoDecided: ok=%v err=%v", ok, err)
	}

	auto, err := es.AutoDecidePendingIntents(50)
	if err != nil {
		t.Fatalf("AutoDecidePendingIntents: %v", err)
	}
	for _, a := range auto {
		if a.ID == id {
			t.Fatalf("已标记研判过的 #%d 不应再进候选（会无限重复调 LLM）", id)
		}
	}

	// 正文被改（版本前进）→ 必须重新进候选，否则改稿后的意图永远得不到自动处置。
	// pending 稿不允许走 UpdateIntent（状态门禁），这里直接推 content_version
	// 模拟「内容变了」这一事实——判据只看版本号，与本测试的意图一致。
	if _, err := es.db.Exec(`UPDATE exploration_nodes SET content_version=content_version+1
WHERE id=$1 AND exploration_id=$2`, id, es.expID); err != nil {
		t.Fatalf("bump content_version: %v", err)
	}
	after, err := es.AutoDecidePendingIntents(50)
	if err != nil {
		t.Fatalf("AutoDecidePendingIntents after edit: %v", err)
	}
	found := false
	for _, a := range after {
		if a.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("正文改版后 #%d 应重新进入自动处置候选", id)
	}
}

// TestMarkAutoDecidedDoesNotBumpVersion 审核元数据不得推 content_version，
// 否则「标记已研判」这个纯记账动作会把自己变成新一轮需研判的工作。
func TestMarkAutoDecidedDoesNotBumpVersion(t *testing.T) {
	_, es := setupReviewFlowExploration(t)

	id := mustAddPendingReview(t, es, "标记不推版本", "")
	n, err := es.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if ok, err := es.MarkAutoDecided(id, n.ContentVersion); err != nil || !ok {
		t.Fatalf("MarkAutoDecided: ok=%v err=%v", ok, err)
	}
	after, err := es.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode after: %v", err)
	}
	if after.ContentVersion != n.ContentVersion {
		t.Fatalf("标记研判不应推 content_version：before=%d after=%d", n.ContentVersion, after.ContentVersion)
	}
	if after.State != "proposed" {
		t.Fatalf("标记研判不应改状态，实际 %s", after.State)
	}
}

// TestMarkAutoDecidedIgnoresAlreadyDecided 人工抢先处置后，自动轮迟到的标记必须静默失效。
func TestMarkAutoDecidedIgnoresAlreadyDecided(t *testing.T) {
	_, es := setupReviewFlowExploration(t)

	id := mustAddPendingReview(t, es, "人工已处置", "worker")
	n, err := es.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if ok, err := es.ApproveIntentAt(id, n.ContentVersion); err != nil || !ok {
		t.Fatalf("ApproveIntentAt: ok=%v err=%v", ok, err)
	}
	ok, err := es.MarkAutoDecided(id, n.ContentVersion)
	if err != nil {
		t.Fatalf("MarkAutoDecided 对已处置意图不应报错（自动轮本就不该覆盖人工）：%v", err)
	}
	if ok {
		t.Fatalf("已批准转 open 的意图不该被标记为已自动研判")
	}
}
