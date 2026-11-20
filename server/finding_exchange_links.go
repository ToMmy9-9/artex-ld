// finding_exchange_links.go：漏洞↔**原始流量包**（exchange）级别的关联接口。
//
// 与 server/finding_traffic.go 的关系（两者都叫「流量关联」但语义不同，不要混）：
//   - finding_traffic.go（artex-ld 自有）：**快照式**证据绑定。把某条流量复制成
//     不可变快照存进 traffic_evidence_snapshots，带 role/note 与乐观锁 version。
//     适合「报告里引用这条证据」，证据内容此后不随流量记录变化。
//   - 本文件（AntiFox 移植）：**引用式**关联。finding_traffic 表只存 exchange_id，
//     报文正文按需从流量库实时取。适合「跳回当时录制的那个原始包」。
//
// 两者共用 `/api/exploration/findings/{id}/traffic` 前缀会撞路由（Go 的 ServeMux
// 同模式重复注册会 panic），因此这里用独立的 `/traffic-exchanges` 命名空间，
// 与 finding_traffic.go 的 `/traffic` 并存共存。
//
// 关联落在 PG（finding_traffic 表），报文元数据从流量库（SQLite）按 id 解析。

package server

import (
	"net/http"

	"artex/traffic"
)

// POST /api/exploration/findings/{id}/traffic-exchanges — body {exchange_ids:[]} 关联流量到漏洞。
func (s *Server) linkFindingTrafficExchange(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	nodeID, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "无效的漏洞 id")
		return
	}
	var req struct {
		ExchangeIDs []string `json:"exchange_ids"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if err := pg.LinkFindingTraffic(nodeID, req.ExchangeIDs, "manual"); err != nil {
		writeErr(w, 500, "关联失败: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "linked": len(req.ExchangeIDs)})
}

// GET /api/exploration/findings/{id}/traffic-exchanges — 列出该漏洞关联的流量（带元数据；报文正文按需另取）。
func (s *Server) listFindingTrafficExchanges(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	nodeID, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "无效的漏洞 id")
		return
	}
	ids, err := pg.ListFindingTrafficIDs(nodeID)
	if err != nil {
		writeErr(w, 500, "获取失败")
		return
	}
	var metas []traffic.ExchangeMeta
	if tr := s.m.Traffic(); tr != nil && len(ids) > 0 {
		metas, _ = tr.MetaByIDs(ids)
	}
	writeJSON(w, 200, map[string]any{"exchanges": trafficDTOs(metas)})
}

// DELETE /api/exploration/findings/{id}/traffic-exchanges/{exchange_id} — 解除关联。
func (s *Server) unlinkFindingTrafficExchange(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	nodeID, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "无效的漏洞 id")
		return
	}
	if err := pg.UnlinkFindingTraffic(nodeID, r.PathValue("exchange_id")); err != nil {
		writeErr(w, 500, "解除失败: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
