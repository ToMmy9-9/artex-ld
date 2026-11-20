// toolenv_http.go：环境准备的 HTTP 入口与审计查询。
//
// 两个路由：
//
//	POST /api/tool-assets/prepare-env  人手动触发一次准备（也供前端使用）
//	GET  /api/tool-env-preps           查历史记录
//
// 手动入口的意义：agent 自己准备失败时（比如需要内网凭证、需要手动挑选
// 入口程序），人可以拿着同一个能力补一刀，而不是又回到"自己解压好再传"。
package server

import (
	"encoding/json"
	"io"
	"net/http"

	"artex/db"
)

// POST /api/tool-assets/prepare-env
//
// body: {tool_name, url, entry_bin?, claim?}
//
// 这是一个会写磁盘的接口，所以它落在 requireAuth 之后（同其余 /api 路由）。
// 审计里的 requested_by 记成 human，与 keeper agent 触发的记录区分开——
// 事后追责时"是模型自己下的"和"是人点的"是两类完全不同的事故。
func (s *Server) toolEnvPrepHandler(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	// 读一次原始 body：既要解出结构体，又要保留"claim 是否显式传了"这个
	// 信息（没传 = 默认 true，传了 false = 尊重它）。
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, 400, "读取请求失败")
		return
	}
	var req toolEnvPrepRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeErr(w, 400, "请求格式错误: "+err.Error())
		return
	}
	claim, ok := boolField(raw, "claim")
	if !ok {
		claim = true
	}
	req.Claim = claim

	res, perr := s.toolEnvPrep(r.Context(), req, "human")
	if perr != nil {
		writeErr(w, 400, perr.Error())
		return
	}
	writeJSON(w, 200, res)
}

// GET /api/tool-env-preps?tool=<name>&limit=<n>
func (s *Server) toolEnvPrepList(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parseID(v); err == nil && n > 0 {
			limit = int(n)
		}
	}
	var (
		preps []*db.ToolEnvPrep
		err   error
	)
	if tool := r.URL.Query().Get("tool"); tool != "" {
		preps, err = pg.ListToolEnvPrepsByName(tool, limit)
	} else {
		preps, err = pg.ListToolEnvPreps(limit)
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if preps == nil {
		preps = []*db.ToolEnvPrep{}
	}
	writeJSON(w, 200, map[string]any{"preps": preps, "dir": s.toolAssetDir()})
}
