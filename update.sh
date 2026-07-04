#!/usr/bin/env bash
#
# ArchiveSync 更新脚本 (Linux)
#
# 读取 install.sh 生成的 .install.conf，(可选)拉取最新代码、重建前后端，
# 并热更新到原安装目录，保留现有 config.yaml 与数据。
#
#   ./update.sh              # git pull + 重建 + 重新部署 + 重启服务
#   ./update.sh --no-pull    # 跳过 git pull，仅用当前代码重建部署
#
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INFO_FILE="${REPO_DIR}/.install.conf"

# --- 颜色 -------------------------------------------------------------------
if [ -t 1 ]; then
  C_G=$'\e[32m'; C_Y=$'\e[33m'; C_R=$'\e[31m'; C_0=$'\e[0m'
else
  C_G=""; C_Y=""; C_R=""; C_0=""
fi
info() { echo "${C_G}==>${C_0} $*"; }
warn() { echo "${C_Y}警告:${C_0} $*" >&2; }
die()  { echo "${C_R}错误:${C_0} $*" >&2; exit 1; }

# --- 读取安装记录 -----------------------------------------------------------
if [ ! -f "${INFO_FILE}" ]; then
  die "未找到安装记录 ${INFO_FILE}。请先在本目录运行 ./install.sh 完成安装。"
fi
# shellcheck disable=SC1090
. "${INFO_FILE}"

INSTALL_DIR="${ARCHIVE_SYNC_INSTALL_DIR:-}"
BIN_NAME="${ARCHIVE_SYNC_BIN_NAME:-archive-sync}"
SVC_USER="${ARCHIVE_SYNC_SVC_USER:-}"
[ -n "${INSTALL_DIR}" ] || die "安装记录缺少 ARCHIVE_SYNC_INSTALL_DIR，请重新运行 ./install.sh"

# root / sudo
if [ "$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi

echo
echo "  ArchiveSync 更新 —— 目标目录: ${INSTALL_DIR}"
echo

# --- 1. 拉取最新代码 --------------------------------------------------------
if [ "${1:-}" != "--no-pull" ] && [ -d "${REPO_DIR}/.git" ]; then
  info "拉取最新代码 (git pull --ff-only) …"
  git -C "${REPO_DIR}" pull --ff-only || warn "git pull 失败，改用当前代码继续构建"
fi

# --- 2. 构建前端 + 后端 -----------------------------------------------------
command -v go >/dev/null 2>&1 || die "未找到 go，请安装 Go 1.25+"
if command -v npm >/dev/null 2>&1; then
  info "构建前端 (web/) …"
  ( cd "${REPO_DIR}/web" && npm install --no-audit --no-fund && npm run build )
else
  warn "未找到 npm，跳过前端构建（沿用已存在的 web/dist）"
fi
info "编译后端二进制 …"
( cd "${REPO_DIR}" && CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X archivesync/internal/version.Commit=$(git -C "${REPO_DIR}" rev-parse --short HEAD 2>/dev/null || echo release) -X archivesync/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o "${REPO_DIR}/${BIN_NAME}" ./cmd/archive-sync )

# --- 3. 停服务 → 替换二进制 → 起服务 ---------------------------------------
UNIT="/etc/systemd/system/archive-sync.service"
HAS_SVC=0
if [ -f "${UNIT}" ] && command -v systemctl >/dev/null 2>&1; then HAS_SVC=1; fi

if [ "${HAS_SVC}" = "1" ]; then
  info "停止服务 …"
  $SUDO systemctl stop archive-sync.service || true
fi

info "更新二进制 → ${INSTALL_DIR}/${BIN_NAME}"
$SUDO install -m 0755 "${REPO_DIR}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
if [ -n "${SVC_USER}" ] && id "${SVC_USER}" >/dev/null 2>&1; then
  $SUDO chown "${SVC_USER}:${SVC_USER}" "${INSTALL_DIR}/${BIN_NAME}" || true
fi

if [ "${HAS_SVC}" = "1" ]; then
  info "启动服务 …"
  $SUDO systemctl start archive-sync.service
  sleep 1
  $SUDO systemctl --no-pager --lines=0 status archive-sync.service || true
else
  warn "未检测到 systemd 服务，请手动重启 ArchiveSync（例如: ${BIN_NAME} serve）"
fi

echo
info "更新完成: $("${INSTALL_DIR}/${BIN_NAME}" version 2>/dev/null || echo "${BIN_NAME}")"
echo "  配置与数据保持不变；数据库迁移会在服务启动时自动执行。"
echo
