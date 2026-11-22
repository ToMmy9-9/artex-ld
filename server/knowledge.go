package server

// Knowledge-base HTTP surface: two-layer docs (global + per-task) CRUD, upload,
// keyword/semantic search preview, retrieval logs, and the embedder config.
// The heavy lifting (chunk/embed/retrieve) lives in the knowledge package.

import (
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"artex/db"
	"artex/knowledge"
)

func (s *Server) kb() *knowledge.Service { return knowledge.NewService(s.m.pg) }

// maxKnowledgeUpload 是知识库上传请求体的硬上限。与下面 io.LimitReader(file, 32<<20)
// 的单文件上限保持一致：整个请求体（含 multipart 边界与其它表单字段）留 2MB 余量。
const maxKnowledgeUpload = 34 << 20

func (s *Server) indexKnowledgeAsync(id int64) {
	if err := s.kb().IndexDoc(s.ctx, id); err != nil {
		log.Printf("[knowledge] 索引文档 #%d 失败: %v", id, err)
	}
}

// GET /api/knowledge?scope=&task=
func (s *Server) listKnowledge(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	taskID, _ := strconv.ParseInt(r.URL.Query().Get("task"), 10, 64)
	docs, err := s.m.pg.ListKnowledgeDocs(scope, taskID)
	if err != nil {
		writeErr(w, 500, "读取失败: "+err.Error())
		return
	}
	writeJSON(w, 200, docs)
}

// GET /api/knowledge/{id}
func (s *Server) getKnowledge(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	doc, err := s.m.pg.GetKnowledgeDoc(id)
	if err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	if doc == nil {
		writeErr(w, 404, "文档不存在")
		return
	}
	writeJSON(w, 200, doc)
}

type knowledgeDocBody struct {
	Scope    string `json:"scope"`
	TaskID   int64  `json:"task_id"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

// POST /api/knowledge
func (s *Server) createKnowledge(w http.ResponseWriter, r *http.Request) {
	var b knowledgeDocBody
	if err := decode(r, &b); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if strings.TrimSpace(b.Title) == "" {
		writeErr(w, 400, "标题不能为空")
		return
	}
	doc := &db.KnowledgeDoc{Scope: b.Scope, Category: b.Category, Title: b.Title, Content: b.Content, Source: "manual"}
	if b.Scope == "task" && b.TaskID > 0 {
		doc.TaskID = &b.TaskID
	}
	id, err := s.m.pg.AddKnowledgeDoc(doc)
	if err != nil {
		writeErr(w, 500, "保存失败: "+err.Error())
		return
	}
	go s.indexKnowledgeAsync(id)
	out, _ := s.m.pg.GetKnowledgeDoc(id)
	writeJSON(w, 200, out)
}

// PUT /api/knowledge/{id}
func (s *Server) updateKnowledge(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var b knowledgeDocBody
	if err := decode(r, &b); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if err := s.m.pg.UpdateKnowledgeDoc(id, b.Category, b.Title, b.Content); err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	go s.indexKnowledgeAsync(id)
	out, _ := s.m.pg.GetKnowledgeDoc(id)
	writeJSON(w, 200, out)
}

// DELETE /api/knowledge/{id}
func (s *Server) deleteKnowledge(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.m.pg.DeleteKnowledgeDoc(id); err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/knowledge/upload — multipart: file + scope + task_id + category.
func (s *Server) uploadKnowledge(w http.ResponseWriter, r *http.Request) {
	// ParseMultipartForm 的 32MB 只是「内存阈值」，超过后 Go 会把溢出的部分
	// 落到临时文件，body 本身仍是无上限的。实测传 80MB 原始 body 时服务端直接
	// 断连（客户端 WinError 10054），既没有干净的错误响应，也白读了一堆磁盘。
	// 这里先给 body 一个硬上限，再做解析。
	r.Body = http.MaxBytesReader(w, r.Body, maxKnowledgeUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if maxBytesExceeded(err) {
			writeErr(w, 413, "上传内容超过 32MB 上限")
			return
		}
		writeErr(w, 400, "上传解析失败")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "缺少文件")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		writeErr(w, 400, "读取失败")
		return
	}
	text, err := knowledge.ExtractText(hdr.Filename, raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(text) == "" {
		writeErr(w, 400, "文档内容为空或无法抽取文本")
		return
	}
	scope := r.FormValue("scope")
	taskID, _ := strconv.ParseInt(r.FormValue("task_id"), 10, 64)
	// A task-scoped upload MUST name a task — otherwise it silently lands as an
	// orphan (scope=task, task_id=NULL) that reads back as global. Reject instead.
	if scope == "task" && taskID <= 0 {
		writeErr(w, 400, "任务知识需指定所属任务：请选择要归属的运行任务，或到该任务详情页的「知识」标签页上传")
		return
	}
	doc := &db.KnowledgeDoc{
		Scope:    scope,
		Category: r.FormValue("category"),
		Title:    strings.TrimSuffix(hdr.Filename, filepath.Ext(hdr.Filename)),
		Content:  text,
		Source:   "upload",
		FileName: hdr.Filename,
		Bytes:    int64(len(raw)),
	}
	if scope == "task" && taskID > 0 {
		doc.TaskID = &taskID
	}
	id, err := s.m.pg.AddKnowledgeDoc(doc)
	if err != nil {
		writeErr(w, 500, "保存失败: "+err.Error())
		return
	}
	go s.indexKnowledgeAsync(id)
	out, _ := s.m.pg.GetKnowledgeDoc(id)
	writeJSON(w, 200, out)
}

// GET /api/knowledge/categories?task=
func (s *Server) knowledgeCategories(w http.ResponseWriter, r *http.Request) {
	taskID, _ := strconv.ParseInt(r.URL.Query().Get("task"), 10, 64)
	cats, err := s.m.pg.ListKnowledgeCategories(taskID)
	if err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, cats)
}

// POST /api/knowledge/search {query, task_id, top_k} — preview retrieval from the UI.
func (s *Server) searchKnowledge(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Query  string `json:"query"`
		TaskID int64  `json:"task_id"`
		TopK   int    `json:"top_k"`
	}
	if err := decode(r, &b); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	hits, mode, err := s.kb().SearchAndLog(s.ctx, b.Query, b.TaskID, b.TopK, "手动检索")
	if err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"mode": mode, "hits": hits})
}

// GET /api/knowledge/logs
func (s *Server) knowledgeLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.m.pg.ListKnowledgeRetrievalLogs(200)
	if err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, logs)
}

// GET /api/knowledge/config
func (s *Server) getKnowledgeConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.m.pg.GetKnowledgeConfig()
	if err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	global, task, _ := s.m.pg.KnowledgeDocCount()
	writeJSON(w, 200, map[string]any{
		"enabled":          cfg.Enabled,
		"base_url":         cfg.BaseURL,
		"model":            cfg.Model,
		"dim":              cfg.Dim,
		"has_key":          cfg.HasKey,
		"vector_available": s.m.pg.KnowledgeVectorAvailable(),
		"semantic_ready":   s.kb().SemanticReady(),
		"docs_global":      global,
		"docs_task":        task,
	})
}

// PUT /api/knowledge/config {enabled, base_url, model, api_key, dim}
func (s *Server) saveKnowledgeConfig(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Enabled bool   `json:"enabled"`
		BaseURL string `json:"base_url"`
		Model   string `json:"model"`
		APIKey  string `json:"api_key"`
		Dim     int    `json:"dim"`
	}
	if err := decode(r, &b); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if err := s.m.pg.SaveKnowledgeConfig(b.Enabled, b.BaseURL, b.Model, b.APIKey, b.Dim); err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/knowledge/config/test — probe the configured embedder.
func (s *Server) testKnowledgeEmbedder(w http.ResponseWriter, r *http.Request) {
	dim, err := s.kb().TestEmbedder(s.ctx)
	if err != nil {
		writeErr(w, 400, "连通失败: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "dim": dim})
}

// POST /api/knowledge/reindex — re-embed every doc (after an embedder/model change).
func (s *Server) reindexKnowledge(w http.ResponseWriter, r *http.Request) {
	n, err := s.kb().Reindex(s.ctx)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "indexed": n})
}
