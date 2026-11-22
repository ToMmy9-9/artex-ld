// bodylimit.go：全局请求体上限中间件（JSON/文本类接口兜底）。
//
// 为什么需要它：仓库里 29 处接口各自写了 http.MaxBytesReader，但其余写接口
// （POST /api/tasks、PUT /api/settings、POST /api/llm、POST /api/workspace/write、
// PUT /api/skills/{name}/files/... 等）完全没有上限。实测后果：
//   - POST /api/tasks 传 20MB description → 201 落库，DB 立涨 1.5MB，响应还回吐 20MB；
//   - PUT /api/settings 传 10MB → 200 接受；
//   - POST /api/llm 传 10MB → 200 接受。
// 这些接口的上限只能靠"逐处补"，而入口会越加越多、必然漏。所以这里做一层兜底：
// 所有非上传类请求统一限到 maxJSONBody，上传类路由显式放宽。
//
// 为什么不覆盖上传类：multipart 上传有自己的上限（workspace/tool-assets 512MiB、
// chat 128MiB、skills zip 20MiB、knowledge 32MiB），且必须走流式解析，不能被
// 这里先掐断。按路径给出宽松上限，只防"完全没有上限"导致的无限读盘。
//
// 状态码约定：MaxBytesReader 只把超限暴露成读错误，状态码由各 handler 决定。
// 想回 413 的 handler 用 errors.As 判断 *http.MaxBytesError 即可（见 remarks.go
// 的既有写法）；本中间件不劫持、不改写状态码，避免把无关的 400 误改成 413。
package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	// maxJSONBody 是普通 JSON/文本接口的请求体上限。取 16MiB 而不是更小的值，
	// 是因为 PUT /api/workspace/write 与 PUT /api/skills/{name}/files/... 用 JSON
	// 传整个文件正文（现有最大 SKILL.md 约 20KB，留足两个数量级余量给报告/日志类
	// 粘贴内容），同时把"20MB 描述""10MB 设置"这类滥用挡在外面。
	maxJSONBody = 16 << 20

	// maxUploadBody 是上传类路由的宽松兜底上限，取现有最大上限（512MiB）之上
	// 留一点余量。真正的限制仍由各 handler 自己的 MaxBytesReader / ParseMultipartForm
	// 负责；这里只防"完全没有上限"造成的无界读。
	maxUploadBody = 600 << 20
)

// uploadPathPrefixes 列出必须走 maxUploadBody 的上传类路由前缀。
// 用前缀匹配是因为这些路由后面还跟着子路径。
var uploadPathPrefixes = []string{
	"/api/workspace/upload",
	"/api/tool-assets/upload",
	"/api/chat/upload",
	"/api/skills/upload",
	"/api/knowledge/upload",
	"/api/report-assets/logo",
	"/api/update/apply", // 上传升级包
}

// isUploadPath 判断该路径是否属于上传类（需要放宽上限）。
func isUploadPath(p string) bool {
	for _, pre := range uploadPathPrefixes {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

// limitRequestBody 给写请求套上统一上限。
// 放在 requireAuth 之后，避免未授权请求白白占用读带宽；放在 mux 之前，
// 让所有 handler 都拿不到无界的 r.Body。
func limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil || r.Body == http.NoBody {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			next.ServeHTTP(w, r)
			return
		}
		limit := int64(maxJSONBody)
		if isUploadPath(r.URL.Path) {
			limit = maxUploadBody
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

// maxBytesExceeded 判断 err 是否来自 MaxBytesReader 的超限。
// handler 想在超限时回 413 就调它，其余情况维持原有状态码。
func maxBytesExceeded(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// writeDecodeErr 是 JSON 解码失败的统一出口。
//
// 为什么要有它：全仓 38 处 handler 都是
//
//	if err := decode(r, &req); err != nil { writeErr(w, 400, err.Error()); return }
//
// 而 decode 只把超限暴露成读错误，于是用户传了大请求体时看到的是裸的
// "http: request body too large" 加 400 —— 既没说是哪个上限，也没说该往哪走，
// 还会让人误以为是自己写错了格式。逐处补 maxBytesExceeded 判断会在新增接口时
// 继续漏，所以统一收在这里：超限回 413 并点明上限，其余错误保持原样。
//
// 不改变非超限错误的行为，因此调用方可以无脑替换原来那两行。
func writeDecodeErr(w http.ResponseWriter, err error) {
	if maxBytesExceeded(err) {
		writeErr(w, http.StatusRequestEntityTooLarge, fmt.Sprintf(
			"请求正文超过 %dMB 上限，请精简内容或改用上传接口", maxJSONBody>>20))
		return
	}
	writeErr(w, http.StatusBadRequest, err.Error())
}
