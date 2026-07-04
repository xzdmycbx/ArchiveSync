# ArchiveSync — 设计规格（实现契约）

> 本文件是所有实现模块的**共享契约**。任何实现代码必须严格遵守此处定义的包名、类型签名与接口。
> 修改契约前必须先更新本文件。

## 1. 总体

- **单可执行文件**：Go 后端 + 内嵌（`go:embed`）已构建的 Vue 前端。
- **模块名**：`archivesync`
- **Go 版本**：1.25（纯 Go，`CGO_ENABLED=0`；SQLite 使用 `modernc.org/sqlite`）
- **数据库**：SQLite（配置 / 历史 / 会话）。敏感配置 JSON 在写入前用主密钥 AES-256-GCM 加密。
- **前端**：Vue 3 `<script setup>` + Vite + Pinia + Vue Router + axios，自建轻量 CSS 设计系统（不依赖第三方 UI 库）。
- **认证**：TransCircle IAM，OIDC Authorization Code + PKCE(S256)，服务端会话（HttpOnly Cookie）。

## 2. 目录结构

```
cmd/archive-sync/main.go        // 入口，调用 internal/cli
internal/
  version/      // 版本常量
  models/       // 领域类型（契约核心，仅依赖标准库）
  config/       // bootstrap 配置(YAML) 加载/保存
  crypto/       // AES-GCM 字段加密
  store/        // Store 接口 + sqlite 实现
  storage/      // Backend 接口 + s3 + local 实现 + 注册表
  notify/       // Notifier 接口 + discord/telegram/smtp/webhook + 分发器
  archive/      // 目录打包 (tar.gz / zip)
  retention/    // 保留策略引擎
  backup/       // 备份执行引擎（编排 archive+storage+retention+notify+store）
  scheduler/    // 基于 cron 的调度
  auth/         // OIDC/IAM 登录 + 会话中间件
  api/          // HTTP 路由与处理器 + 前端静态资源服务
  cli/          // cobra 命令
web/            // Vue 前端
install.sh
archive-sync.service
README.md
```

## 3. 领域模型（`internal/models`，仅标准库）

见 `internal/models/models.go`。关键类型：

- `Channel`（存储渠道）：`Type` ∈ {`s3`,`local`}，`Config ChannelConfig`
- `Notifier`（通知渠道）：`Type` ∈ {`discord`,`telegram`,`smtp`,`webhook`}，`Config NotifierConfig`，`Events []string`
- `Target`（备份目录/目标）：`SourcePath`、`Schedule`、`Retention`、`ChannelIDs []string`、`NotifierIDs []string`、`Archive`
- `Schedule`：`Mode` ∈ {`cron`,`times`,`interval`}
- `RetentionPolicy`：分层保留
- `BackupRun` / `RunDestination`：历史记录

## 4. 接口契约

### 4.1 `storage.Backend`（`internal/storage`）
```go
type Object struct { Key string; Size int64; LastModified time.Time }
type Backend interface {
    Put(ctx context.Context, key string, r io.Reader, size int64) error
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    List(ctx context.Context, prefix string) ([]Object, error)
    Delete(ctx context.Context, key string) error
    Ping(ctx context.Context) error
    Kind() string
}
// New 按渠道配置构造后端；由各实现在 init() 注册到内部表。
func New(ch models.Channel) (Backend, error)
```
- `s3`：使用 `aws-sdk-go-v2`，支持自定义 `Endpoint`（R2/MinIO）、`Region`、`ForcePathStyle`、静态凭证。`key` 会自动拼接 `Config.Prefix`。
- `local`：写入 `Config.BasePath` 下。

### 4.2 `notify.Notifier`（`internal/notify`）
```go
type Event struct {
    Type       string            // "start" | "success" | "failure"
    TargetName string
    Title      string
    Message    string
    Run        *models.BackupRun
    Timestamp  time.Time
    Fields     map[string]string
}
type Notifier interface {
    Send(ctx context.Context, ev Event) error
    Kind() string
}
func New(n models.Notifier) (Notifier, error)   // 工厂
// Dispatcher：按 target 关联的 notifier 列表 + 事件类型过滤后并发发送
```

### 4.3 `store.Store`（`internal/store`）
CRUD：Channels / Notifiers / Targets（各有 List/Get/Create/Update/Delete）；
Runs：`CreateRun/UpdateRun/GetRun/ListRuns(targetID string, limit int)/RecentRuns(limit int)/PruneRuns(targetID string, keepIDs []string)`；
Settings：`GetSetting/SetSetting/AllSettings`；
Sessions：`CreateSession/GetSession/DeleteSession/PruneExpiredSessions`。
构造：`func Open(path string, cipher *crypto.Cipher) (Store, error)`。

### 4.4 `retention` 引擎
```go
// Plan 根据策略计算：在给定对象集合中，哪些 key 应删除。
// objects 已含从 key 解析出的时间（用 ParseTime）。
func Plan(policy models.RetentionPolicy, objects []retention.Item, now time.Time) (keep []string, delete []string)
```
对象 key 命名规范（打包与保留共用）：
`<prefix>/<targetSlug>/<targetSlug>-<UTCyyyyMMddTHHmmssZ>.tar.gz`
时间戳始终为 UTC，格式 `20060102T150405Z`。

### 4.5 保留策略语义（`RetentionPolicy`）
- `Timezone`：策略时区（如 `Asia/Shanghai`）。所有“日”按此时区的日历日划分。
- `KeepAllDays`：最近 N 个日历日内的**所有**备份都保留（N=1 表示保留“今天”全部）。
- `DailyAnchors []string`：形如 `["00:00","12:00"]`。对于比 KeepAllDays 更旧、但在 KeepDays 内的每个日历日，保留最接近每个锚点时刻的那个备份。
- `KeepDays`：日快照总保留天数；更旧的删除。
- `MaxVersions`：全局硬上限（0=不限）。超过时优先删最旧。
- `MinKeep`：无论如何至少保留最近 N 个（默认 1，避免误删到 0）。

### 4.6 认证（`internal/auth`）
- `Config`：`Issuer, ClientID, ClientSecret, RedirectURL, Scopes, AppKey, RequiredPermission, RequiredRole`。
- 流程：`/api/auth/login` → 生成 state+nonce+PKCE，302 到 IAM；`/api/auth/callback` → 换 token、校验 id_token(JWKS)、拉 userinfo（含 `tc_permissions/tc_groups`）、建会话、写 Cookie，302 回前端。
- 中间件 `RequireAuth`：校验会话 Cookie；可选校验 `RequiredPermission`/`RequiredRole`。
- `/api/auth/me` 返回当前用户；`/api/auth/logout` 注销。

## 5. REST API（`/api`，除 auth 外均需登录）
- `GET  /api/auth/login` / `GET /api/auth/callback` / `POST /api/auth/logout` / `GET /api/auth/me`
- `GET/POST /api/channels`，`GET/PUT/DELETE /api/channels/{id}`，`POST /api/channels/{id}/test`
- `GET/POST /api/notifiers`，`GET/PUT/DELETE /api/notifiers/{id}`，`POST /api/notifiers/{id}/test`
- `GET/POST /api/targets`，`GET/PUT/DELETE /api/targets/{id}`，`POST /api/targets/{id}/run`
- `GET /api/runs?target=&limit=`，`GET /api/runs/{id}`
- `GET /api/status`（服务状态、各 target 下次运行时间、最近结果、统计）
- `GET /api/settings` / `PUT /api/settings`（保留/清理策略等全局项，可选）
- 统一响应：成功返回资源 JSON；错误 `{"error":{"code":"...","message":"..."}}` + 对应 HTTP 码。
- 写敏感字段：返回时对 secret 字段做掩码（如 `***`），保存时若为掩码值则保留原值。

## 6. CLI（`archive-sync`，cobra）
- `archive-sync serve`：启动服务（读取 bootstrap 配置）。
- `archive-sync status`：连接本地/服务状态，打印 targets、最近备份、下次调度。
- `archive-sync iam`：交互式重新配置 IAM（写入 bootstrap 配置）。
- `archive-sync config [show|set k v]`：查看/修改 bootstrap 配置。
- `archive-sync backup <targetNameOrID>`：立即执行一次备份。
- `archive-sync version`。
- 全局 `--config` 指定 bootstrap 配置路径（默认：`$ARCHIVE_SYNC_CONFIG` 或安装时写入路径或 `./config.yaml`）。

## 7. bootstrap 配置（`internal/config`，YAML）
```yaml
listen: ":8787"
data_dir: "./data"          # sqlite、临时打包目录
base_url: "http://localhost:8787"
master_key: "<base64 32B>"   # 字段加密主密钥（安装脚本生成）
iam:
  issuer: "https://iam.transcircle.org"
  client_id: ""
  client_secret: ""
  redirect_url: "http://localhost:8787/api/auth/callback"
  app_key: "archive-sync"
  scopes: ["openid","profile","email","tc.permissions"]
  required_permission: ""    # 为空则任意登录用户可用
  required_role: ""
session_ttl_hours: 168
```

## 8. 通知内容
事件文本包含：目标名、标题、状态、开始/结束时间、耗时、归档大小/文件数、目的渠道逐个结果、错误信息（失败时）。各 notifier 自行渲染为其平台格式（Discord embed / TG Markdown / SMTP HTML / Webhook JSON）。
