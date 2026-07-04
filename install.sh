#!/usr/bin/env bash
#
# ArchiveSync 安装脚本 (Linux)
#
# 交互式安装 ArchiveSync：选择安装路径、录入 IAM 接入信息，构建并部署二进制，
# 注册全局命令 archive-sync，并可选安装 systemd 服务。
#
#   sudo ./install.sh
#
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_NAME="archive-sync"

# --- 颜色 -------------------------------------------------------------------
if [ -t 1 ]; then
  C_B=$'\e[1m'; C_G=$'\e[32m'; C_Y=$'\e[33m'; C_R=$'\e[31m'; C_D=$'\e[2m'; C_0=$'\e[0m'
else
  C_B=""; C_G=""; C_Y=""; C_R=""; C_D=""; C_0=""
fi
info()  { echo "${C_G}==>${C_0} $*"; }
warn()  { echo "${C_Y}警告:${C_0} $*" >&2; }
die()   { echo "${C_R}错误:${C_0} $*" >&2; exit 1; }

# root / sudo
if [ "$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi

ask() {
  # ask <提示> <默认值>
  local prompt="$1" def="${2:-}" reply
  if [ -t 0 ]; then
    if [ -n "$def" ]; then read -r -p "$prompt [$def]: " reply || true
    else read -r -p "$prompt: " reply || true; fi
  fi
  echo "${reply:-$def}"
}
ask_secret() {
  local prompt="$1" reply
  if [ -t 0 ]; then read -rs -p "$prompt: " reply || true; echo >&2; fi
  echo "${reply:-}"
}

echo
echo "${C_B}  ArchiveSync 安装程序${C_0}"
echo "${C_D}  备份同步系统 + 管理面板${C_0}"
echo

# --- 1. 收集配置 ------------------------------------------------------------
INSTALL_DIR="$(ask "安装路径" "/opt/archive-sync")"
LISTEN="$(ask "监听地址（如 :8787 或 0.0.0.0:8787）" ":8787")"
# Coerce a bare port (e.g. "8787") into ":8787" so net.Listen accepts it.
case "$LISTEN" in *:*) ;; *) LISTEN=":${LISTEN}" ;; esac
DEFAULT_HOST="$(hostname -f 2>/dev/null || hostname 2>/dev/null || echo localhost)"
BASE_URL="$(ask "面板对外访问地址 (Base URL)" "http://${DEFAULT_HOST}:8787")"
DATA_DIR="$(ask "数据目录" "${INSTALL_DIR}/data")"

echo
echo "${C_B}TransCircle IAM 接入${C_0}（用于面板登录；留空 Client ID 则以开发模式运行，无需登录）"
IAM_ISSUER="$(ask "IAM Issuer" "https://iam.transcircle.org")"
IAM_CLIENT_ID="$(ask "OIDC Client ID（留空=开发模式）" "")"
IAM_CLIENT_SECRET=""
IAM_APP_KEY="archive-sync"
IAM_REQ_PERM=""
IAM_REQ_ROLE=""
IAM_REDIRECT="${BASE_URL%/}/api/auth/callback"
if [ -n "$IAM_CLIENT_ID" ]; then
  IAM_CLIENT_SECRET="$(ask_secret "OIDC Client Secret（公共客户端可留空）")"
  IAM_APP_KEY="$(ask "应用 Key (tc_app)" "archive-sync")"
  IAM_REDIRECT="$(ask "Redirect URL" "$IAM_REDIRECT")"
  IAM_REQ_PERM="$(ask "要求权限（可留空）" "")"
  IAM_REQ_ROLE="$(ask "要求角色（可留空）" "")"
fi

echo
SVC_USER="$(ask "运行服务的系统用户" "archive-sync")"
INSTALL_SVC="$(ask "安装并启用 systemd 服务? (y/n)" "y")"

# --- 2. 构建 ----------------------------------------------------------------
BUILT_BIN=""
if [ -x "${REPO_DIR}/${BIN_NAME}" ]; then
  BUILT_BIN="${REPO_DIR}/${BIN_NAME}"
  info "使用已存在的二进制: ${BUILT_BIN}"
else
  command -v go >/dev/null 2>&1 || die "未找到 go，且无预编译二进制。请安装 Go 1.25+ 或提供 ./${BIN_NAME}"
  if [ ! -f "${REPO_DIR}/web/dist/index.html" ] || ! grep -q '/assets/' "${REPO_DIR}/web/dist/index.html" 2>/dev/null; then
    command -v npm >/dev/null 2>&1 || die "未找到 npm，无法构建前端。请先安装 Node.js 18+"
    info "构建前端 (web/) …"
    ( cd "${REPO_DIR}/web" && npm install --no-audit --no-fund && npm run build )
  else
    info "检测到已构建的前端，跳过 npm 构建"
  fi
  info "编译后端二进制 …"
  ( cd "${REPO_DIR}" && CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X archivesync/internal/version.Commit=$(git -C "${REPO_DIR}" rev-parse --short HEAD 2>/dev/null || echo release) -X archivesync/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      -o "${REPO_DIR}/${BIN_NAME}" ./cmd/archive-sync )
  BUILT_BIN="${REPO_DIR}/${BIN_NAME}"
fi

# --- 3. 部署 ----------------------------------------------------------------
info "创建目录 ${INSTALL_DIR}"
$SUDO mkdir -p "${INSTALL_DIR}" "${DATA_DIR}"
info "安装二进制到 ${INSTALL_DIR}/${BIN_NAME}"
$SUDO install -m 0755 "${BUILT_BIN}" "${INSTALL_DIR}/${BIN_NAME}"

CONFIG_PATH="${INSTALL_DIR}/config.yaml"
info "写入配置 ${CONFIG_PATH}"
TMP_CFG="$(mktemp)"
cat > "${TMP_CFG}" <<EOF
listen: "${LISTEN}"
data_dir: "${DATA_DIR}"
base_url: "${BASE_URL}"
session_ttl_hours: 24
iam:
  issuer: "${IAM_ISSUER}"
  client_id: "${IAM_CLIENT_ID}"
  client_secret: "${IAM_CLIENT_SECRET}"
  redirect_url: "${IAM_REDIRECT}"
  app_key: "${IAM_APP_KEY}"
  scopes: ["openid", "profile", "email", "tc.permissions"]
  required_permission: "${IAM_REQ_PERM}"
  required_role: "${IAM_REQ_ROLE}"
EOF
$SUDO install -m 0600 "${TMP_CFG}" "${CONFIG_PATH}"
rm -f "${TMP_CFG}"

# --- 4. 全局命令 archive-sync (wrapper) -------------------------------------
info "注册全局命令 /usr/local/bin/${BIN_NAME}"
TMP_WRAP="$(mktemp)"
cat > "${TMP_WRAP}" <<EOF
#!/usr/bin/env bash
# ArchiveSync 全局命令封装：固定使用安装时写入的配置。
export ARCHIVE_SYNC_CONFIG="${CONFIG_PATH}"
exec "${INSTALL_DIR}/${BIN_NAME}" "\$@"
EOF
$SUDO install -m 0755 "${TMP_WRAP}" "/usr/local/bin/${BIN_NAME}"
rm -f "${TMP_WRAP}"

# --- 5. systemd 服务 --------------------------------------------------------
if [ "${INSTALL_SVC}" = "y" ] || [ "${INSTALL_SVC}" = "Y" ]; then
  if ! id "${SVC_USER}" >/dev/null 2>&1; then
    info "创建系统用户 ${SVC_USER}"
    $SUDO useradd --system --no-create-home --shell /usr/sbin/nologin "${SVC_USER}" || \
      warn "创建用户失败，服务将以 root 运行"
  fi
  $SUDO chown -R "${SVC_USER}:${SVC_USER}" "${INSTALL_DIR}" 2>/dev/null || true

  UNIT="/etc/systemd/system/archive-sync.service"
  info "写入 systemd 单元 ${UNIT}"
  TMP_UNIT="$(mktemp)"
  cat > "${TMP_UNIT}" <<EOF
[Unit]
Description=ArchiveSync 备份同步服务
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SVC_USER}
Group=${SVC_USER}
Environment=ARCHIVE_SYNC_CONFIG=${CONFIG_PATH}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${BIN_NAME} serve
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=full
ReadWritePaths=${DATA_DIR} ${INSTALL_DIR}

[Install]
WantedBy=multi-user.target
EOF
  $SUDO install -m 0644 "${TMP_UNIT}" "${UNIT}"
  rm -f "${TMP_UNIT}"
  $SUDO systemctl daemon-reload
  $SUDO systemctl enable --now archive-sync.service
  sleep 1
  $SUDO systemctl --no-pager --lines=0 status archive-sync.service || true
fi

# --- 6. 记录安装信息（供 update.sh 使用） ----------------------------------
SERVICE_FLAG=0
if [ "${INSTALL_SVC}" = "y" ] || [ "${INSTALL_SVC}" = "Y" ]; then SERVICE_FLAG=1; fi
INFO_FILE="${REPO_DIR}/.install.conf"
info "记录安装信息到 ${INFO_FILE}（供 ./update.sh 使用）"
cat > "${INFO_FILE}" <<EOF
# ArchiveSync 安装记录 —— 由 install.sh 生成，供 update.sh 读取。
# 本文件包含本机安装路径，已被 .gitignore 忽略，请勿提交。
ARCHIVE_SYNC_INSTALL_DIR="${INSTALL_DIR}"
ARCHIVE_SYNC_BIN_NAME="${BIN_NAME}"
ARCHIVE_SYNC_CONFIG="${CONFIG_PATH}"
ARCHIVE_SYNC_DATA_DIR="${DATA_DIR}"
ARCHIVE_SYNC_SVC_USER="${SVC_USER}"
ARCHIVE_SYNC_SERVICE="${SERVICE_FLAG}"
ARCHIVE_SYNC_INSTALLED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
EOF

# --- 完成 -------------------------------------------------------------------
echo
info "${C_B}安装完成${C_0}"
echo
echo "  面板地址 : ${BASE_URL}"
echo "  配置文件 : ${CONFIG_PATH}"
echo "  数据目录 : ${DATA_DIR}"
echo "  全局命令 : ${BIN_NAME}  (例如 '${BIN_NAME} status')"
if [ -n "$IAM_CLIENT_ID" ]; then
  echo
  echo "  ${C_Y}请在 TransCircle IAM 的 OIDC 客户端中登记以下 Redirect URL:${C_0}"
  echo "    ${IAM_REDIRECT}"
else
  echo
  echo "  ${C_Y}当前为开发模式（无需登录）。生产环境请运行 '${BIN_NAME} iam' 配置 IAM。${C_0}"
fi
echo
echo "  常用命令:"
echo "    ${BIN_NAME} status          # 查看状态"
echo "    ${BIN_NAME} iam             # 重新配置 IAM"
echo "    ${BIN_NAME} backup <目标>    # 立即备份"
echo "    systemctl restart archive-sync   # 重启服务"
echo "    ./update.sh                 # 拉取更新并重新部署到本目录"
echo
