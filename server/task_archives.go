// task_archives.go：任务冷归档/恢复/删除的后台 worker 与批量 HTTP 接口。
// 主要函数/类型：
//   - startTaskArchiveWorker / notifyTaskArchiveWorker / runOneTaskArchiveJob：归档 worker 的启动、唤醒与单任务领取执行。
//   - archiveTask / restoreTask / restoreTaskArchivePayload / deleteTaskArchive：归档、恢复（含载荷安装）与删除的具体流程。
//   - loadRestoredTask：恢复后重新加载任务运行时。
//   - listTaskArchives / getTaskArchive / queueTaskArchive / queueTaskArchiveRestore / queueTaskArchiveDelete：归档记录的查询与单个入队接口。
//   - queueTaskArchivesBatch / restoreTaskArchivesBatch / deleteTaskArchivesBatch：批量归档/恢复/删除接口（含依赖排序 topoTaskIDs）。
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"artex/agent"
	pgdb "artex/db"
	"artex/traffic"
)

const taskArchivePollInterval = 2 * time.Second

type archiveBatchRequest struct {
	TaskIDs    []string `json:"task_ids"`
	ArchiveIDs []int64  `json:"archive_ids"`
}

type archiveBatchItem struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Queued  bool   `json:"queued"`
	Error   string `json:"error,omitempty"`
	Archive int64  `json:"archive_id,omitempty"`
}

func (s *Server) startTaskArchiveWorker() {
	if s.m == nil || s.m.pg == nil {
		return
	}
	if err := s.m.pg.RecoverTaskArchiveJobs(); err != nil {
		log.Printf("[task-archive] recover jobs: %v", err)
	}
	recoveryFailed := false
	if err := recoverTaskArchiveStages(s.m.dir, s.m.pg); err != nil {
		log.Printf("[task-archive] recover file staging: %v", err)
		recoveryFailed = true
	}
	if err := recoverTaskArchiveRestoreStages(s.m.dir, s.m.pg); err != nil {
		log.Printf("[task-archive] recover restore staging: %v", err)
		recoveryFailed = true
	}
	if err := recoverTaskArchiveDeletePackages(s.m.dir, s.m.pg); err != nil {
		log.Printf("[task-archive] recover delete packages: %v", err)
		recoveryFailed = true
	}
	if recoveryFailed {
		log.Printf("[task-archive] worker disabled because startup recovery is incomplete")
		return
	}
	s.archiveWG.Add(1)
	go func() {
		defer s.archiveWG.Done()
		ticker := time.NewTicker(taskArchivePollInterval)
		defer ticker.Stop()
		for {
			if err := s.runOneTaskArchiveJob(); err != nil {
				log.Printf("[task-archive] worker: %v", err)
			}
			select {
			case <-s.ctx.Done():
				return
			case <-s.archiveWake:
			case <-ticker.C:
			}
		}
	}()
	s.notifyTaskArchiveWorker()
}

func (s *Server) notifyTaskArchiveWorker() {
	if s == nil || s.archiveWake == nil {
		return
	}
	select {
	case s.archiveWake <- struct{}{}:
	default:
	}
}

func (s *Server) runOneTaskArchiveJob() error {
	job, err := s.m.pg.ClaimTaskArchiveJob(s.ctx)
	if err != nil || job == nil {
		return err
	}
	var runErr error
	switch job.State {
	case pgdb.Archiving:
		runErr = s.archiveTask(job)
	case pgdb.Restoring:
		runErr = s.restoreTaskArchive(job)
	case pgdb.Deleting:
		runErr = s.deleteTaskArchive(job)
	default:
		runErr = fmt.Errorf("unknown claimed archive state %q", job.State)
	}
	if runErr != nil {
		// error 列会被归档列表直接渲染，必须是人话；原文只进日志，
		// 两边都不能丢：用户看中文，排障照着日志里的英文检索。
		log.Printf("[task-archive] job %d (task %d, %s) 失败: %v", job.ID, job.TaskID, job.State, runErr)
		if failErr := s.m.pg.FailTaskArchiveJob(job.ID, job.State, errors.New(archiveFailureMessage(runErr))); failErr != nil {
			return errors.Join(runErr, failErr)
		}
		return fmt.Errorf("archive job %d (%s): %w", job.ID, job.State, runErr)
	}
	// Drain another queued item without waiting for the periodic poll.
	s.notifyTaskArchiveWorker()
	return nil
}

func (s *Server) archiveTask(job *pgdb.TaskArchive) (runErr error) {
	taskID := strconv.FormatInt(job.TaskID, 10)
	if !s.beginTaskDelete(taskID) {
		return errors.New("任务正在进行其他归档或删除操作")
	}
	committed := false
	defer func() {
		if !committed {
			s.abortTaskDelete(taskID)
		}
	}()
	s.cancelTaskChat(taskID, agent.AbortTaskDeleted)
	s.cancelTaskReview(taskID)
	drainCtx, cancel := context.WithTimeout(s.ctx, taskDeleteDrainTimeout)
	defer cancel()
	if err := s.waitTaskQuiescent(drainCtx, taskID); err != nil {
		return errors.New("任务仍有运行中的 Agent，请先暂停后重试归档")
	}
	if err := s.drainTaskSideQuestions(drainCtx, taskID); err != nil {
		// 超时原文是 "context deadline exceeded"，这条错误会落到归档作业的
		// error 列上并被界面渲染，必须先翻成中文。
		return errors.New(sideDrainMessage(err))
	}
	task, err := s.m.pg.GetTask(job.TaskID)
	if err != nil {
		return err
	}
	if task == nil {
		return pgdb.ErrTaskArchiveNotFound
	}
	_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "stage_files", 10)
	fileStage, err := stageTaskArchiveFiles(s.m.dir, job.ID, taskID, task.ExplorationID)
	if err != nil {
		return err
	}
	defer func() {
		if !committed {
			runErr = errors.Join(runErr, fileStage.rollback())
		}
	}()
	streamPath := filepath.Join(fileStage.payload, filepath.FromSlash(pgdb.TaskArchiveLLMRecordsPath))
	if err := os.MkdirAll(filepath.Dir(streamPath), archiveDirMode); err != nil {
		return err
	}
	streamFile, err := os.OpenFile(streamPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, archiveFileMode)
	if err != nil {
		return err
	}
	_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "snapshot_database", 20)
	snapshot, snapshotErr := s.m.pg.SnapshotTaskArchiveWithLLMRecords(job.TaskID, streamFile)
	var syncErr error
	if snapshotErr == nil {
		syncErr = streamFile.Sync()
	}
	closeErr := streamFile.Close()
	if err := errors.Join(snapshotErr, syncErr, closeErr); err != nil {
		return err
	}
	if snapshot.ExplorationID != task.ExplorationID {
		return errors.New("任务探索记录在归档快照期间发生变化")
	}
	if s.m.traffic != nil && len(snapshot.Hosts) > 0 {
		_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "snapshot_traffic", 38)
		trafficCount, err := s.m.traffic.ExportHosts(snapshot.Hosts, filepath.Join(fileStage.payload, "traffic"))
		if err != nil {
			return err
		}
		snapshot.DataCounts["traffic"] = trafficCount
	}
	archivePath := taskArchivePath(s.m.dir, job.ID, taskID)
	evidenceSnapshots, err := pgdb.ArchiveEvidenceSnapshots(snapshot)
	if err != nil {
		return err
	}
	if err = s.evidenceStore().CopySnapshots(s.ctx, evidenceSnapshots, filepath.Join(fileStage.payload, "evidence")); err != nil {
		return err
	}
	_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "compress", 50)
	originalSize, compressedSize, checksum, err := writeTaskArchivePackage(archivePath, fileStage.payload, snapshot)
	if err != nil {
		return err
	}
	removePackage := true
	defer func() {
		if removePackage {
			_ = os.Remove(archivePath)
		}
	}()
	var trafficStage *traffic.HostDeleteStage
	if s.m.traffic != nil && len(snapshot.ExclusiveHosts) > 0 {
		_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "compact_traffic", 68)
		trafficStage, err = s.m.traffic.StageDeleteHostsExactForArchive(snapshot.ExclusiveHosts, job.ID, job.TaskID)
		if err != nil {
			return err
		}
		defer func() {
			if !committed {
				runErr = errors.Join(runErr, trafficStage.Rollback())
			}
		}()
	}
	_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "compact_database", 78)
	if err := s.m.pg.CompleteTaskArchive(job.ID, snapshot, archivePath, checksum, originalSize, compressedSize); err != nil {
		return err
	}
	committed = true
	removePackage = false
	// 清理失败不影响归档结果，但 warnings 会直接显示在归档列表里，
	// 所以文案只说"发生了什么、还能不能还原"，原始错误（含本地路径）只进日志。
	if trafficStage != nil {
		if err := trafficStage.Commit(); err != nil {
			log.Printf("[task-archive] task %s 流量热存储清理失败: %v", taskID, err)
			_ = s.m.pg.AppendTaskArchiveWarning(job.ID, "归档已完成，但独占流量热存储清理失败（归档包仍可恢复），详情见服务日志")
		}
	}
	if err := fileStage.commit(); err != nil {
		log.Printf("[task-archive] task %s 暂存目录清理失败: %v", taskID, err)
		_ = s.m.pg.AppendTaskArchiveWarning(job.ID, "归档已完成，但暂存目录残留未清理，详情见服务日志")
	}
	s.engine.StopTask(taskID)
	s.taskAgentMu.Lock()
	delete(s.taskAgents, taskID)
	s.taskAgentMu.Unlock()
	s.m.forgetTask(taskID, job.TaskID)
	return nil
}

func (s *Server) restoreTaskArchive(job *pgdb.TaskArchive) (runErr error) {
	if err := validateArchivePath(s.m.dir, job.ArchivePath); err != nil {
		return err
	}
	// A prior attempt may have committed PostgreSQL and files, then failed only
	// while consuming the package. Finish that cleanup without replaying rows.
	if restored, err := s.m.pg.IsTaskArchiveRestored(job.ID); err != nil {
		return err
	} else if restored {
		if err := os.Remove(job.ArchivePath); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := s.m.pg.CompleteTaskArchiveRestore(job.ID); err != nil {
			return err
		}
		s.loadRestoredTask(job.TaskID)
		return nil
	}
	_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "verify_package", 10)
	restoreParent := filepath.Join(taskArchiveRoot(s.m.dir), ".restore")
	if err := os.MkdirAll(restoreParent, archiveDirMode); err != nil {
		return err
	}
	extracted, err := os.MkdirTemp(restoreParent, fmt.Sprintf("%d-", job.ID))
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(extracted) }()
	if err := extractTaskArchivePackage(job.ArchivePath, job.SHA256, extracted); err != nil {
		return err
	}
	manifestRaw, err := os.ReadFile(filepath.Join(extracted, "manifest.json"))
	if err != nil {
		return err
	}
	var snapshot pgdb.TaskArchiveSnapshot
	if err := json.Unmarshal(manifestRaw, &snapshot); err != nil {
		return err
	}
	if snapshot.TaskID != job.TaskID || !pgdb.IsTaskArchiveFormatSupported(snapshot.FormatVersion) {
		return pgdb.ErrTaskArchiveFormatMismatch
	}
	evidenceSnapshots, err := pgdb.ArchiveEvidenceSnapshots(&snapshot)
	if err != nil {
		return err
	}
	return s.evidenceStore().WithInstalledSnapshots(s.ctx, evidenceSnapshots, filepath.Join(extracted, "evidence"), func() error {
		return s.restoreTaskArchivePayload(job, &snapshot, extracted)
	})
}

func (s *Server) restoreTaskArchivePayload(job *pgdb.TaskArchive, snapshot *pgdb.TaskArchiveSnapshot, extracted string) (runErr error) {
	var err error
	var llmRecordsFile *os.File
	relative, hasStreamedLLMRecords := snapshot.StreamedTables["llm_records"]
	if len(snapshot.StreamedTables) > 0 &&
		(snapshot.FormatVersion < 2 || !hasStreamedLLMRecords || len(snapshot.StreamedTables) != 1 ||
			relative != pgdb.TaskArchiveLLMRecordsPath) {
		return pgdb.ErrTaskArchiveFormatMismatch
	}
	if hasStreamedLLMRecords {
		llmRecordsFile, err = os.Open(filepath.Join(extracted, filepath.FromSlash(relative)))
		if err != nil {
			return err
		}
		defer func() {
			if llmRecordsFile != nil {
				_ = llmRecordsFile.Close()
			}
		}()
	}
	if s.m.traffic != nil {
		_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "restore_traffic", 35)
		if _, err := s.m.traffic.ImportArchive(filepath.Join(extracted, "traffic")); err != nil {
			return err
		}
	}
	_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "restore_files", 55)
	files, err := installTaskArchiveFiles(s.m.dir, extracted, strconv.FormatInt(job.TaskID, 10), job.ID)
	if err != nil {
		return err
	}
	databaseRestored := false
	defer func() {
		if !databaseRestored {
			runErr = errors.Join(runErr, files.rollback())
		}
	}()
	_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "restore_database", 70)
	var warnings []string
	if llmRecordsFile != nil {
		warnings, err = s.m.pg.RestoreTaskArchiveWithLLMRecords(
			job.ID, snapshot, job.RemainingTimeoutSeconds, llmRecordsFile,
		)
		closeErr := llmRecordsFile.Close()
		llmRecordsFile = nil
		err = errors.Join(err, closeErr)
	} else {
		warnings, err = s.m.pg.RestoreTaskArchive(job.ID, snapshot, job.RemainingTimeoutSeconds)
	}
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		log.Printf("[task-archive] restored task %d with warning: %s", job.TaskID, warning)
	}
	if len(warnings) > 0 {
		if _, err := s.m.pg.Exploration(snapshot.ExplorationID).AppendActivity(pgdb.Activity{
			Worker: "system", Kind: "system", Summary: "任务还原完成，但部分关联已降级", Detail: strings.Join(warnings, "\n"),
		}); err != nil {
			log.Printf("[task-archive] persist restore warnings for task %d: %v", job.TaskID, err)
		}
	}
	databaseRestored = true
	if err := files.commit(); err != nil {
		return err
	}
	_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "consume_package", 95)
	if err := os.Remove(job.ArchivePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := s.m.pg.CompleteTaskArchiveRestore(job.ID); err != nil {
		return err
	}
	s.loadRestoredTask(job.TaskID)
	return nil
}

func (s *Server) loadRestoredTask(taskID int64) {
	id := strconv.FormatInt(taskID, 10)
	var restored *Task
	for _, task := range s.m.LoadExisting() {
		if task.ID == id {
			restored = task
			break
		}
	}
	if restored == nil {
		if task, ok := s.m.Task(id); ok {
			restored = task
		}
	}
	if restored == nil {
		return
	}
	lifecycle := restored.lifecycleSnapshot()
	if lifecycle.Paused {
		s.engine.Pause(id, agent.AbortPausedOnReload)
	}
	if !isTerminalStatus(lifecycle.Status) {
		s.engine.startDeadlineCoordinator(s.ctx, restored)
	}
}

func (s *Server) deleteTaskArchive(job *pgdb.TaskArchive) error {
	if err := validateArchivePath(s.m.dir, job.ArchivePath); err != nil {
		return err
	}
	_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "stage_package_delete", 35)
	staged, moved, err := stageTaskArchivePackageDelete(job.ArchivePath, job.ID)
	if err != nil {
		return err
	}
	_ = s.m.pg.UpdateTaskArchiveProgress(job.ID, "delete_metadata", 70)
	if err := s.m.pg.DeleteTaskArchiveStub(job.ID); err != nil {
		if moved {
			_ = os.Rename(staged, job.ArchivePath)
		}
		return err
	}
	if moved {
		if err := os.Remove(staged); err != nil && !os.IsNotExist(err) {
			log.Printf("[task-archive] archived task %d metadata deleted; stale package cleanup failed: %v", job.TaskID, err)
		}
	}
	return nil
}

func validateArchivePath(dataDir, candidate string) error {
	if strings.TrimSpace(candidate) == "" {
		return errors.New("归档包路径为空")
	}
	root, err := filepath.Abs(taskArchiveRoot(dataDir))
	if err != nil {
		return err
	}
	path, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("归档包路径不在受管目录内")
	}
	return nil
}

func (s *Server) listTaskArchives(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	// "all" 是页面筛选下拉里的「全部状态」，等价于不传。
	if state == "all" {
		state = ""
	}
	// 不认识的取值必须报错。state 是精确匹配，静默放过只会返回 0 条，
	// 用户看到「0 个归档」会以为归档数据丢了 —— 这比报错难查得多。
	if state != "" && !pgdb.IsTaskArchiveState(state) {
		writeErr(w, 400, "不支持的归档状态筛选："+state+"，请重新选择")
		return
	}
	result, err := s.m.pg.ListTaskArchives(r.URL.Query().Get("q"), state, page, size)
	if err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) getTaskArchive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "归档 id 无效")
		return
	}
	item, err := s.m.pg.GetTaskArchive(id)
	if err != nil {
		writeErrRedacted(w, 500, err.Error())
		return
	}
	if item == nil {
		writeErr(w, 404, "归档不存在")
		return
	}
	writeJSON(w, 200, item)
}

func (s *Server) queueTaskArchive(w http.ResponseWriter, r *http.Request) {
	id, ok := canonicalTaskID(r.PathValue("id"))
	if !ok {
		writeErr(w, 400, "任务 id 无效")
		return
	}
	numeric, _ := strconv.ParseInt(id, 10, 64)
	item, err := s.m.pg.QueueTaskArchive(numeric)
	if err != nil {
		writeArchiveError(w, err)
		return
	}
	s.notifyTaskArchiveWorker()
	writeJSON(w, http.StatusAccepted, item)
}

func (s *Server) queueTaskArchiveRestore(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "归档 id 无效")
		return
	}
	item, err := s.m.pg.QueueTaskArchiveRestore(id)
	if err != nil {
		writeArchiveError(w, err)
		return
	}
	s.notifyTaskArchiveWorker()
	writeJSON(w, http.StatusAccepted, item)
}

func (s *Server) queueTaskArchiveDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "归档 id 无效")
		return
	}
	item, err := s.m.pg.QueueTaskArchiveDelete(id)
	if err != nil {
		writeArchiveError(w, err)
		return
	}
	s.notifyTaskArchiveWorker()
	writeJSON(w, http.StatusAccepted, item)
}

func (s *Server) queueTaskArchivesBatch(w http.ResponseWriter, r *http.Request) {
	var request archiveBatchRequest
	if err := decode(r, &request); err != nil {
		writeDecodeErr(w, err)
		return
	}
	parsed := normalizeBatchTaskIDs(request.TaskIDs)
	if len(parsed) == 0 {
		writeErr(w, 400, "task_ids 不能为空")
		return
	}
	if len(parsed) > 100 {
		writeErr(w, 400, "一次最多处理 100 个任务")
		return
	}
	ids := make([]string, 0, len(parsed))
	items := make([]archiveBatchItem, 0, len(parsed))
	for _, item := range parsed {
		if !item.valid {
			items = append(items, archiveBatchItem{ID: item.id, Error: "任务 id 无效"})
			continue
		}
		ids = append(ids, item.id)
	}
	ids = s.orderLiveTaskArchiveBatch(ids)
	for _, id := range ids {
		numeric, _ := strconv.ParseInt(id, 10, 64)
		archive, err := s.m.pg.QueueTaskArchive(numeric)
		item := archiveBatchItem{ID: id, OK: err == nil, Queued: err == nil}
		if err != nil {
			// 批量结果会以 "#id（原因）" 拼进 toast，原因同样要能看懂；
			// 认不出的错误给可重试提示，原文留日志。
			if !isKnownArchiveError(err) {
				log.Printf("[task-archive] batch archive task %d 失败: %v", numeric, err)
			}
			item.Error = archiveRequestMessage(err)
		} else {
			item.Archive = archive.ID
		}
		items = append(items, item)
	}
	s.notifyTaskArchiveWorker()
	writeJSON(w, http.StatusAccepted, map[string]any{"items": items})
}

func (s *Server) queueTaskArchiveActionBatch(w http.ResponseWriter, r *http.Request, action string) {
	var request archiveBatchRequest
	if err := decode(r, &request); err != nil {
		writeDecodeErr(w, err)
		return
	}
	ids, err := normalizeArchiveIDs(request.ArchiveIDs)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ids = s.orderColdTaskArchiveBatch(ids, action == "restore")
	items := make([]archiveBatchItem, 0, len(ids))
	for _, id := range ids {
		var archive *pgdb.TaskArchive
		if action == "restore" {
			archive, err = s.m.pg.QueueTaskArchiveRestore(id)
		} else {
			archive, err = s.m.pg.QueueTaskArchiveDelete(id)
		}
		item := archiveBatchItem{ID: strconv.FormatInt(id, 10), Archive: id, OK: err == nil, Queued: err == nil}
		if err != nil {
			// 批量结果会被拼成 "#12（原因）" 进 toast，原因同样必须是能看懂的中文。
			if !isKnownArchiveError(err) {
				log.Printf("[task-archive] batch %s archive %d 失败: %v", action, id, err)
			}
			item.Error = archiveRequestMessage(err)
		} else if archive != nil {
			item.Archive = archive.ID
		}
		items = append(items, item)
	}
	s.notifyTaskArchiveWorker()
	writeJSON(w, http.StatusAccepted, map[string]any{"items": items})
}

func (s *Server) restoreTaskArchivesBatch(w http.ResponseWriter, r *http.Request) {
	s.queueTaskArchiveActionBatch(w, r, "restore")
}

func (s *Server) deleteTaskArchivesBatch(w http.ResponseWriter, r *http.Request) {
	s.queueTaskArchiveActionBatch(w, r, "delete")
}

func normalizeArchiveIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, errors.New("archive_ids 不能为空")
	}
	if len(ids) > 100 {
		return nil, errors.New("一次最多处理 100 个归档")
	}
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("归档 id %d 无效", id)
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *Server) orderLiveTaskArchiveBatch(ids []string) []string {
	sources := map[string][]string{}
	for _, id := range ids {
		if task, ok := s.m.Task(id); ok {
			for _, sourceID := range task.lifecycleSnapshot().SourceTaskIDs {
				sources[id] = append(sources[id], strconv.FormatInt(sourceID, 10))
			}
		}
	}
	return topoTaskIDs(ids, sources, false)
}

func (s *Server) orderColdTaskArchiveBatch(ids []int64, restore bool) []int64 {
	taskToArchive := map[string]int64{}
	sources := map[string][]string{}
	var taskIDs []string
	for _, id := range ids {
		archive, err := s.m.pg.GetTaskArchive(id)
		if err != nil || archive == nil {
			continue
		}
		taskID := strconv.FormatInt(archive.TaskID, 10)
		taskToArchive[taskID] = id
		taskIDs = append(taskIDs, taskID)
		for _, sourceID := range archive.SourceTaskIDs {
			sources[taskID] = append(sources[taskID], strconv.FormatInt(sourceID, 10))
		}
	}
	orderedTasks := topoTaskIDs(taskIDs, sources, restore)
	ordered := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, taskID := range orderedTasks {
		id := taskToArchive[taskID]
		ordered = append(ordered, id)
		seen[id] = true
	}
	for _, id := range ids {
		if !seen[id] {
			ordered = append(ordered, id)
		}
	}
	return ordered
}

// topoTaskIDs orders source->dependent for restore and dependent->source for
// archive/delete. Cycles are impossible at creation, but stable fallback keeps a
// damaged database operable.
func topoTaskIDs(ids []string, sources map[string][]string, restore bool) []string {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	adjacency := map[string][]string{}
	indegree := map[string]int{}
	for id := range set {
		indegree[id] = 0
	}
	for dependent, sourceIDs := range sources {
		for _, source := range sourceIDs {
			if !set[dependent] || !set[source] {
				continue
			}
			from, to := dependent, source
			if restore {
				from, to = source, dependent
			}
			adjacency[from] = append(adjacency[from], to)
			indegree[to]++
		}
	}
	queue := []string{}
	for _, id := range ids {
		if indegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	ordered := make([]string, 0, len(ids))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		ordered = append(ordered, id)
		for _, next := range adjacency[id] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if len(ordered) != len(ids) {
		seen := map[string]bool{}
		for _, id := range ordered {
			seen[id] = true
		}
		for _, id := range ids {
			if !seen[id] {
				ordered = append(ordered, id)
			}
		}
	}
	return ordered
}

// archiveErrorSentinels 是归档相关「可预期前置条件」的全集。
// 新增哨兵时这里和 archiveErrorMessage 必须一起补：测试会逐条校验两者对齐，
// 漏一条就意味着那条错误又变成英文原文直出。
var archiveErrorSentinels = []error{
	pgdb.ErrTaskArchiveNotFound,
	pgdb.ErrTaskArchiveQueued,
	pgdb.ErrTaskArchiveIneligible,
	pgdb.ErrTaskArchiveDependent,
	pgdb.ErrTaskArchiveDeleteBlocked,
	pgdb.ErrTaskArchiveFormatMismatch,
	pgdb.ErrTaskArchiveState,
}

func isKnownArchiveError(err error) bool {
	for _, target := range archiveErrorSentinels {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// archiveErrorMessage 把归档相关的哨兵错误翻成用户能照着做的中文。
//
// 为什么要翻：这些错误原本直接 err.Error() 透传，前端 toast 拼的是
// "归档失败：" + 原文，于是中文界面上出现一句半截英文
// （"归档失败：queued task must be paused before archiving"）。
// 用户既看不懂，也不知道下一步该点哪里。这些错误都是可预期的操作前置条件，
// 不是内部故障，所以逐个说清楚"现在为什么不行、要怎么做"。
func archiveErrorMessage(err error) string {
	switch {
	case errors.Is(err, pgdb.ErrTaskArchiveNotFound):
		return "归档不存在或已被删除"
	case errors.Is(err, pgdb.ErrTaskArchiveQueued):
		return "任务还在并发队列里排队，请先暂停它再归档"
	case errors.Is(err, pgdb.ErrTaskArchiveIneligible):
		return "任务需要先暂停、或已经结束，才能归档"
	case errors.Is(err, pgdb.ErrTaskArchiveDependent):
		return "这个任务的结果正被其它未归档的任务继承，请先处理那些下游任务"
	case errors.Is(err, pgdb.ErrTaskArchiveDeleteBlocked):
		return "这个归档仍被其它归档引用，无法删除"
	case errors.Is(err, pgdb.ErrTaskArchiveFormatMismatch):
		return "不支持的归档格式"
	case errors.Is(err, pgdb.ErrTaskArchiveState):
		return "归档当前状态不允许该操作，请刷新后重试"
	}
	// 未识别的错误保持原文 —— 只在没有 HTTP 出口的场景（日志、内部拼装）里用得到。
	return err.Error()
}

// archiveRequestMessage 用于「用户点了按钮，但请求没被接受」的出口：
// 单个接口的 4xx、批量结果里的逐条原因。
//
// 认得出的前置条件说清楚下一步怎么做；认不出的（数据库抖动之类）给一句能重试的话 ——
// 批量结果会被拼进 toast，把驱动原文塞进去既看不懂，也等于把 schema 送出去。
func archiveRequestMessage(err error) string {
	if isKnownArchiveError(err) {
		return archiveErrorMessage(err)
	}
	return "归档服务暂时不可用，请稍后重试"
}

// archiveFailureMessage 是后台归档作业失败后写进 task_archives.error 的用户可见原因。
//
// 这一列会被归档列表直接渲染（web 侧 <p>{archive.error}</p>），所以它必须是人话：
// 修复前这里存的是 cause.Error()，页面上出现过 "task archive checksum mismatch"、
// "mkdir D:\data\tasks\12\...: Access is denied"，甚至驱动返回的 SQL 原文。分四类处理：
//  1. 可预期前置条件 → 与同步接口同一套中文（同一原因两处说法必须一致）；
//  2. 归档包本身坏了 → 这是用户唯一能自己动手的，必须说清后果；
//  3. 磁盘/权限 → 告诉用户去查什么；
//  4. 其余 → 一句内部错误 + 去查日志。失败阶段不写在这里：FailTaskArchiveJob 保留
//     原 phase，界面上"归档失败"徽章旁边本来就会显示卡在哪一步。
func archiveFailureMessage(err error) string {
	if isKnownArchiveError(err) {
		return archiveErrorMessage(err)
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "task archive checksum mismatch"):
		return "归档包校验不通过，文件可能已损坏，无法继续还原"
	case strings.Contains(msg, "exceeds extraction safety limits"):
		return "归档包解压后体积异常，已中止还原"
	case strings.Contains(msg, "unsafe task archive"):
		return "归档包内含不安全的路径，已中止还原"
	case strings.Contains(msg, "unsupported task archive entry type"):
		return "归档包内含不支持的文件类型，已中止还原"
	case errors.Is(err, syscall.ENOSPC):
		return "磁盘空间不足，操作已中止，请清理磁盘后重试"
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return "没有文件读写权限，操作已中止，请检查数据目录权限"
	}
	// 流程里已经写成中文的原因（如"任务仍有运行中的 Agent，请先暂停后重试归档"）
	// 本身就是给用户看的，原样保留。
	if hasCJK(msg) {
		return msg
	}
	return "归档过程中出现内部错误，操作未完成，详情见服务日志"
}

// hasCJK 判断文案里是否已有汉字：用来区分"写给人看的原因"和"Go 运行时原文"。
func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func writeArchiveError(w http.ResponseWriter, err error) {
	// 认不出的错误是内部故障：与全仓 5xx 出口同一约定，详情只进日志，
	// 客户端拿固定文案，避免把路径/SQL 原文吐出去。
	if !isKnownArchiveError(err) {
		writeErrRedacted(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := http.StatusConflict
	if errors.Is(err, pgdb.ErrTaskArchiveNotFound) {
		status = http.StatusNotFound
	}
	writeErr(w, status, archiveErrorMessage(err))
}
