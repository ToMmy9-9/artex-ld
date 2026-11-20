package db

import (
	"testing"
	"time"
)

// TestFindingDeliveryTablesCascadeWithNode 验证 AntiFox 交付域三张表
// （finding_enrichment / finding_evidence / finding_traffic）随探索节点一起被清理。
//
// 这三张表以**探索节点 id** 为键。删任务时节点随 exploration 级联删除；若这三张表
// 没挂外键，就会留下一批永远读不到的孤儿行（node_id 指向已不存在的节点），
// 既占空间，也会让"这个漏洞还有富化/证据/流量关联"的查询出现幽灵数据。
func TestFindingDeliveryTablesCascadeWithNode(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	tk, err := d.CreateTask("交付域级联测试", "目标", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(tk.ID)

	es := d.Exploration(tk.ExplorationID)
	nodeID, err := es.AddNode(KindFinding, map[string]any{"summary": "x", "severity": "high"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	fid, err := d.AddFinding(tk.ID, nodeID, "SQL注入", "SQL 注入", "high", "summary", "poc", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}

	// 三张表各写一行。
	if err := d.UpsertFindingEnrichment(&FindingEnrichment{NodeID: nodeID, Name: "富化标题", Severity: "high"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateFindingEvidence(nodeID, "shot.png", "证据图注", 1); err != nil {
		t.Fatal(err)
	}
	if err := d.LinkFindingTraffic(nodeID, []string{"ex-1"}, "manual"); err != nil {
		t.Fatal(err)
	}

	count := func(q string) int64 {
		t.Helper()
		var n int64
		if err := d.QueryRow(q, nodeID).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", q, err)
		}
		return n
	}
	if got := count(`SELECT count(*) FROM finding_enrichment WHERE node_id=$1`); got != 1 {
		t.Fatalf("enrichment rows = %d, want 1", got)
	}
	if got := count(`SELECT count(*) FROM finding_evidence WHERE node_id=$1`); got != 1 {
		t.Fatalf("evidence rows = %d, want 1", got)
	}
	if got := count(`SELECT count(*) FROM finding_traffic WHERE node_id=$1`); got != 1 {
		t.Fatalf("finding_traffic rows = %d, want 1", got)
	}

	// 删漏洞 → 节点被删 → 三张表必须跟着清空（不留孤儿行）。
	if _, err := d.DeleteFinding(fid); err != nil {
		t.Fatal(err)
	}
	if got := count(`SELECT count(*) FROM finding_enrichment WHERE node_id=$1`); got != 0 {
		t.Errorf("enrichment rows after node delete = %d, want 0", got)
	}
	if got := count(`SELECT count(*) FROM finding_evidence WHERE node_id=$1`); got != 0 {
		t.Errorf("evidence rows after node delete = %d, want 0", got)
	}
	if got := count(`SELECT count(*) FROM finding_traffic WHERE node_id=$1`); got != 0 {
		t.Errorf("finding_traffic rows after node delete = %d, want 0", got)
	}
}

// TestFindingDeliveryTablesRejectUnknownNode 验证三张表拒绝挂到不存在的节点上。
// 外键的作用不只是清理，还包括**挡住写错 id 空间**：调用方可能拿独立 findings 表的
// 行 id 当节点 id 用，两个 id 空间并不相等 —— 写错时必须报错，而不是静默写到别的节点上。
func TestFindingDeliveryTablesRejectUnknownNode(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	const bogusNodeID = int64(1) << 40 // 远超任何真实序列值，必然不存在
	if err := d.UpsertFindingEnrichment(&FindingEnrichment{NodeID: bogusNodeID, Name: "x"}); err == nil {
		t.Error("enrichment upsert on unknown node should fail")
	}
	if _, err := d.CreateFindingEvidence(bogusNodeID, "x.png", "", 0); err == nil {
		t.Error("evidence insert on unknown node should fail")
	}
	if err := d.LinkFindingTraffic(bogusNodeID, []string{"ex-1"}, "manual"); err == nil {
		t.Error("finding_traffic insert on unknown node should fail")
	}
}

// TestTaskArchiveCapturesFindingDeliveryTables 验证归档快照带上了这三张表，
// 且恢复后能挂回节点（否则归档→恢复会丢交付域数据）。
func TestTaskArchiveCapturesFindingDeliveryTables(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()
	// 归档快照会读 llm_records / llm_usage（与既有归档测试同前提）。
	if err := d.EnsureLLMRecordsTable(); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}

	tk, err := d.CreateTask("交付域归档测试", "目标", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(tk.ID)

	es := d.Exploration(tk.ExplorationID)
	nodeID, err := es.AddNode(KindFinding, map[string]any{"summary": "x", "severity": "high"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddFinding(tk.ID, nodeID, "SQL注入", "SQL 注入", "high", "summary", "poc", "worker", nil); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertFindingEnrichment(&FindingEnrichment{NodeID: nodeID, Name: "富化标题", Severity: "high"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateFindingEvidence(nodeID, "shot.png", "证据图注", 1); err != nil {
		t.Fatal(err)
	}
	if err := d.LinkFindingTraffic(nodeID, []string{"ex-1"}, "manual"); err != nil {
		t.Fatal(err)
	}

	snap, err := d.SnapshotTaskArchive(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"finding_enrichment", "finding_evidence", "finding_traffic"} {
		if got := rawRowCount(snap.Tables[table]); got != 1 {
			t.Errorf("archive snapshot %s rows = %d, want 1", table, got)
		}
	}
}

// TestArchiveRestoreRebindsFindingDeliveryTables 验证归档→恢复后，三张表的行
// 重新挂到了（被重建的）探索节点上，而不是因外键缺失/冲突而恢复失败。
func TestArchiveRestoreRebindsFindingDeliveryTables(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()
	if err := d.EnsureLLMRecordsTable(); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}

	tk, err := d.CreateTask("交付域恢复测试", "目标", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(tk.ID)

	es := d.Exploration(tk.ExplorationID)
	nodeID, err := es.AddNode(KindFinding, map[string]any{"summary": "x", "severity": "high"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddFinding(tk.ID, nodeID, "SQL注入", "SQL 注入", "high", "summary", "poc", "worker", nil); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertFindingEnrichment(&FindingEnrichment{NodeID: nodeID, Name: "富化标题", Severity: "high"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateFindingEvidence(nodeID, "shot.png", "证据图注", 1); err != nil {
		t.Fatal(err)
	}
	if err := d.LinkFindingTraffic(nodeID, []string{"ex-1"}, "manual"); err != nil {
		t.Fatal(err)
	}

	// 走真实归档→恢复链路：归档会删掉活节点（级联清掉三张表），恢复再把它们建回来。
	// 归档要求任务处于暂停/终态。
	if err := d.SetPaused(tk.ID, true); err != nil {
		t.Fatal(err)
	}
	archive, err := d.QueueTaskArchive(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = d.Exec(`DELETE FROM task_archives WHERE id=$1`, archive.ID) }()
	claimed, err := d.ClaimTaskArchiveJob(t.Context())
	if err != nil || claimed == nil || claimed.ID != archive.ID || claimed.State != Archiving {
		t.Fatalf("archive claim = %+v, %v", claimed, err)
	}
	snap, err := d.SnapshotTaskArchive(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.CompleteTaskArchive(archive.ID, snap, "/tmp/test-delivery.tar.zst", "abc", 100, 50); err != nil {
		t.Fatal(err)
	}
	// 归档删了节点 → 三张表必须跟着清空（不留孤儿行）。
	var count int64
	if err := d.QueryRow(`SELECT count(*) FROM finding_enrichment WHERE node_id=$1`, nodeID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("enrichment rows after archive = %d, want 0 (cascade)", count)
	}
	if err := d.QueryRow(`SELECT count(*) FROM finding_evidence WHERE node_id=$1`, nodeID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("evidence rows after archive = %d, want 0 (cascade)", count)
	}
	if err := d.QueryRow(`SELECT count(*) FROM finding_traffic WHERE node_id=$1`, nodeID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("finding_traffic rows after archive = %d, want 0 (cascade)", count)
	}

	if _, err := d.QueueTaskArchiveRestore(archive.ID); err != nil {
		t.Fatal(err)
	}
	restoring, err := d.ClaimTaskArchiveJob(t.Context())
	if err != nil || restoring == nil || restoring.State != Restoring {
		t.Fatalf("restore claim = %+v, %v", restoring, err)
	}
	if _, err := d.RestoreTaskArchive(archive.ID, snap, int64((10 * time.Minute).Seconds())); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT count(*) FROM finding_enrichment WHERE node_id=$1`, nodeID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("enrichment rows after restore = %d, want 1", count)
	}
	if err := d.QueryRow(`SELECT count(*) FROM finding_evidence WHERE node_id=$1`, nodeID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("evidence rows after restore = %d, want 1", count)
	}
	if err := d.QueryRow(`SELECT count(*) FROM finding_traffic WHERE node_id=$1`, nodeID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("finding_traffic rows after restore = %d, want 1", count)
	}
}
