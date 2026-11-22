import { archivePhaseLabel } from "./archive-phase.ts";
import assert from "node:assert/strict";
import test from "node:test";

// 后端真实会写进 task_archives.phase 的全部取值：
// server/task_archives.go 各 UpdateTaskArchiveProgress 的参数，
// db/task_archives.go 的 queued / starting / failed / interrupted，
// db/task_archives_restore.go 的 ready / database_restored。
// 后端加阶段时这里会先红，等于给「中文界面又冒出英文」设了闸。
const BACKEND_PHASES = [
  "queued",
  "starting",
  "ready",
  "failed",
  "interrupted",
  "stage_files",
  "snapshot_database",
  "snapshot_traffic",
  "compress",
  "compact_traffic",
  "compact_database",
  "verify_package",
  "restore_traffic",
  "restore_files",
  "restore_database",
  "database_restored",
  "consume_package",
  "stage_package_delete",
  "delete_metadata",
];

test("后端每个阶段都有中文说法，不会退化成兜底文案", () => {
  for (const phase of BACKEND_PHASES) {
    const label = archivePhaseLabel(phase);
    assert.notEqual(label, "处理中", `${phase} 未收录，界面上会显示成「处理中」`);
  }
});

test("阶段文案里不残留英文内部标记", () => {
  for (const phase of BACKEND_PHASES) {
    const label = archivePhaseLabel(phase);
    assert.ok(label.length > 0, `${phase} 翻译为空`);
    assert.ok(!/[A-Za-z_]/.test(label), `${phase} 的文案含英文/下划线：${label}`);
  }
});

test("归档与还原阶段要能区分开，不能笼统写「处理中」", () => {
  // 归档和还原都有「处理文件」这一步，但用户看到的应该是两件事。
  assert.notEqual(archivePhaseLabel("snapshot_database"), archivePhaseLabel("restore_database"));
  assert.notEqual(archivePhaseLabel("compact_traffic"), archivePhaseLabel("restore_traffic"));
});

test("未知标记兜底成中文，不透传内部 token", () => {
  // 后端将来新增阶段而前端没跟上时，宁可显示「处理中」。
  for (const unknown of ["brand_new_phase", "STEP_X", "???", "0x20"]) {
    assert.equal(archivePhaseLabel(unknown), "处理中", unknown);
  }
});

test("空值不炸，也不渲染空白", () => {
  for (const empty of [null, undefined, "", "   "]) {
    assert.equal(archivePhaseLabel(empty), "处理中", String(empty));
  }
});

test("两端空格不影响识别（列里存脏值时仍能翻译）", () => {
  assert.equal(archivePhaseLabel(" compress "), "打包压缩");
});
