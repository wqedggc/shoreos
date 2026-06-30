#!/usr/bin/env bash
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok() { echo -e "${GREEN}[OK]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

trap 'log_error "部署中断，请检查上方日志。退出码: $?"' ERR

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_ROOT"

log_info "项目根目录: $PROJECT_ROOT"

log_info "Step 1/7: 检查前置条件..."
command -v go >/dev/null || { log_error "未找到 go"; exit 1; }
command -v mysql >/dev/null || { log_error "未找到 mysql 客户端"; exit 1; }
command -v curl >/dev/null || { log_error "未找到 curl"; exit 1; }
log_ok "$(go version)"
log_ok "$(mysql --version)"

log_info "Step 2/7: 加载环境变量..."
if [ ! -f ".env" ]; then
  if [ -f ".env.example" ]; then
    cp .env.example .env
    log_warn ".env 已从 .env.example 创建，请编辑真实密码后重跑: vi .env"
    exit 0
  fi
  log_error ".env 和 .env.example 均不存在"
  exit 1
fi

set -a
# shellcheck disable=SC1091
source .env
set +a

SHOREOS_HTTP_ADDR="${SHOREOS_HTTP_ADDR:-:8090}"
SHOREOS_MYSQL_USER="${SHOREOS_MYSQL_USER:-root}"
SHOREOS_MYSQL_PASSWORD="${SHOREOS_MYSQL_PASSWORD:-}"
SHOREOS_MYSQL_HOST="${SHOREOS_MYSQL_HOST:-127.0.0.1}"
SHOREOS_MYSQL_PORT="${SHOREOS_MYSQL_PORT:-3306}"
SHOREOS_MYSQL_DATABASE="${SHOREOS_MYSQL_DATABASE:-shoreos}"
SHOREOS_MYSQL_SOCKET="${SHOREOS_MYSQL_SOCKET:-}"
export SHOREOS_HTTP_ADDR SHOREOS_MYSQL_USER SHOREOS_MYSQL_PASSWORD SHOREOS_MYSQL_HOST SHOREOS_MYSQL_PORT SHOREOS_MYSQL_DATABASE SHOREOS_MYSQL_SOCKET

log_ok "环境变量加载完成: ${SHOREOS_MYSQL_USER}@${SHOREOS_MYSQL_HOST}:${SHOREOS_MYSQL_PORT}/${SHOREOS_MYSQL_DATABASE}, http ${SHOREOS_HTTP_ADDR}"
if [ -n "$SHOREOS_MYSQL_SOCKET" ]; then
  log_info "MySQL socket: $SHOREOS_MYSQL_SOCKET"
fi

log_info "Step 3/7: 生成 MySQL 客户端配置..."
mkdir -p configs/mysql
cat > configs/mysql/shoreos_agent.cnf <<EOF
[client]
user=${SHOREOS_MYSQL_USER}
password=${SHOREOS_MYSQL_PASSWORD}
host=${SHOREOS_MYSQL_HOST}
port=${SHOREOS_MYSQL_PORT}
database=${SHOREOS_MYSQL_DATABASE}
default-character-set=utf8mb4
EOF
if [ -n "$SHOREOS_MYSQL_SOCKET" ]; then
  echo "socket=${SHOREOS_MYSQL_SOCKET}" >> configs/mysql/shoreos_agent.cnf
fi
chmod 600 configs/mysql/shoreos_agent.cnf
log_ok "configs/mysql/shoreos_agent.cnf 已生成"

MYSQL_ARGS=(-u"${SHOREOS_MYSQL_USER}" -h"${SHOREOS_MYSQL_HOST}" -P"${SHOREOS_MYSQL_PORT}" --default-character-set=utf8mb4)
if [ -n "${SHOREOS_MYSQL_PASSWORD}" ]; then
  MYSQL_ARGS+=(-p"${SHOREOS_MYSQL_PASSWORD}")
fi
if [ -n "${SHOREOS_MYSQL_SOCKET}" ]; then
  MYSQL_ARGS+=(--socket="${SHOREOS_MYSQL_SOCKET}")
fi

log_info "Step 4/7: 初始化 MySQL schema..."
mysql "${MYSQL_ARGS[@]}" < schema/mysql/001_shoreos_fire.sql
log_ok "MySQL schema 已就绪"

log_info "Step 5/7: 编译服务端..."
if pgrep -f "shoreos-server" >/dev/null; then
  log_warn "检测到已有 shoreos-server 进程，正在停止..."
  pkill -f "shoreos-server" || true
  sleep 2
fi
go build -o shoreos-server ./cmd/server
log_ok "编译完成: shoreos-server"

log_info "Step 6/7: 启动服务..."
nohup ./shoreos-server > server.log 2>&1 &
SERVER_PID=$!
log_info "PID: $SERVER_PID"

log_info "Step 7/7: 健康检查..."
HOST_PORT="${SHOREOS_HTTP_ADDR##*:}"
BASE_URL="http://127.0.0.1:${HOST_PORT}"
for _ in $(seq 1 15); do
  if curl -sf "${BASE_URL}/healthz" >/dev/null; then
    curl -sf "${BASE_URL}/readyz" >/dev/null
    curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/" | grep -q "200"
    log_ok "ShoreOS FIRE 已启动: ${BASE_URL}/"
    exit 0
  fi
  sleep 1
done

log_error "服务启动失败，最近日志如下:"
tail -40 server.log || true
exit 1
