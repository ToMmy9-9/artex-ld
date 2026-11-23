package db

import (
	"testing"
)

// TestAssetIDSetMatchesListQuery 保证「全选全部筛选结果」取到的 id 集合与
// 资产列表的分页查询是同一批数据。两者口径一旦漂移，用户全选后批量操作
// 就会作用到列表里看不到的资产上 —— 所以这里逐条比对 id 集合。
func TestAssetIDSetMatchesListQuery(t *testing.T) {
	d, av2, _ := testSetup(t)
	t.Cleanup(func() { _ = d.Close() })

	// 用独立 IP 段造数据，避免与其它用例的夹具互相干扰。
	ips := []string{"198.51.100.11", "198.51.100.12", "198.51.100.13"}
	created := map[int64]bool{}
	for _, ip := range ips {
		id, err := av2.UpsertIP(UpsertIPReq{IP: ip, TaskID: 0})
		if err != nil {
			t.Fatalf("upsert ip %s: %v", ip, err)
		}
		created[id] = true
	}
	// 另一条路径（root_domain）必须出现在 type=ip 的结果之外。
	rdID, err := av2.UpsertRootDomain(UpsertRootDomainReq{Domain: "assetidset.example", TaskID: 0})
	if err != nil {
		t.Fatalf("upsert root domain: %v", err)
	}
	created[rdID] = true

	t.Run("type 过滤与列表一致", func(t *testing.T) {
		rows, err := av2.QueryByType("ip", 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		listIDs := map[int64]bool{}
		for _, r := range rows {
			listIDs[r.ID] = true
		}
		// 只关心本次造的 3 条 IP。
		want := 0
		for id := range created {
			if listIDs[id] {
				want++
			}
		}
		if want != len(ips) {
			t.Fatalf("列表未返回全部 %d 条 IP（实际 %d）", len(ips), want)
		}

		ids, _, err := av2.AssetIDSet("", "ip", 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		got := 0
		for _, id := range ids {
			if created[id] {
				got++
			}
		}
		if got != len(ips) {
			t.Fatalf("AssetIDSet 未返回全部 %d 条 IP（实际 %d）", len(ips), got)
		}
		for _, id := range ids {
			if id == rdID {
				t.Fatal("AssetIDSet(type=ip) 混入了 root_domain 资产")
			}
		}
	})

	t.Run("id 集合是列表的超集且无越界", func(t *testing.T) {
		ids, labels, err := av2.AssetIDSet("", "ip", 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			if seen[id] {
				t.Fatalf("AssetIDSet 返回了重复 id %d", id)
			}
			seen[id] = true
			var typ string
			if err := d.QueryRow(`SELECT type FROM assets WHERE id=$1`, id).Scan(&typ); err != nil {
				t.Fatalf("id %d 不在 assets 表里: %v", id, err)
			}
			if typ != "ip" {
				t.Fatalf("id %d 的 type 是 %q，与筛选条件 ip 不符", id, typ)
			}
		}
		// 展示名必须与 assets 表里的实际值一致，删除确认弹窗要靠它列清单。
		for id, label := range labels {
			var want string
			if err := d.QueryRow(
				`SELECT COALESCE(NULLIF(domain,''), NULLIF(ip,''), NULLIF(url,''), '') FROM assets WHERE id=$1`,
				id,
			).Scan(&want); err != nil {
				t.Fatalf("读资产 %d 失败: %v", id, err)
			}
			if label != want {
				t.Fatalf("资产 %d 的展示名是 %q，期望 %q", id, label, want)
			}
		}
	})

	t.Run("DSL 路径与列表一致", func(t *testing.T) {
		// 资产 DSL 只支持 "="（ILIKE 包含匹配）；"==" 会被解析成裸文本并报语法错。
		dsl := `ip = 198.51.100.11`
		rows, err := av2.QueryDSL(dsl, "", 0, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		ids, _, err := av2.AssetIDSet(dsl, "", 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) != len(rows) {
			t.Fatalf("DSL 路径口径不一致：列表 %d 条，AssetIDSet %d 条", len(rows), len(ids))
		}
		listSet := map[int64]bool{}
		for _, r := range rows {
			listSet[r.ID] = true
		}
		for _, id := range ids {
			if !listSet[id] {
				t.Fatalf("AssetIDSet 返回了列表里没有的 id %d", id)
			}
		}
	})
}
