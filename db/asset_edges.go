// asset_edges.go：资产图的父子边（root_domain → subdomain → IP → service → endpoint）。
//
// 为什么单独建表：assets 只存单条资产，跨资产的“谁属于谁 / 谁解析到谁”此前只能在
// 应用层用 bound_domains / record_value / url 前缀临时拼，无法做子树遍历或路径查询。
// 这里把关系落成显式边，写入由各 Upsert* 统一维护（失败只忽略，不阻断资产入库）。
package db

import (
	"fmt"
	"strings"
)

// 资产边关系。方向固定为“容器/上游 → 被包含/下游”：
//
//	root_domain --contains--> subdomain
//	subdomain   --resolves_to--> ip
//	ip          --hosts--> service
//	domain      --serves--> service（无 IP 时退化为域名直连服务）
//	service     --exposes--> endpoint
const (
	AssetRelContains   = "contains"
	AssetRelResolvesTo = "resolves_to"
	AssetRelHosts      = "hosts"
	AssetRelServes     = "serves"
	AssetRelExposes    = "exposes"
)

// AssetRelation is one row of a subtree/path query, joined with the asset row's
// display label so callers need no second query.
type AssetRelation struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Label    string `json:"label"`
	Rel      string `json:"rel"`
	Depth    int    `json:"depth"`
	ParentID int64  `json:"parent_id"`
}

// addAssetEdge inserts one edge. Best-effort by design: an edge failure must never
// fail the asset upsert that triggered it.
func (s *AssetStore) addAssetEdge(parentID, childID int64, rel string) {
	if s == nil || s.tx == nil || parentID <= 0 || childID <= 0 || parentID == childID {
		return
	}
	_, _ = s.tx.Exec(`
INSERT INTO asset_edges(parent_id, child_id, rel)
VALUES ($1, $2, $3)
ON CONFLICT (parent_id, child_id, rel) DO NOTHING`, parentID, childID, rel)
}

// findAssetIDByDomain looks up the most specific domain asset (subdomain before
// root_domain) inside the current transaction.
func (s *AssetStore) findAssetIDByDomain(domain string) (int64, bool) {
	domain = DomainKey(domain)
	if s == nil || s.tx == nil || domain == "" {
		return 0, false
	}
	// 同一个域名可能有多行（subdomain 的唯一键是 domain+record_type，所以
	// "api.x.com/A" 与 linkHostAssets 补的 "api.x.com/''" 会并存）。图里必须挑
	// 一个规范节点：优先带 record_type 的子域（信息更全），其次无 record_type
	// 的子域，最后才是 root_domain。
	var id int64
	err := s.tx.QueryRow(`
SELECT id FROM assets
WHERE type IN ('subdomain','root_domain') AND domain = $1
ORDER BY
    CASE type WHEN 'subdomain' THEN 0 ELSE 1 END,
    CASE WHEN COALESCE(record_type,'') <> '' THEN 0 ELSE 1 END,
    id ASC
LIMIT 1`, domain).Scan(&id)
	return id, err == nil
}

func (s *AssetStore) findAssetIDByIP(ip string) (int64, bool) {
	ip = strings.TrimSpace(ip)
	if s == nil || s.tx == nil || ip == "" {
		return 0, false
	}
	var id int64
	err := s.tx.QueryRow(`SELECT id FROM assets WHERE type = 'ip' AND ip = $1 ORDER BY id DESC LIMIT 1`, ip).Scan(&id)
	return id, err == nil
}

// findHTTPServiceIDByOrigin finds the HTTP service whose URL is exactly the
// origin (optionally with a trailing slash or a path). Used to attach endpoints.
func (s *AssetStore) findHTTPServiceIDByOrigin(origin string) (int64, bool) {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if s == nil || s.tx == nil || origin == "" {
		return 0, false
	}
	var id int64
	err := s.tx.QueryRow(`
SELECT id FROM assets
WHERE type = 'service' AND service_type = 'http'
  AND (url = $1 OR url = $1 || '/' OR url LIKE $1 || '/%')
ORDER BY length(url) ASC, id DESC
LIMIT 1`, origin).Scan(&id)
	return id, err == nil
}

// linkServiceParent attaches a service to its host: IP first, then the most
// specific domain asset. Keeps the graph a tree (one parent per service).
func (s *AssetStore) linkServiceParent(serviceID int64, ip, domain, rootDomain string) {
	if serviceID <= 0 {
		return
	}
	if ip != "" {
		if pid, ok := s.findAssetIDByIP(ip); ok {
			s.addAssetEdge(pid, serviceID, AssetRelHosts)
			return
		}
	}
	if domain != "" {
		if pid, ok := s.findAssetIDByDomain(domain); ok {
			s.addAssetEdge(pid, serviceID, AssetRelServes)
			return
		}
	}
	if rootDomain != "" {
		if pid, ok := s.findAssetIDByDomain(rootDomain); ok {
			s.addAssetEdge(pid, serviceID, AssetRelServes)
		}
	}
}

// AssetNeighbors returns the descendants of rootID (depth-first tree) and its
// direct parents. Depth is capped at 10 to bound recursive queries.
func (s *AssetStore) AssetNeighbors(rootID int64, maxDepth int) ([]AssetRelation, error) {
	if rootID <= 0 {
		return nil, fmt.Errorf("asset id is required")
	}
	if maxDepth <= 0 {
		maxDepth = 1
	}
	if maxDepth > 10 {
		maxDepth = 10
	}
	rows, err := s.db.Query(`
WITH RECURSIVE tree AS (
    SELECT e.child_id AS id, e.rel, e.parent_id, 1 AS depth,
           ARRAY[e.parent_id, e.child_id] AS path
    FROM asset_edges e
    WHERE e.parent_id = $1
    UNION ALL
    SELECT e.child_id, e.rel, e.parent_id, t.depth + 1,
           t.path || e.child_id
    FROM asset_edges e
    JOIN tree t ON e.parent_id = t.id
    WHERE t.depth < $2 AND NOT (e.child_id = ANY(t.path))
)
SELECT t.id, a.type,
       COALESCE(NULLIF(a.domain,''), NULLIF(a.url,''), NULLIF(a.ip,''),
                NULLIF(a.app_name,''), NULLIF(a.service_name,''), ''),
       t.rel, t.depth, t.parent_id
FROM tree t
JOIN assets a ON a.id = t.id
ORDER BY t.depth, t.id`, rootID, maxDepth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AssetRelation, 0, 16)
	seen := map[int64]bool{}
	for rows.Next() {
		var r AssetRelation
		if err := rows.Scan(&r.ID, &r.Type, &r.Label, &r.Rel, &r.Depth, &r.ParentID); err != nil {
			return nil, err
		}
		out = append(out, r)
		seen[r.ID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Direct parents are useful context (root_id 是谁发现的 / 属于哪个域) and
	// cheap: one more join, no recursion.
	prows, err := s.db.Query(`
SELECT e.parent_id, a.type,
       COALESCE(NULLIF(a.domain,''), NULLIF(a.url,''), NULLIF(a.ip,''),
                NULLIF(a.app_name,''), NULLIF(a.service_name,''), ''),
       e.rel, e.child_id
FROM asset_edges e
JOIN assets a ON a.id = e.parent_id
WHERE e.child_id = $1
ORDER BY a.id`, rootID)
	if err != nil {
		return nil, err
	}
	defer prows.Close()
	for prows.Next() {
		var r AssetRelation
		if err := prows.Scan(&r.ID, &r.Type, &r.Label, &r.Rel, &r.ParentID); err != nil {
			return nil, err
		}
		if seen[r.ID] {
			continue
		}
		r.Depth = 0
		out = append(out, r)
	}
	return out, prows.Err()
}

// AssetPath returns one shortest parent→child path from fromID to toID (as asset
// ids), or nil when no path exists within maxDepth.
func (s *AssetStore) AssetPath(fromID, toID int64, maxDepth int) ([]int64, error) {
	if fromID <= 0 || toID <= 0 {
		return nil, fmt.Errorf("from and to asset ids are required")
	}
	if maxDepth <= 0 {
		maxDepth = 6
	}
	if maxDepth > 10 {
		maxDepth = 10
	}
	var joined string
	err := s.db.QueryRow(`
WITH RECURSIVE p AS (
    SELECT e.parent_id, e.child_id,
           ARRAY[e.parent_id, e.child_id] AS path, 1 AS depth
    FROM asset_edges e
    WHERE e.parent_id = $1
    UNION ALL
    SELECT e.parent_id, e.child_id, p.path || e.child_id, p.depth + 1
    FROM asset_edges e
    JOIN p ON e.parent_id = p.child_id
    WHERE p.depth < $3 AND NOT (e.child_id = ANY(p.path))
)
SELECT array_to_string(path, ',')
FROM p
WHERE child_id = $2
ORDER BY depth
LIMIT 1`, fromID, toID, maxDepth).Scan(&joined)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, err
	}
	parts := strings.Split(joined, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		var id int64
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}
