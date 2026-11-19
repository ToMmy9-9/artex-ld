#!/usr/bin/env bash
# artex-ld 运维脚本 —— 启停 / 状态 / 健康监测 / 日志 / 备份
#
# 本脚本为本机部署时新增（不属于上游仓库文件），与仓库自带脚本分工：
#   deploy.sh     一键部署（构建 + 启动 + 健康检查），首次部署用
#   update.sh     升级
#   manage.sh     日常运维 ← 本脚本
#
# 用法: ./manage.sh <命令> [参数]
#   start                     启动容器栈，并等待服务就绪
#   stop                      停止容器栈（容器与数据卷均保留）
#   restart [artex|postgres]  重启服务，默认 artex
#   status                    状态一览：容器 / 服务 / LLM / 资源 / 自启
#   health                    健康检查：单行输出 + 退出码（适合 cron、监控系统）
#   watch [秒] [--auto-restart]
#                             持续监测，默认 10 秒一轮；加 --auto-restart 时
#                             发现异常自动拉起容器（两次拉起间隔不小于 60 秒），
#                             自动拉起记录写入 /var/log/artex-ld-watch.log
#   logs [artex|postgres] [行数] [-f]
#                             查看日志，默认 artex 最近 100 行；-f 持续跟随
#   rebuild                   从当前源码重新构建镜像并重建容器
#   backup [目录]             备份数据库 + data 目录 + .env，默认 ./backups
#   shell                     进入 artex 容器
#   psql                      进入 PostgreSQL 交互终端
#   help                      显示本帮助
#
# health / watch 退出码：
#   0 健康   1 应用无响应   2 容器未运行或未创建   3 Docker 不可用
#
# 可用环境变量覆盖默认值：
#   ARTEX_PORT（默认 8799）  ARTEX_PROXY_PORT（默认 8800）
#   ARTEX_WATCH_INTERVAL（默认 10）
set -uo pipefail

cd "$(cd "$(dirname "$0")" && pwd)" || exit 1

# 本机 Docker 是静态二进制安装，位于 /usr/local/bin；先补齐 PATH 再检测
case ":$PATH:" in *:/usr/local/bin:*) ;; *) export PATH="/usr/local/bin:$PATH" ;; esac

APP_SVC="artex"
DB_SVC="postgres"
APP_PORT="${ARTEX_PORT:-8799}"
PROXY_PORT="${ARTEX_PROXY_PORT:-8800}"
HEALTH_URL="http://127.0.0.1:${APP_PORT}/api/health"
WATCH_LOG="${ARTEX_WATCH_LOG:-/var/log/artex-ld-watch.log}"

info(){ printf '\033[36m[*]\033[0m %s\n' "$*"; }
ok(){   printf '\033[32m[+]\033[0m %s\n' "$*"; }
warn(){ printf '\033[33m[!]\033[0m %s\n' "$*"; }
err(){  printf '\033[31m[x]\033[0m %s\n' "$*" >&2; }
die(){  err "$*"; exit 1; }
bold(){ printf '\033[1m%s\033[0m' "$*"; }

# ── 基础探测 ─────────────────────────────────────
have_docker(){ command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; }

require_docker(){
  have_docker || die "未检测到 docker / docker compose（本机为静态二进制安装，应在 /usr/local/bin）"
  [ -f .env ] || warn "未找到 .env —— 若首次部署请先运行 ./deploy.sh"
}

cid(){ docker compose ps -aq "$1" 2>/dev/null | head -1; }

cstate(){ local id; id="$(cid "$1")"; [ -n "$id" ] && docker inspect -f '{{.State.Status}}' "$id" 2>/dev/null; }

chealth(){ local id; id="$(cid "$1")"; [ -n "$id" ] && docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}-{{end}}' "$id" 2>/dev/null; }

cuptime(){ local id; id="$(cid "$1")"; [ -n "$id" ] && docker ps --filter "id=$id" --format '{{.Status}}' 2>/dev/null; }

is_running(){ [ "$(cstate "$1")" = "running" ]; }

db_query(){
  local id; id="$(cid "$DB_SVC")"; [ -n "$id" ] || return 1
  is_running "$DB_SVC" || return 1
  docker exec -e Q="$1" "$id" sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "$Q"' 2>/dev/null
}

proxy_listening(){ ss -lnt 2>/dev/null | grep -qE "[:.]${PROXY_PORT}[[:space:]]"; }

# HTTP 探测：结果写入 HTTP_CODE / HTTP_BODY，返回 0 表示健康
http_check(){
  local resp
  HTTP_CODE="000"; HTTP_BODY=""
  resp="$(curl -sS --max-time 8 -w '\n%{http_code}' "$HEALTH_URL" 2>/dev/null)" || true
  [ -n "$resp" ] || return 1
  HTTP_CODE="${resp##*$'\n'}"
  HTTP_BODY="${resp%$'\n'*}"
  HTTP_BODY="${HTTP_BODY%$'\n'}"
  case "$HTTP_BODY" in *'"ok":true'*) return 0 ;; *) return 1 ;; esac
}

# ── 统一采集一轮状态 ─────────────────────────────
gather(){
  DOCKER_OK=0; APP_STATE=""; APP_HEALTH=""; APP_UP=""; DB_STATE=""; DB_HEALTH=""; DB_UP=""
  HTTP_CODE="000"; HTTP_BODY=""; PROXY_OK=0; INIT="未知"; LLM_TOTAL=""; LLM_ACTIVE=""; LLM_ENV=""
  have_docker || return 3
  DOCKER_OK=1

  APP_STATE="$(cstate "$APP_SVC")"; APP_HEALTH="$(chealth "$APP_SVC")"; APP_UP="$(cuptime "$APP_SVC")"
  DB_STATE="$(cstate "$DB_SVC")";   DB_HEALTH="$(chealth "$DB_SVC")";   DB_UP="$(cuptime "$DB_SVC")"

  [ -z "$APP_STATE" ] && [ -z "$DB_STATE" ] && return 2
  is_running "$APP_SVC" || return 2
  is_running "$DB_SVC"  || return 2

  proxy_listening && PROXY_OK=1
  http_check || true   # 结果落在 HTTP_CODE / HTTP_BODY

  local auth
  auth="$(curl -sS --max-time 8 "http://127.0.0.1:${APP_PORT}/api/auth/status" 2>/dev/null)" || auth=""
  case "$auth" in
    *'"initialized":true'*)  INIT="已完成" ;;
    *'"initialized":false'*) INIT="未完成" ;;
    *)                        INIT="未知" ;;
  esac

  LLM_TOTAL="$(db_query 'SELECT count(*) FROM llm_profiles' | tr -d '[:space:]')"
  LLM_ACTIVE="$(db_query 'SELECT count(*) FROM llm_profiles WHERE is_default' | tr -d '[:space:]')"
  LLM_ENV="$(grep -E '^(ANTHROPIC_API_KEY|OPENAI_API_KEY)=' .env 2>/dev/null | grep -v '=$' | wc -l)"
  return 0
}

# ── 命令实现 ─────────────────────────────────────
cmd_status(){
  gather; local rc=$?
  echo "$(bold 'artex-ld 状态')"
  echo "  目录        : $(pwd)"
  echo "  版本        : $(curl -sS --max-time 5 "$HEALTH_URL" 2>/dev/null | grep -o '"version":"[^"]*"' | cut -d'"' -f4)"
  echo
  printf '  %-22s %s\n' "artex 容器:" "${APP_STATE:-未创建} ${APP_UP:+($APP_UP)}"
  printf '  %-22s %s\n' "postgres 容器:" "${DB_STATE:-未创建} ${DB_UP:+($DB_UP)}"
  printf '  %-22s %s\n' "服务健康:" "${HTTP_BODY:-无响应} (HTTP $HTTP_CODE)"
  printf '  %-22s %s\n' "代理端口 $PROXY_PORT:" "$([ "$PROXY_OK" = 1 ] && echo '监听中' || echo '未监听')"
  printf '  %-22s %s\n' "管理员初始化:" "$INIT"
  printf '  %-22s %s (默认档 %s)\n' "LLM 配置档:" "$LLM_TOTAL" "$LLM_ACTIVE"
  printf '  %-22s %s\n' "env 中的 LLM key:" "$LLM_ENV"
  echo
  echo "  内存:"; free -h | awk 'NR==2{printf "    总 %s  已用 %s  可用 %s\n",$2,$3,$7}'
  echo "  磁盘:"; df -h / | awk 'NR==2{printf "    %s 已用 %s (剩余 %s)\n",$2,$3,$4}'
  echo "  容器自启: $(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' "$(cid "$APP_SVC")" 2>/dev/null || echo '未知') / Docker 守护 $(systemctl is-enabled docker 2>/dev/null || echo '未知')"
  return $rc
}

cmd_health(){
  gather; local rc=$?
  case $rc in
    0) echo "健康 | HTTP $HTTP_CODE | ${HTTP_BODY:-} | artex=${APP_STATE} postgres=${DB_STATE}" ;;
    1) echo "异常 | 应用无响应 | HTTP $HTTP_CODE | ${HTTP_BODY:-}" ;;
    2) echo "异常 | 容器未运行 | artex=${APP_STATE:-未创建} postgres=${DB_STATE:-未创建}" ;;
    3) echo "异常 | Docker 不可用" ;;
  esac
  return $rc
}

cmd_start(){
  require_docker
  info "启动容器栈 …"
  docker compose up -d --no-build || return 1
  info "等待服务就绪 …"
  local i
  for i in $(seq 1 60); do
    if http_check; then ok "服务已就绪：${HEALTH_URL}（${HTTP_BODY}）"; return 0; fi
    sleep 2
  done
  err "启动超时，查看日志：./manage.sh logs artex 100"
  return 1
}

cmd_stop(){
  require_docker
  info "停止容器栈（数据卷保留）…"
  docker compose stop
}

cmd_restart(){
  require_docker
  local svc="${1:-artex}"
  case "$svc" in
    artex|postgres) ;;
    *) die "未知服务: $svc（可选 artex / postgres）" ;;
  esac
  info "重启 $svc …"
  docker compose restart "$svc"
  if [ "$svc" = "artex" ]; then
    local i
    for i in $(seq 1 30); do http_check && { ok "服务已就绪"; return 0; }; sleep 2; done
    warn "重启后未在 60 秒内就绪"
  fi
}

cmd_logs(){
  require_docker
  local svc="${1:-artex}" lines="${2:-100}" follow=""
  case "$svc" in artex|postgres) ;; *) die "未知服务: $svc" ;; esac
  case "${3:-}" in -f|--follow) follow="-f" ;; esac
  docker compose logs $follow --tail "$lines" "$svc"
}

cmd_rebuild(){
  require_docker
  info "从当前源码重新构建镜像（较慢：含 npm ci 与 Playwright Chromium 下载）…"
  docker compose build artex || die "构建失败"
  info "重建容器 …"
  docker compose up -d --no-build
  cmd_start >/dev/null 2>&1 || true
  ok "重建完成"
}

cmd_backup(){
  require_docker
  local dest="${1:-./backups}"
  mkdir -p "$dest"
  local stamp; stamp="$(date +%Y%m%d-%H%M%S)"
  local out="$dest/artex-ld-$stamp"
  mkdir -p "$out"
  info "导出数据库 → $out/artex-db.sql"
  docker exec "$(cid "$DB_SVC")" sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > "$out/artex-db.sql" 2>/dev/null
  info "复制 data 目录 → $out/data"
  cp -r data "$out/data" 2>/dev/null
  cp .env "$out/.env" 2>/dev/null
  chmod 600 "$out/.env" 2>/dev/null
  ok "备份完成：$out"
}

cmd_watch(){
  require_docker
  local interval="${1:-10}" autorestart=""
  case "${2:-}" in --auto-restart) autorestart=1 ;; esac
  [ "$interval" -ge 1 ] 2>/dev/null || die "轮询间隔必须是正整数秒"
  local last_pull=0
  info "开始监测（每 ${interval}s 一轮，Ctrl-C 退出）…"
  while true; do
    gather; local rc=$?
    local ts; ts="$(date '+%F %T')"
    if [ $rc -eq 0 ]; then
      printf '%s \033[32m健康\033[0m HTTP %s %s\n' "$ts" "$HTTP_CODE" "$HTTP_BODY"
    else
      case $rc in
        1) printf '%s \033[31m应用无响应\033[0m HTTP %s\n' "$ts" "$HTTP_CODE" ;;
        2) printf '%s \033[31m容器未运行\033[0m\n' "$ts" ;;
        *) printf '%s \033[31mDocker 不可用\033[0m\n' "$ts" ;;
      esac
      if [ -n "$autorestart" ]; then
        local now; now="$(date +%s)"
        if [ $((now - last_pull)) -ge 60 ]; then
          last_pull=$now
          warn "尝试自动拉起 …"
          docker compose up -d --no-build >/dev/null 2>&1
          echo "$(date '+%F %T') auto-restart triggered (rc=$rc)" >> "$WATCH_LOG"
        fi
      fi
    fi
    sleep "$interval"
  done
}

cmd_help(){
  sed -n '2,40p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-help}" in
  start)      cmd_start ;;
  stop)       cmd_stop ;;
  restart)    cmd_restart "${2:-artex}" ;;
  status)     cmd_status ;;
  health)     cmd_health ;;
  watch)      cmd_watch "${2:-10}" "${3:-}" ;;
  logs)       cmd_logs "${2:-artex}" "${3:-100}" "${4:-}" ;;
  rebuild)    cmd_rebuild ;;
  backup)     cmd_backup "${2:-}" ;;
  shell)      require_docker; docker compose exec artex /bin/bash ;;
  psql)       require_docker; docker compose exec postgres psql -U artex -d artex ;;
  help|-h|--help) cmd_help ;;
  *) die "未知命令: $1（./manage.sh help 看用法）" ;;
esac
