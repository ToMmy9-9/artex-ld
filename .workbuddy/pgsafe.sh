#!/usr/bin/env bash
# pgsafe.sh —— 只读地测生产库性能的包装脚本。
#
# 背景：2026-11-23 我在生产库 artex_audit 上执行
#   SET default_transaction_read_only=on;
#   EXPLAIN (ANALYZE) DELETE FROM llm_records WHERE ...;
# 误删了 8651 行（EXPLAIN ANALYZE 会真的执行语句，而 default_transaction_read_only
# 只影响「之后开启」的事务，在同一 session 内不生效）。
#
# 用法：
#   bash .workbuddy/pgsafe.sh "EXPLAIN (ANALYZE, BUFFERS) SELECT ..."   # 自动 ROLLBACK
#   bash .workbuddy/pgsafe.sh --commit "INSERT ..."                    # 明确允许写
#
# 规则：
#   - 默认包在 BEGIN ... ROLLBACK 里执行，语句跑完即回滚，生产库零副作用。
#   - 强制 SET TRANSACTION READ ONLY，语句若含写操作会直接报错（而不是靠
#     default_transaction_read_only 那种「同事务无效」的假防护）。
#   - 除非显式传 --commit，否则绝不留事务。
set -uo pipefail

PSQL="/d/PenTesting/Tools/db-connect/postgresql-server/bin/psql.exe"
DSN="postgres://artex:01EDTUB7HLyFQ3ii@127.0.0.1:5432/artex_audit"
export PGCLIENTENCODING=UTF8
export PGPASSWORD=01EDTUB7HLyFQ3ii

COMMIT=0
if [ "${1:-}" = "--commit" ]; then
  COMMIT=1
  shift
fi

SQL="${1:-}"
if [ -z "$SQL" ]; then
  echo "用法: bash pgsafe.sh [--commit] \"<SQL>\"" >&2
  exit 2
fi

if [ "$COMMIT" -eq 1 ]; then
  echo "⚠️  提交模式：语句会真正写入生产库。" >&2
  printf '%s\n' "$SQL" | "$PSQL" "$DSN" -v ON_ERROR_STOP=1
  exit $?
fi

# 只读事务 + 结束时 ROLLBACK。
# 注意 SET TRANSACTION READ ONLY 必须在 BEGIN 之后、任何语句之前下达，
# 它对当前事务立即生效（不同于 default_transaction_read_only）。
cat <<SQL | "$PSQL" "$DSN" -v ON_ERROR_STOP=1
BEGIN;
SET TRANSACTION READ ONLY;
$SQL
ROLLBACK;
SQL
