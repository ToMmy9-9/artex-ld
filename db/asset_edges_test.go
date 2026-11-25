package db

import (
	"testing"
)

// cleanupAssetGraph removes every asset created by these tests plus its edges.
// asset_edges has ON DELETE CASCADE, so deleting the assets is enough.
func cleanupAssetGraph(t *testing.T, d *DB, ids ...int64) {
	t.Helper()
	for _, id := range ids {
		if id > 0 {
			d.Exec(`DELETE FROM assets WHERE id = $1`, id)
		}
	}
}

// TestAssetGraphLinksHostChain 是资产图的核心用例：写一次完整链条
// （root_domain → subdomain → IP → service → endpoint），断言每一层边都落库，
// 并且 AssetNeighbors 能从根域一路展开到 endpoint。
func TestAssetGraphLinksHostChain(t *testing.T) {
	d, as, _ := testSetup(t)

	const root = "graph-chain-test.example"
	const sub = "api.graph-chain-test.example"
	const ip = "203.0.113.77"

	rootID, err := as.UpsertRootDomain(UpsertRootDomainReq{Domain: root})
	if err != nil {
		t.Fatalf("UpsertRootDomain: %v", err)
	}
	subID, err := as.UpsertSubdomain(UpsertSubdomainReq{
		Domain: sub, RecordType: "A", RecordValue: []string{ip},
	})
	if err != nil {
		t.Fatalf("UpsertSubdomain: %v", err)
	}
	ipID, err := as.UpsertIP(UpsertIPReq{IP: ip, BoundDomains: []string{sub}})
	if err != nil {
		t.Fatalf("UpsertIP: %v", err)
	}
	svcID, err := as.UpsertHTTPService(UpsertHTTPServiceReq{URL: "https://" + sub + "/", IP: ip})
	if err != nil {
		t.Fatalf("UpsertHTTPService: %v", err)
	}
	epID, err := as.UpsertEndpoint(UpsertEndpointReq{URL: "https://" + sub + "/api/orders/1", Method: "GET"})
	if err != nil {
		t.Fatalf("UpsertEndpoint: %v", err)
	}
	t.Cleanup(func() {
		cleanupAssetGraph(t, d, epID, svcID, ipID, subID, rootID)
	})

	edges := map[string]bool{}
	rows, err := d.Query(`SELECT parent_id, child_id, rel FROM asset_edges
		WHERE parent_id = ANY($1) OR child_id = ANY($1)`,
		[]int64{rootID, subID, ipID, svcID, epID})
	if err != nil {
		t.Fatalf("query edges: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p, c int64
		var rel string
		if err := rows.Scan(&p, &c, &rel); err != nil {
			t.Fatalf("scan: %v", err)
		}
		edges[rel] = true
	}

	for _, want := range []string{AssetRelContains, AssetRelResolvesTo, AssetRelHosts, AssetRelExposes} {
		if !edges[want] {
			t.Errorf("missing edge rel=%q (have %v)", want, edges)
		}
	}

	// 从根域递归展开：必须能看到 subdomain / ip / service / endpoint。
	rels, err := as.AssetNeighbors(rootID, 5)
	if err != nil {
		t.Fatalf("AssetNeighbors: %v", err)
	}
	seen := map[int64]string{}
	for _, r := range rels {
		seen[r.ID] = r.Type
	}
	for id, label := range map[int64]string{
		subID: "subdomain", ipID: "ip", svcID: "service", epID: "endpoint",
	} {
		if _, ok := seen[id]; !ok {
			t.Errorf("subtree of root domain is missing %s (id=%d); got %v", label, id, seen)
		}
	}

	// 从 endpoint 往上看，应能看到它的直属上级（service）。
	up, err := as.AssetNeighbors(epID, 1)
	if err != nil {
		t.Fatalf("AssetNeighbors(endpoint): %v", err)
	}
	hasParent := false
	for _, r := range up {
		if r.Depth == 0 && r.ID == svcID {
			hasParent = true
		}
	}
	if !hasParent {
		t.Errorf("endpoint %d has no parent service %d; got %+v", epID, svcID, up)
	}

	// 路径查询：root → endpoint 必须连通，且首尾正确。
	path, err := as.AssetPath(rootID, epID, 6)
	if err != nil {
		t.Fatalf("AssetPath: %v", err)
	}
	if len(path) < 3 {
		t.Fatalf("path root→endpoint = %v, want at least root/subdomain/…/endpoint", path)
	}
	if path[0] != rootID || path[len(path)-1] != epID {
		t.Fatalf("path = %v, want first=%d last=%d", path, rootID, epID)
	}
}

// TestAssetGraphIdempotent 重复写入同一条链不应产生重复边（PRIMARY KEY +
// ON CONFLICT DO NOTHING）。
func TestAssetGraphIdempotent(t *testing.T) {
	d, as, _ := testSetup(t)

	const root = "graph-idem-test.example"
	const sub = "www.graph-idem-test.example"

	rootID, err := as.UpsertRootDomain(UpsertRootDomainReq{Domain: root})
	if err != nil {
		t.Fatalf("UpsertRootDomain: %v", err)
	}
	subID, err := as.UpsertSubdomain(UpsertSubdomainReq{Domain: sub})
	if err != nil {
		t.Fatalf("UpsertSubdomain: %v", err)
	}
	t.Cleanup(func() { cleanupAssetGraph(t, d, subID, rootID) })

	for i := 0; i < 3; i++ {
		if _, err := as.UpsertSubdomain(UpsertSubdomainReq{Domain: sub}); err != nil {
			t.Fatalf("UpsertSubdomain round %d: %v", i, err)
		}
	}
	var n int
	if err := d.QueryRow(`SELECT count(*) FROM asset_edges WHERE parent_id = $1 AND child_id = $2 AND rel = $3`,
		rootID, subID, AssetRelContains).Scan(&n); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if n != 1 {
		t.Fatalf("duplicate contains edges: %d, want 1", n)
	}
}

// TestAssetNeighborsRejectsBadInput 非法入参必须报错而不是静默返回空。
func TestAssetNeighborsRejectsBadInput(t *testing.T) {
	_, as, _ := testSetup(t)
	if _, err := as.AssetNeighbors(0, 2); err == nil {
		t.Fatal("AssetNeighbors(0) must fail")
	}
	if _, err := as.AssetPath(0, 1, 3); err == nil {
		t.Fatal("AssetPath with zero id must fail")
	}
}

// TestAssetPathMissingReturnsNil 没有路径时返回 (nil, nil) —— 调用方据空结果
// 判定「不连通」，不是错误。
func TestAssetPathMissingReturnsNil(t *testing.T) {
	_, as, _ := testSetup(t)
	path, err := as.AssetPath(999999991, 999999992, 4)
	if err != nil {
		t.Fatalf("AssetPath: %v", err)
	}
	if path != nil {
		t.Fatalf("path = %v, want nil", path)
	}
}

// TestAssetGraphSingleNodePerDomain 同一域名只能有一个图谱节点。
//
// 背景（运行时实测发现的缺陷）：subdomain 的唯一键是 (domain, record_type)，
// 而 linkHostAssets 在服务/端点落库时会补一条 record_type=” 的子域行，
// 于是 "api.x.com/A"（insert_assets 写入）与 "api.x.com/”"（host 链接补的）
// 并存，图谱里同一个域名出现两个节点、边挂在随机一条上。
func TestAssetGraphSingleNodePerDomain(t *testing.T) {
	d, as, _ := testSetup(t)

	const root = "graph-dup-test.example"
	const sub = "api.graph-dup-test.example"
	const ip = "198.51.100.91"

	rootID, err := as.UpsertRootDomain(UpsertRootDomainReq{Domain: root})
	if err != nil {
		t.Fatalf("UpsertRootDomain: %v", err)
	}
	subID, err := as.UpsertSubdomain(UpsertSubdomainReq{
		Domain: sub, RecordType: "A", RecordValue: []string{ip},
	})
	if err != nil {
		t.Fatalf("UpsertSubdomain: %v", err)
	}
	// 这一步会经过 linkHostAssets，旧实现会在同一域名下再插一行 record_type=''。
	svcID, err := as.UpsertHTTPService(UpsertHTTPServiceReq{URL: "https://" + sub + "/", IP: ip})
	if err != nil {
		t.Fatalf("UpsertHTTPService: %v", err)
	}
	t.Cleanup(func() {
		// ip 是字符串；按 IP 删掉它的资产行（UpsertIP 可能创建了多行同 IP 之外的行）。
		d.Exec(`DELETE FROM assets WHERE type='ip' AND ip = $1`, ip)
		cleanupAssetGraph(t, d, svcID, subID, rootID)
	})

	var rows int
	if err := d.QueryRow(`SELECT count(*) FROM assets WHERE type='subdomain' AND domain = $1`, sub).Scan(&rows); err != nil {
		t.Fatalf("count subdomains: %v", err)
	}
	if rows != 1 {
		t.Fatalf("subdomain rows for %s = %d, want 1 (duplicate '' row would split the graph)", sub, rows)
	}

	// 服务应挂在 IP 下（ip --hosts--> service），并因此能从唯一子域节点沿路径到达。
	// 这里验的是「图谱仍然是连通的、没有因为重复行而断开」，而不是服务的直接父节点是谁。
	path, err := as.AssetPath(subID, svcID, 6)
	if err != nil {
		t.Fatalf("AssetPath: %v", err)
	}
	if len(path) < 2 {
		rels, _ := as.AssetNeighbors(subID, 3)
		t.Fatalf("service %d is unreachable from subdomain %d; path=%v neighbors=%+v", svcID, subID, path, rels)
	}
	if path[0] != subID || path[len(path)-1] != svcID {
		t.Fatalf("path = %v, want first=%d last=%d", path, subID, svcID)
	}
}
