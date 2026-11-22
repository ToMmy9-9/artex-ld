// toolassets.go：外部工具资产的上传、管理与受管认领。
//
// 分工（这是本设计的核心）：
//   平台做确定性的事：算哈希、判平台、落盘、定义权限上限、强制执行契约。
//   agent 做语义性的事：跑 --help 解析参数、判断工具类型、写调用示例、
//                        标注风险点、产出契约。
//
// agent 的产出永远只能是"更紧"的契约（见 db.ValidateContractAgainstLimits），
// 放宽权限的权利留在平台代码里。这样即便 agent 被提示注入或模型幻觉，
// 它也无法把一个高危工具的权限上限抬高。
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"artex/db"
)

// maxToolUpload caps one uploaded binary (memory + spill).
const maxToolUpload = 512 << 20 // 512 MiB

// toolAssetDir is the managed directory holding uploaded binaries.
//
// It sits NEXT TO data/ (not inside it), so backing up or migrating data/
// never drags hundreds of megabytes of third-party binaries along, and a
// wiped data/ does not take the tool library with it.
func (s *Server) toolAssetDir() string {
	// Prefer the install root (baseDir). dataDir's parent is only a fallback:
	// under `go run` dataDir is the relative path "data", and taking its
	// parent would put the tool library INSIDE data/ — exactly what we want to
	// avoid (a data backup would then carry every uploaded binary).
	base := strings.TrimSpace(s.baseDir)
	if base == "" {
		abs, err := filepath.Abs(s.dataDir)
		if err != nil {
			base = s.dataDir
		} else {
			base = filepath.Dir(abs)
		}
	}
	abs, err := filepath.Abs(filepath.Join(base, "tools"))
	if err != nil {
		return filepath.Join(base, "tools")
	}
	return abs
}

// toolNameRe constrains uploaded tool names to a safe charset: they become
// directory names and appear in argv, so path separators and shell metacharacters
// are rejected outright.
var toolNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// ─────────────────────────── 上传 ───────────────────────────

// POST /api/tool-assets/upload — multipart form field "file" (one or more).
//
// The binary lands in the managed directory and a tool_assets row is created
// in state=uploaded. The platform then wakes the internal tool_keeper agent
// to claim it (see toolassets_triggerClaim).
func (s *Server) toolAssetUpload(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxToolUpload)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, 400, "上传请求格式错误: "+err.Error())
		return
	}
	dir := s.toolAssetDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeErr(w, 500, "创建工具目录失败: "+err.Error())
		return
	}

	var created []map[string]any
	var rejected []map[string]any
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, 400, "读取上传内容失败: "+err.Error())
			return
		}
		name := part.FileName()
		if name == "" {
			_ = part.Close()
			continue
		}
		// basename only: the client-supplied path must not influence the
		// destination directory.
		base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
		base = strings.TrimSpace(base)
		if !toolNameRe.MatchString(base) {
			_ = part.Close()
			rejected = append(rejected, map[string]any{
				"name": base, "reason": "文件名只允许字母数字与 . _ -，且必须以字母数字开头",
			})
			continue
		}
		toolName := strings.TrimSuffix(base, filepath.Ext(base))
		if toolName == "" {
			_ = part.Close()
			rejected = append(rejected, map[string]any{"name": base, "reason": "无法从文件名解析出工具名"})
			continue
		}

		dst := filepath.Join(dir, base)
		// 同名工具的重新上传 = 升级。旧记录（及其审计历史）保留，但工具本身
		// 被新版取代：先删旧二进制，DB 行稍后按 (name, platform) 覆盖。
		if _, statErr := os.Stat(dst); statErr == nil {
			_ = os.Remove(dst)
		}
		f, openErr := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
		if openErr != nil {
			_ = part.Close()
			rejected = append(rejected, map[string]any{"name": base, "reason": "写入失败: " + openErr.Error()})
			continue
		}
		h := sha256.New()
		size, copyErr := io.Copy(io.MultiWriter(f, h), part)
		closeErr := f.Close()
		_ = part.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(dst)
			writeErr(w, 500, "保存文件失败")
			return
		}

		sum := hex.EncodeToString(h.Sum(nil))
		platform := detectToolPlatform(dst)
		asset := &db.ToolAsset{
			Name:        toolName,
			DisplayName: base,
			SHA256:      sum,
			SizeBytes:   size,
			Platform:    platform,
			RelPath:     base,
			State:       "uploaded",
			// 平台先给一份默认上限（按 utility），让上传即可用；
			// keeper 认领时会按判定出的类别给出更贴合的收紧版契约。
			Limits: db.DefaultContractJSON("utility"),
		}
		// (name, platform) 唯一：已存在 = 这个工具之前传过，本次是升级。
		// 覆盖时重置认领结论（工具可能换了版本，旧的契约不再适用），
		// 但保留审计历史。
		replaced := false
		if prev, perr := pg.ToolAssetByName(toolName, platform); perr == nil && prev != nil {
			replaced = true
			asset.ID = prev.ID
			if err := pg.ReplaceToolAssetBinary(prev.ID, asset); err != nil {
				_ = os.Remove(dst)
				rejected = append(rejected, map[string]any{"name": base, "reason": "覆盖登记失败: " + err.Error()})
				continue
			}
		} else if _, err := pg.InsertToolAsset(asset); err != nil {
			_ = os.Remove(dst)
			rejected = append(rejected, map[string]any{"name": base, "reason": "登记失败: " + err.Error()})
			continue
		}
		if len(asset.Limits) > 0 {
			if err := pg.SetToolAssetLimits(asset.ID, asset.Limits); err != nil {
				log.Printf("[tool-assets] 写入默认权限上限失败 id=%d: %v", asset.ID, err)
			}
		}
		created = append(created, map[string]any{
			"id": asset.ID, "name": asset.Name, "file": base, "size": size,
			"sha256": sum, "platform": platform, "state": asset.State,
			"replaced": replaced,
		})
	}
	if len(created) == 0 && len(rejected) == 0 {
		writeErr(w, 400, "没有收到任何文件")
		return
	}
	// Mirror the new assets into the tool catalogue right away so an agent can
	// see them even before the keeper has claimed them (the catalogue entry is
	// refreshed again once the claim completes).
	s.syncToolAssetsToCatalog()
	// Wake the internal keeper so the user does not have to configure anything.
	s.toolassets_triggerClaim(created)
	writeJSON(w, 200, map[string]any{"created": created, "rejected": rejected})
}


// toolAssetPath resolves an asset's on-disk path, refusing anything that
// escapes the managed directory.
//
// rel_path 有两种形态：
//   - 单文件上传：就是受管目录下的文件名（"gogo.exe"）
//   - 环境准备的发布包：子目录 + 入口程序（"gogo/gogo.exe"），包里的
//     DLL/字典/模板都在同一子目录下，工具必须在那个目录里才能跑
//
// 所以不能再用 filepath.Base 拍平——那会让所有子目录形态的工具永远
// "文件不存在"。改成：先拒绝任何 ".." 段与绝对路径，再拼前缀校验，
// 两道一起保证出不去。
func (s *Server) toolAssetPath(asset *db.ToolAsset) (string, error) {
	root := filepath.Clean(s.toolAssetDir())
	rel := strings.TrimSpace(asset.RelPath)
	if rel == "" {
		return "", fmt.Errorf("工具路径为空")
	}
	// 统一分隔符：rel_path 在库里存的是 slash 形态。
	rel = strings.ReplaceAll(rel, "\\", "/")
	if strings.HasPrefix(rel, "/") || (len(rel) >= 2 && rel[1] == ':') {
		return "", fmt.Errorf("工具路径非法（绝对路径）")
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return "", fmt.Errorf("工具路径非法（含 ..）")
		}
	}
	full := filepath.Join(root, filepath.FromSlash(path.Clean(rel)))
	// filepath.Join already cleans the result; compare against the cleaned root.
	if !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("工具路径非法")
	}
	if _, err := os.Stat(full); err != nil {
		return "", fmt.Errorf("工具文件不存在，可能已被删除")
	}
	return full, nil
}

// detectToolPlatform sniffs the executable header to decide which platform the
// binary targets. A wrong platform label would make the tool show up on the
// wrong deployment's inventory, so we look at the actual bytes.
func detectToolPlatform(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "unknown"
	}
	defer f.Close()
	head := make([]byte, 4)
	n, _ := io.ReadFull(f, head)
	if n < 2 {
		return "unknown"
	}
	switch {
	case head[0] == 'M' && head[1] == 'Z':
		return "windows"
	case head[0] == 0x7f && head[1] == 'E' && head[2] == 'L':
		return "linux"
	case head[0] == 0xCF && head[1] == 0xFA && head[2] == 0xED:
		return "darwin"
	}
	// Shell/python scripts are platform-neutral enough to run anywhere with a
	// shebang, but a .py/.sh with no shebang still needs an interpreter.
	return "script"
}

// ─────────────────────────── 列表 / 删除 ───────────────────────────

// GET /api/tool-assets — list uploaded tool assets.
func (s *Server) toolAssetList(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	assets, err := pg.ListToolAssets()
	if err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	if assets == nil {
		assets = []*db.ToolAsset{}
	}
	// 每个资产带上"实际生效的范围执行模式"和"是否建议降级"，让 UI 能
	// 明确告诉用户：这个工具的范围管控是强制的、提示式的，还是不适用。
	// 缺了这些信息，一个 advisory 的工具看起来和 enforced 一样受管控。
	type item struct {
		*db.ToolAsset
		ScopeEnforcement string `json:"scope_enforcement"`
		ScopeSuggest     string `json:"scope_suggestion"`
		NativeSocket     bool   `json:"native_socket"`
	}
	out := make([]item, 0, len(assets))
	for _, a := range assets {
		mode := effectiveScopeEnforcement(a)
		suggest, native := db.SuggestScopeEnforcement(a.Category)
		out = append(out, item{ToolAsset: a, ScopeEnforcement: mode, ScopeSuggest: suggest, NativeSocket: native})
	}
	writeJSON(w, 200, map[string]any{"assets": out, "dir": s.toolAssetDir()})
}

// DELETE /api/tool-assets/{id} — remove an asset and its binary.
func (s *Server) toolAssetDelete(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	asset, err := pg.ToolAssetByID(id)
	if err != nil {
		writeErr(w, 404, "工具不存在")
		return
	}
	// Only remove files that live inside the managed directory; a corrupted
	// rel_path must never turn into a path traversal delete.
	//
	// 发布包形态的工具（环境准备产生）在受管目录下有自己的子目录，
	// 删资产时要把整个子目录删掉——只删入口 exe 会留下一堆孤儿依赖文件，
	// 而它们还占着几百 MB。判断依据：rel_path 含 '/' 时，第一段就是
	// 该工具独占的目录。
	root := filepath.Clean(s.toolAssetDir())
	rel := filepath.ToSlash(strings.ReplaceAll(strings.TrimSpace(asset.RelPath), "\\", "/"))
	var dst string
	if first, _, ok := strings.Cut(rel, "/"); ok && first != "" && first != ".." {
		dst = filepath.Join(root, filepath.FromSlash(first))
	} else {
		dst = filepath.Join(root, filepath.Base(rel))
	}
	if !strings.HasPrefix(filepath.Clean(dst), root+string(os.PathSeparator)) {
		writeErr(w, 500, "工具路径非法")
		return
	}
	if strings.Contains(rel, "/") {
		_ = os.RemoveAll(dst)
	} else {
		_ = os.Remove(dst)
	}
	if err := pg.DeleteToolAsset(id); err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"deleted": id})
}

// POST /api/tool-assets/{id}/state — change lifecycle state
// (managed / rejected / quarantined / uploaded).
func (s *Server) toolAssetSetState(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	var req struct {
		State string `json:"state"`
		Note  string `json:"note"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	switch req.State {
	case "uploaded", "managed", "rejected", "quarantined":
	default:
		writeErr(w, 400, "state 只能是 uploaded/managed/rejected/quarantined")
		return
	}
	if err := pg.UpdateToolAssetState(id, req.State); err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "state": req.State})
}

// GET /api/tool-assets/{id}/runs — audit trail of executions.
func (s *Server) toolAssetRuns(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	runs, err := pg.ListToolAssetRuns(id, 100)
	if err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	if runs == nil {
		runs = []*db.ToolAssetRun{}
	}
	writeJSON(w, 200, map[string]any{"runs": runs})
}

// ─────────────────────────── 受限探测 ───────────────────────────

// probeAllowArgs is the whitelist of argument shapes the keeper agent may use
// while inspecting an uploaded binary.
//
// The keeper runs a third-party binary to learn its interface. That binary is
// untrusted by definition — it may be a trojan wearing gogo's name. So the
// platform runs the probe itself with a fixed argv grammar and a short
// timeout, instead of letting the agent compose an arbitrary command line.
// Only the classic "print usage and exit" flags are allowed, and never
// together with anything that looks like a target.
var probeAllowArgs = map[string]bool{
	"--help": true, "-h": true, "-help": true, "help": true,
	"--version": true, "-version": true, "-V": true, "version": true,
}

// probeMaxOutput caps how much probe output we keep per tool.
const probeMaxOutput = 16 << 10 // 16 KiB

// probeTimeout is the wall-clock limit for one probe invocation.
const probeTimeout = 20 * time.Second

// ProbeToolBinary runs a whitelisted "print usage" probe against an uploaded
// binary and returns the captured output.
//
// flag must be one of the allowed usage flags. The command is executed with an
// empty working directory and without the agent's proxy environment, so a
// probe cannot be steered at a network target.
func (s *Server) ProbeToolBinary(path string) (output string, err error) {
	return s.ProbeToolBinaryWithFlag(path, "--help")
}

// ProbeToolBinaryWithFlag runs the binary with one whitelisted usage flag.
//
// The argv is composed by the platform, never by the caller: the agent picks
// only WHICH flag, so it cannot smuggle a target or a script path into the
// probe.
func (s *Server) ProbeToolBinaryWithFlag(path, flag string) (output string, err error) {
	if !probeAllowArgs[flag] {
		return "", fmt.Errorf("探测参数 %q 不被允许", flag)
	}
	cmd := exec.Command(path, flag)
	cmd.Dir = os.TempDir()
	// No proxy env: a probe must not be able to reach out. Keep PATH only so
	// shell-script tools can find their interpreter.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	done := make(chan struct{})
	var buf strings.Builder
	cmd.Stdout = &limitedWriter{w: &buf, limit: probeMaxOutput}
	cmd.Stderr = &limitedWriter{w: &buf, limit: probeMaxOutput}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(probeTimeout):
		_ = cmd.Process.Kill()
		<-done
		return buf.String(), fmt.Errorf("探测超时")
	}
	return buf.String(), nil
}

// limitedWriter truncates output at a byte limit so a chatty binary cannot
// flood the database.
type limitedWriter struct {
	w     *strings.Builder
	limit int
	n     int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n >= l.limit {
		return len(p), nil
	}
	room := l.limit - l.n
	if len(p) > room {
		l.w.Write(p[:room])
		l.n = l.limit
		return len(p), nil
	}
	l.w.Write(p)
	l.n += len(p)
	return len(p), nil
}

// ─────────────────────────── 认领触发 ───────────────────────────

// toolassets_triggerClaim wakes the internal tool-keeper agent so it inspects
// and claims freshly uploaded binaries.
//
// The agent does the semantic work; the platform has already done the
// deterministic part (hash, platform sniff, on-disk placement). If the keeper
// agent is not configured the upload still succeeds — the assets simply stay
// in state=uploaded and can be claimed later.
func (s *Server) toolassets_triggerClaim(created []map[string]any) {
	if len(created) == 0 {
		return
	}
	var names []string
	for _, c := range created {
		if n, ok := c["name"].(string); ok && n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return
	}
	msg := fmt.Sprintf(
		"有 %d 个新上传的外部工具待认领：%s。\n\n"+
			"请对每个工具：\n"+
			"1) 用 probe_tool --help 之类的方式读取它的用法（平台已限制探测参数，只能用 --help/-h/--version 这类只读参数）。\n"+
			"2) 判断它属于哪一类（scanner/brute/poc/proxy/recon/crawler/utility）、风险等级（low/medium/high/critical）。\n"+
			"3) 用 claim_tool 登记结论与调用示例。\n"+
			"注意：登记的契约只会作为平台权限上限的**子集**生效，你无法放宽平台的限制。",
		len(names), strings.Join(names, ", "))

	title := fmt.Sprintf("工具认领 · %s", strings.Join(names, ", "))
	s.StartTriggeredRun("tool_keeper", title, msg, 0, false, "外部工具认领", title)
}

// ─────────────────────────── 契约写入 ───────────────────────────

// toolAssetClaim is the request the keeper agent posts when it has finished
// inspecting a tool.
type toolAssetClaim struct {
	AssetID      int64              `json:"asset_id"`
	Category     string             `json:"category"`
	RiskLevel    string             `json:"risk_level"`
	Summary      string             `json:"summary"`
	UsageExample string             `json:"usage_example"`
	Contract     *db.ToolContract   `json:"contract"`
	ProbeOutput  string             `json:"probe_output"`
}

// POST /api/tool-assets/{id}/claim — the keeper agent records its conclusion.
//
// The contract is validated against the platform's limits BEFORE it is
// stored: an agent can narrow the permissions but never widen them. A
// contract that fails validation is rejected outright (not silently
// downgraded) so the agent gets a clear signal to fix its output.
func (s *Server) toolAssetClaim(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	asset, err := pg.ToolAssetByID(id)
	if err != nil {
		writeErr(w, 404, "工具不存在")
		return
	}
	var req toolAssetClaim
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误: "+err.Error())
		return
	}
	if req.Category != "" && !inSet(req.Category,
		"scanner", "brute", "poc", "proxy", "recon", "crawler", "utility") {
		writeErr(w, 400, "category 只能是 scanner/brute/poc/proxy/recon/crawler/utility")
		return
	}
	if req.RiskLevel != "" && !inSet(req.RiskLevel, "low", "medium", "high", "critical") {
		writeErr(w, 400, "risk_level 只能是 low/medium/high/critical")
		return
	}
	// 认领时若给出了类别，平台按该类别重算一份更贴合的上限（认领前是通用
	// 的 utility 版）。这条逻辑必须与 agent 侧的 claim_tool 保持一致——
	// 两条路径的行为不同会让"重新认领"的结果取决于走了哪个入口。
	if req.Category != "" {
		fresh := db.DefaultContractJSON(req.Category)
		if err := pg.SetToolAssetLimits(id, fresh); err == nil {
			asset.Limits = fresh
		}
	}
	if req.Contract != nil {
		limits, lerr := db.ParseContract(asset.Limits)
		if lerr != nil {
			writeErr(w, 500, "平台权限上限解析失败: "+lerr.Error())
			return
		}
		if err := db.ValidateContractAgainstLimits(req.Contract, limits); err != nil {
			writeErr(w, 400, "契约未通过平台校验: "+err.Error())
			return
		}
		raw, merr := json.Marshal(req.Contract)
		if merr != nil {
			writeErr(w, 500, "契约序列化失败")
			return
		}
		if err := pg.SetToolAssetContract(id, raw); err != nil {
			writeErrRedacted(w, 500, err.Error())
			return
		}
	}
	if err := pg.UpdateToolAssetMeta(id, req.Category, req.RiskLevel, req.Summary,
		req.UsageExample, req.ProbeOutput, "tool_keeper"); err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	updated, _ := pg.ToolAssetByID(id)
	// Same as the agent-side claim: publish to the catalogue and announce it
	// to the running tasks.
	s.syncToolAssetsToCatalog()
	if updated != nil {
		s.notifyToolAvailable(updated)
	}
	writeJSON(w, 200, map[string]any{"ok": true, "asset": updated})
}

// POST /api/tool-assets/{id}/limits — an administrator sets the platform's
// permission ceiling for a tool. Only this path can widen anything.
func (s *Server) toolAssetSetLimits(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	var req struct {
		Limits *db.ToolContract `json:"contract_limits"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "请求格式错误: "+err.Error())
		return
	}
	if req.Limits == nil {
		writeErr(w, 400, "缺少 contract_limits")
		return
	}
	raw, merr := json.Marshal(req.Limits)
	if merr != nil {
		writeErr(w, 500, "序列化失败")
		return
	}
	if err := pg.SetToolAssetLimits(id, raw); err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	// Re-validate the existing contract against the new ceiling: tightening
	// the ceiling may invalidate what the keeper already claimed.
	asset, aerr := pg.ToolAssetByID(id)
	if aerr == nil {
		if contract, cerr := db.ParseContract(asset.Contract); cerr == nil && len(contract.Params) > 0 {
			if verr := db.ValidateContractAgainstLimits(contract, req.Limits); verr != nil {
				_ = pg.SetToolAssetContract(id, json.RawMessage(`{}`))
				writeJSON(w, 200, map[string]any{
					"ok": false,
					"note": "已保存新上限；原契约不再合规，已清空，需要重新认领",
					"reason": verr.Error(),
				})
				return
			}
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func inSet(v string, set ...string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

// parseID reads a numeric path segment.
func parseID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// toolAssetHostPlatform is the platform label of the machine the server runs
// on; used to decide whether an uploaded binary is directly runnable here.
func toolAssetHostPlatform() string {
	if runtime.GOOS == "windows" {
		return "windows"
	}
	return runtime.GOOS
}

// effectiveScopeEnforcement returns the scope-enforcement mode that actually
// applies to an asset.
//
// Precedence:
//  1. an explicit scope_enforcement in the platform's limits — an
//     administrator's deliberate choice always wins;
//  2. otherwise, the mode implied by the asset's category.
//
// The fallback matters for assets uploaded before this dimension existed: their
// limits carry no scope_enforcement, so without a fallback they would all
// report "enforced" even when the tool plainly does not act on network
// targets (a local text processor, say).
func effectiveScopeEnforcement(asset *db.ToolAsset) string {
	if asset == nil {
		return db.ScopeEnforced
	}
	if lim, err := db.ParseContract(asset.Limits); err == nil && lim.ScopeEnforcement != "" {
		return lim.ScopeEnforcement
	}
	if asset.Category != "" {
		mode, _ := db.SuggestScopeEnforcement(asset.Category)
		return mode
	}
	return db.ScopeEnforced
}
