# ArchiveSync

> 一个用 **Go** 编写的备份同步系统，附带 **Vue 3** 管理面板。单文件部署，内嵌前端。

在管理面板上配置**备份渠道**（S3 / Cloudflare R2 / MinIO / 本地目录）、**备份目标**（源目录 + 调度 + 分层保留策略 + 多渠道分发）与**通知渠道**（Discord / Telegram / 邮件 / Webhook）。可在**详情页按日期浏览并下载**已存归档、查看每次执行的日志。面板通过 **TransCircle IAM**（OIDC + PKCE）单点登录。

---

## ✨ 功能特性

- **备份渠道**：S3 兼容对象存储（AWS S3、Cloudflare R2、MinIO，可自定义 Endpoint / Region / Path-Style）、本地目录。
- **备份目标**：任意源目录 → 打包为 `tar.gz` / `zip`，支持 include / exclude glob；每个目标可分发到**多个渠道**。每个目标有**用户自定义、创建后不可修改的唯一标识**，作为存储目录名。
- **目录选择器**：源目录与本地渠道路径均可通过弹窗**浏览服务器文件系统选择**，避免手输出错。
- **灵活调度**：每日定时（指定时刻或“每天 N 次”均匀分布）、Cron 表达式（5/6 段）、固定间隔；均支持时区（如 `Asia/Shanghai`）。
- **分层保留策略**：面板提供“分层 / 按天数 / 按份数”三种引导式方案并实时预览效果。可实现“**保留今天(UTC+8)的全部 24 次备份；更早的每天只留最接近 00:00（或 12:00 与 24:00）的一份；共保留 7 天**”，另有全局份数上限与最小保留数。
- **浏览与下载**：目标详情页按 `日期 → 文件` 浏览并**下载**归档、查看**最近执行结果与逐渠道日志**；渠道详情页以文件夹形式浏览该渠道下所有备份并下载。
- **通知（卡片化，无 emoji）**：备份开始 / 成功 / 失败时推送。Discord（机器人或 Webhook）发送标准 **embed 卡片**；Webhook 自动识别 Discord / Slack 地址发送对应格式，其余端点收到结构化 JSON。内容含目标名、状态、大小、耗时、逐渠道结果、错误详情。
- **IAM 单点登录**：对接 TransCircle IAM（OIDC Authorization Code + PKCE，服务端会话），可选按权限 / 角色限制访问。开发模式（未配置 IAM）免登录，仅供本地测试。
- **敏感信息加密**：渠道 / 通知的密钥在 SQLite 中以 AES-256-GCM 加密存储；API 响应中脱敏，更新留空表示保留原值。
- **全局命令 `archive-sync`**：查看状态、重新配置 IAM、立即备份、管理配置。
- **一键安装脚本**：选择安装路径、录入 IAM 信息、注册全局命令与 systemd 服务；也可直接在当前目录运行用于测试。

---

## 🧱 技术栈与结构

- 后端：Go 1.25（纯 Go，`CGO_ENABLED=0`），SQLite via `modernc.org/sqlite`，路由 chi，调度 robfig/cron，S3 via aws-sdk-go-v2，OIDC via coreos/go-oidc。
- 前端：Vue 3 `<script setup>` + Vite + Pinia + Vue Router，自建轻量设计系统与全套自定义组件（下拉 / 开关 / 单选卡片 / 模态 / 文件浏览器等，不依赖第三方 UI 库）；构建产物由 `go:embed` 内嵌进二进制。

```
cmd/archive-sync/        入口
internal/
  models/  config/  crypto/  store/(sqlite)
  storage/(s3,local)  notify/(discord,telegram,smtp,webhook)
  archive/  retention/  backup/  scheduler/  auth/(OIDC)
  api/(handlers, fs 目录浏览, storage_browse 对象浏览/下载)  cli/
web/                     Vue 前端（构建到 web/dist 并内嵌）
install.sh               Linux 安装脚本
```

设计契约详见 [`SPEC.md`](./SPEC.md)。

---

## 🚀 快速开始（本地测试，无需 IAM）

```bash
# 1. 构建前端
cd web && npm install && npm run build && cd ..

# 2. 运行（未配置 IAM 时进入开发模式，免登录，仅供本地测试）
go run ./cmd/archive-sync serve
# 打开 http://localhost:8787
```

或先编译二进制：

```bash
CGO_ENABLED=0 go build -o archive-sync ./cmd/archive-sync
./archive-sync serve
```

> 开发模式下所有请求被视为“本地管理员”。**请勿在生产环境使用开发模式。**

前端开发热更新（另开终端，代理到后端）：

```bash
cd web && npm run dev    # http://localhost:5173，/api 自动代理到 :8787
```

---

## 📦 安装（Linux）

```bash
sudo ./install.sh
```

脚本会交互式询问：安装路径、监听地址、面板外部地址、以及 IAM 接入信息（Issuer / Client ID / Secret / 应用 Key 等），然后：

1. 构建前端与后端（若已存在 `./archive-sync` 则直接使用）；
2. 安装到所选目录并写入 `config.yaml`（`0600`）；
3. 注册全局命令 `/usr/local/bin/archive-sync`（封装固定配置路径）；
4. 可选创建系统用户并安装 / 启用 `archive-sync.service`。

安装后请在 IAM 的 OIDC 客户端登记回调地址 `‹BaseURL›/api/auth/callback`。

---

## 🖥 全局命令

```bash
archive-sync serve                 # 启动服务（面板 + 调度器）
archive-sync status                # 查看渠道/通知/目标、下次运行、最近备份
archive-sync iam                   # 交互式重新配置 IAM 并保存
archive-sync config show           # 查看配置（敏感字段脱敏）
archive-sync config set <k> <v>    # 修改单个配置项（如 iam.client_id）
archive-sync backup <目标名或ID>    # 立即执行一次备份
archive-sync version
```

全局 `--config <path>` 或环境变量 `ARCHIVE_SYNC_CONFIG` 指定配置文件。

---

## ⚙️ 配置文件 `config.yaml`

```yaml
listen: ":8787"
data_dir: "/opt/archive-sync/data"       # SQLite 与临时打包目录
base_url: "https://backup.example.com"
master_key: "<自动生成的 base64 32 字节>"   # 敏感字段加密主密钥
session_ttl_hours: 24                     # 会话时长；到期后经 IAM SSO 重新登录
iam:
  issuer: "https://iam.transcircle.org"
  client_id: "..."          # 留空 => 开发模式（免登录）
  client_secret: "..."      # 公共客户端可留空
  redirect_url: "https://backup.example.com/api/auth/callback"
  app_key: "archive-sync"
  scopes: ["openid","profile","email","tc.permissions"]
  required_permission: ""   # 非空则要求用户在该应用下拥有此权限
  required_role: ""
```

`master_key` 在首次 `serve` 时自动生成并写回。渠道/通知的密钥字段用它加密后存入 SQLite。

---

## 🎯 备份目标：唯一标识与存储路径

每个目标都有一个**唯一标识（key）**：创建时由用户填写（字母 / 数字 / `._-`，1–64 位），**全局唯一、创建后不可修改**。它就是该目标在渠道中的存储目录名，因此重命名目标不会打乱已存历史。

归档对象按如下路径存放（日期与时间使用目标所在时区）：

```
<唯一标识>/<YYYY-MM-DD>/<HH-MM-SS>.tar.gz
例：nginx-conf/2026-07-04/03-00-05.tar.gz
```

保留策略与详情页的“按日期浏览”都基于此结构。

---

## 🗂 保留策略说明

面板提供三种引导式方案（**分层 / 按天数 / 按份数**）并实时预览。底层字段（按**策略时区**的日历日划分）：

| 字段 | 含义 |
|---|---|
| `keep_all_days` | 最近 N 个日历日内的**全部**备份都保留（1 = 保留“今天”全部） |
| `daily_anchors` | 形如 `["00:00","12:00"]`；对更早的每一天，保留最接近每个时刻的备份；留空则每天保留最新一份 |
| `keep_days` | 日快照总保留天数，超过即删除 |
| `max_versions` | 全局份数硬上限（0 = 不限；仅设它时表示“只保留最近 N 份”） |
| `min_keep` | 无论如何至少保留最近 N 份（默认 1） |

**示例（经典场景）**：每天备份 24 次，`时区=Asia/Shanghai`，`keep_all_days=1`，`daily_anchors=["00:00"]`，`keep_days=7`
→ 今天的 24 份全部保留；之前每天只保留最接近 00:00 的一份；超过 7 天的全部删除。
若希望“每天保留 12:00 与 24:00 各一份”，设 `daily_anchors=["12:00","24:00"]` 即可（`24:00` 自动规范化为午夜 `00:00`）。

---

## ☁️ 备份渠道示例

- **Cloudflare R2**：Endpoint `https://<account>.r2.cloudflarestorage.com`，Region `auto`，填入 R2 的 Access Key / Secret 与 Bucket。
- **AWS S3**：Endpoint 留空，填 Region（如 `ap-northeast-1`）。
- **MinIO**：Endpoint 指向 MinIO，勾选“强制 Path-Style”。
- **本地目录**：填写可写目录路径（可用“浏览”按钮选择）。

在**渠道详情页**可以文件夹形式浏览该渠道下所有目标的归档并直接下载。

---

## 🔔 通知渠道

- **Discord**：机器人（Bot Token + 群组 ID + 频道 ID，机器人需在该频道有发言权限）发送 **embed 卡片**（彩色状态侧边条、目标 / 状态 / 大小 / 耗时 / 逐渠道结果字段、`ArchiveSync` 页脚）。
- **Telegram**：Bot Token + Chat ID。
- **SMTP**：主机 / 端口 / 账号密码 / 发件人 / 收件人；端口 465 使用隐式 TLS，其余尝试 STARTTLS（不支持 STARTTLS 的远程服务器会被拒绝以避免明文发送，本地中继除外）。
- **Webhook**：URL + 方法 + 自定义 Header。**自动识别**目标地址：
  - Discord Incoming Webhook（`discord.com/api/webhooks/...`）→ 发送 `{"embeds":[卡片]}`；
  - Slack Incoming Webhook（`hooks.slack.com/...`）→ 发送 `{"text":"…"}`；
  - 其他端点 → 发送下述结构化 JSON。

Webhook 通用请求体：

```jsonc
{
  "type":      "success",           // start | success | failure
  "target":    "Nginx 配置",
  "title":     "备份成功 · Nginx 配置",
  "message":   "已上传至 1 个渠道（1.2 MiB，24 个文件）",
  "content":   "…完整多行文本（供 Discord content 使用）",
  "text":      "…同上（供 Slack text 使用）",
  "timestamp": "2026-07-04T12:00:00Z",
  "fields":    { "触发方式": "手动触发" },
  "run": {
    "status": "success", "size_bytes": 1258291,
    "file_count": 24, "duration_ms": 850,
    "destinations": [ { "channel_name": "R2-生产", "success": true, "pruned": 1 } ]
  }
}
```

每个通知可订阅 开始 / 成功 / 失败 事件；面板“通知详情”页可查看使用它的目标并发送测试。

---

## 🔌 REST API 概览

除 `/api/auth/*` 与 `/api/health` 外均需登录。

```
GET  /api/auth/login | callback     POST /api/auth/logout    GET /api/auth/me
GET|POST /api/channels              GET|PUT|DELETE /api/channels/{id}
POST /api/channels/test | /{id}/test
GET  /api/channels/{id}/objects?prefix=      # 文件夹式列举渠道对象
GET  /api/channels/{id}/download?key=        # 流式下载单个归档
GET|POST /api/notifiers             GET|PUT|DELETE /api/notifiers/{id}
POST /api/notifiers/test | /{id}/test
GET|POST /api/targets               GET|PUT|DELETE /api/targets/{id}
POST /api/targets/{id}/run
GET  /api/runs?target=&limit=        GET /api/runs/{id}
GET  /api/fs?path=                   # 服务器目录浏览（供目录选择器）
GET  /api/status                     GET /api/health
```

敏感字段在响应中被清空；更新时留空表示保留原值。目标的 `key` 创建后不可修改。

---

## 📝 许可

基于 [MIT 许可证](./LICENSE) 发布 · © 2026 xzdmycbx
