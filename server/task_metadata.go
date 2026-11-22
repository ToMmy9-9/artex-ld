// task_metadata.go：更新任务列表级元数据（名称/置顶/审核开关）的 HTTP 端点。
// 主要函数/类型：
//   - updateTaskMetadata：PATCH 任务 name/pinned/review_auto/review_enabled，关审核前须清空待审队列。
package server

import (
	"artex/db"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

const (
	maxTaskNameRunes           = 200
	maxTaskMetadataRequestSize = 16 << 10
)

// updateTaskMetadata updates editable metadata and the live asset-sharing policy.
func (s *Server) updateTaskMetadata(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if _, ok := s.m.Task(taskID); !ok {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxTaskMetadataRequestSize)
	var request struct {
		Name              *string `json:"name"`
		Remark            *string `json:"remark"`
		Pinned            *bool   `json:"pinned"`
		ShareGlobalAssets *bool   `json:"share_global_assets"`
		ReviewAuto        *bool   `json:"review_auto"`    // 【二开·P-REVIEW·D2b】自动审核开关
		ReviewEnabled     *bool   `json:"review_enabled"` // 【二开·P-REVIEW·D2c】任务级审核总开关（m03934）
	}
	if err := decode(r, &request); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeErr(w, http.StatusRequestEntityTooLarge, "请求正文过大")
		} else {
			writeErr(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	if request.Name == nil && request.Pinned == nil && request.Remark == nil && request.ShareGlobalAssets == nil && request.ReviewAuto == nil && request.ReviewEnabled == nil {
		writeErr(w, http.StatusBadRequest, "至少需要提供 name、pinned、remark、share_global_assets、review_auto 或 review_enabled")
		return
	}
	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" {
			writeErr(w, http.StatusBadRequest, "任务名称不能为空")
			return
		}
		if utf8.RuneCountInString(name) > maxTaskNameRunes {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("任务名称最多 %d 个字符", maxTaskNameRunes))
			return
		}
		request.Name = &name
	}
	if request.Remark != nil {
		// 【RuoJi6】备注归一：去空白 + 长度上限校验（server 侧拦一次，db 侧再兜一层）。
		remark, err := normalizeRemark(*request.Remark)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		request.Remark = &remark
	}
	// 【二开·审核流 §2.9.6】关闭审核前必须清空待审队列（pending）与待修订稿（rejected）：
	// 任一存在时返回 409 + 数量，要求先批准、修订重送或显式取消；否则 ReviewRunner 停用后
	// 会留下再也无法处置的孤儿稿。开启审核不校验（不回改已有 open，见 §10.1）。
	if request.ReviewEnabled != nil && !*request.ReviewEnabled {
		if t, ok := s.m.Task(taskID); ok && t.Store != nil {
			pend, _ := t.Store.ProposedIntents(1000)
			rej, _ := t.Store.RejectedIntents(1000)
			if len(pend)+len(rej) > 0 {
				writeErr(w, http.StatusConflict, fmt.Sprintf(
					"仍有 %d 条待审、%d 条待修订意图：请先批准、修订重送或显式取消（cancel_intent）后再关闭审核",
					len(pend), len(rej)))
				return
			}
		}
		// 关闭审核时一并关掉自动处置：自动处置完全依赖审核轮（sweep 只拾取
		// review_enabled 的任务），留着 true 就是个再也不生效的僵尸开关 ——
		// 面板上显示"自动审核中"却永远不动，正是「开了但没反应」的经典误判来源。
		off := false
		request.ReviewAuto = &off
	}
	// 反向缺口：审核是关的，却要把自动处置打开。这个组合同样造出僵尸开关 ——
	// 面板显示「自动审核中」，而 sweep 只拾取 review_enabled=true 的任务，审核轮
	// 永远不会跑。它与上面那个方向是同一个 bug 的两面，早期只堵了一面，实测
	// 对 review_enabled=false 的任务 PATCH {"review_auto":true} 仍能成功落库。
	//
	// 这里选择拒绝而不是「顺手把审核也打开」：开启审核会让任务开始拦截意图、
	// 把待审队列堆起来，这是用户没有明说的副作用，静默替人做决定不合适。
	//
	// 判定读数据库而不是内存态：内存里的 t.ReviewEnabled 可能与库不一致（重启、
	// 外部改库、并发 PATCH 都会造成），用它做闸门会误拒合法请求。审核开关是
	// 单表单列，读一次库的代价可以忽略。
	if request.ReviewAuto != nil && *request.ReviewAuto && request.ReviewEnabled == nil {
		var reviewEnabled bool
		if err := s.m.pg.QueryRow(`SELECT review_enabled FROM tasks WHERE id=$1`, taskID).Scan(&reviewEnabled); err == nil {
			if !reviewEnabled {
				writeErr(w, http.StatusConflict, "该任务未开启意图审核：请先开启 review_enabled，再开启自动处置（自动处置依赖审核轮，否则不会生效）")
				return
			}
		}
		// 查询失败不阻断：交给下面的 UpdateTaskMetadata 报出真实错误，
		// 而不是在这里假装「未开启审核」掩盖数据源故障。
	}
	task, err := s.m.UpdateTaskMetadata(taskID, db.TaskPatch{Name: request.Name, Pinned: request.Pinned, Remark: request.Remark, ShareGlobalAssets: request.ShareGlobalAssets, ReviewAuto: request.ReviewAuto, ReviewEnabled: request.ReviewEnabled})
	if err != nil {
		writeErrRedacted(w, http.StatusInternalServerError, err.Error())
		return
	}
	if task == nil {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	writeJSON(w, http.StatusOK, taskDTO(task, s.resolvedTaskStatus(task)))
}
