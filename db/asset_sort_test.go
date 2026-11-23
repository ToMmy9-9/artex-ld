package db

import (
	"strings"
	"testing"
)

// TestParseAssetSortWhitelist 锁定排序字段的白名单行为。排序值会直接进 SQL，
// 未知字段必须回落到默认值而不是被拼进查询。
func TestParseAssetSortWhitelist(t *testing.T) {
	cases := []struct {
		field     string
		direction string
		wantField string
		wantOrder string
	}{
		{"last_seen", "desc", "last_seen", "desc"},
		{"last_seen", "asc", "last_seen", "asc"},
		{"created_at", "asc", "created_at", "asc"},
		{"name", "asc", "name", "asc"},
		{"status_code", "desc", "status_code", "desc"},
		{"content_length", "asc", "content_length", "asc"},
		{"  name  ", "ASC", "name", "asc"},
		{"", "", "last_seen", "desc"},
		// 注入尝试一律回落默认值。
		{"id; DROP TABLE assets", "asc", "last_seen", "asc"},
		{"unknown_field", "desc", "last_seen", "desc"},
	}
	for _, tc := range cases {
		got := ParseAssetSort(tc.field, tc.direction)
		if got.Field != tc.wantField || got.Order != tc.wantOrder {
			t.Errorf("ParseAssetSort(%q,%q) = %+v，期望 field=%q order=%q",
				tc.field, tc.direction, got, tc.wantField, tc.wantOrder)
		}
	}
}

// TestAssetSortZeroValueKeepsDefaultOrder 确认零值 AssetSort 仍是历史的
// 「last_seen DESC」。用 bool 存方向时零值是 false(=ASC)，会让所有旧调用点
// 的排序悄悄翻转 —— 这个用例就是防那件事。
func TestAssetSortZeroValueKeepsDefaultOrder(t *testing.T) {
	zero := AssetSort{}
	if !strings.Contains(zero.orderClause(), "last_seen DESC") {
		t.Fatalf("零值 AssetSort 应为 last_seen DESC，实际 %q", zero.orderClause())
	}
}

// TestAssetSortOrdersRows 验证排序真的生效，且分页在不同排序下不重复不丢行。
func TestAssetSortOrdersRows(t *testing.T) {
	d, av2, _ := testSetup(t)
	t.Cleanup(func() { _ = d.Close() })

	// 造三条有明确域名顺序的资产。
	for _, domain := range []string{"zzz-sort.example", "aaa-sort.example", "mmm-sort.example"} {
		if _, err := av2.UpsertRootDomain(UpsertRootDomainReq{Domain: domain, TaskID: 0}); err != nil {
			t.Fatalf("upsert %s: %v", domain, err)
		}
	}

	rows, err := av2.QueryByType("root_domain", 100, 0, ParseAssetSort("name", "asc"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range rows {
		if r.Domain != "" && (r.Domain == "zzz-sort.example" || r.Domain == "aaa-sort.example" || r.Domain == "mmm-sort.example") {
			names = append(names, r.Domain)
		}
	}
	if len(names) != 3 {
		t.Fatalf("应返回 3 条测试资产，实际 %d 条", len(names))
	}
	if names[0] != "aaa-sort.example" || names[2] != "zzz-sort.example" {
		t.Fatalf("按名称升序结果错误：%v", names)
	}

	desc, err := av2.QueryByType("root_domain", 100, 0, ParseAssetSort("name", "desc"))
	if err != nil {
		t.Fatal(err)
	}
	var descNames []string
	for _, r := range desc {
		if r.Domain == "zzz-sort.example" || r.Domain == "aaa-sort.example" || r.Domain == "mmm-sort.example" {
			descNames = append(descNames, r.Domain)
		}
	}
	if len(descNames) != 3 || descNames[0] != "zzz-sort.example" {
		t.Fatalf("按名称降序结果错误：%v", descNames)
	}
}

// TestAssetSortPagingIsStable 验证翻页时不会出现重复或漏行：排序必须以 id
// 作次级键（很多行的 last_seen 完全相同，缺了它同一行会出现在两页）。
func TestAssetSortPagingIsStable(t *testing.T) {
	d, av2, _ := testSetup(t)
	t.Cleanup(func() { _ = d.Close() })

	for i := 0; i < 6; i++ {
		if _, err := av2.UpsertRootDomain(UpsertRootDomainReq{
			Domain: "stable-" + string(rune('a'+i)) + ".example",
			TaskID: 0,
		}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	sort := ParseAssetSort("last_seen", "desc")
	seen := map[int64]bool{}
	for page := 0; page < 6; page++ {
		rows, err := av2.QueryByType("root_domain", 1, page, sort)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			if seen[r.ID] {
				t.Fatalf("分页出现重复 id %d", r.ID)
			}
			seen[r.ID] = true
		}
	}
	// 6 条资产、每页 1 条，应当正好看到 6 个不同 id。
	if len(seen) != 6 {
		t.Fatalf("每页 1 条翻 6 次应看到 6 个不同 id，实际 %d", len(seen))
	}
}
