// Package models holds ArchiveSync's domain types. It is the shared contract
// consumed by every other package and depends only on the standard library.
package models

import "time"

// ---------------------------------------------------------------------------
// Channel — a storage destination ("backup channel").
// ---------------------------------------------------------------------------

type ChannelType string

const (
	ChannelS3    ChannelType = "s3"
	ChannelLocal ChannelType = "local"
)

// Channel is a configured storage destination.
type Channel struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Type      ChannelType   `json:"type"`
	Config    ChannelConfig `json:"config"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// ChannelConfig is a superset of every backend's settings. Only the fields
// relevant to Channel.Type are meaningful.
type ChannelConfig struct {
	// S3 / S3-compatible (AWS, Cloudflare R2, MinIO, ...)
	Endpoint        string `json:"endpoint,omitempty"` // empty => AWS default
	Region          string `json:"region,omitempty"`   // R2 uses "auto"
	Bucket          string `json:"bucket,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	Prefix          string `json:"prefix,omitempty"`           // key prefix inside bucket
	ForcePathStyle  bool   `json:"force_path_style,omitempty"` // MinIO usually needs true

	// Local filesystem
	BasePath string `json:"base_path,omitempty"`
}

// SecretFields returns the config keys that must be masked in API responses.
func (ChannelConfig) SecretFields() []string { return []string{"secret_access_key"} }

// ---------------------------------------------------------------------------
// Notifier — a notification destination.
// ---------------------------------------------------------------------------

type NotifierType string

const (
	NotifierDiscord  NotifierType = "discord"
	NotifierTelegram NotifierType = "telegram"
	NotifierSMTP     NotifierType = "smtp"
	NotifierWebhook  NotifierType = "webhook"
)

// Event kinds a notifier may subscribe to.
const (
	EventStart   = "start"
	EventSuccess = "success"
	EventFailure = "failure"
)

// Notifier is a configured notification destination.
type Notifier struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Type      NotifierType   `json:"type"`
	Enabled   bool           `json:"enabled"`
	Events    []string       `json:"events"` // subset of {start,success,failure}
	Config    NotifierConfig `json:"config"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// NotifierConfig is a superset of every notifier's settings.
type NotifierConfig struct {
	// Discord bot
	BotToken  string `json:"bot_token,omitempty"`
	GuildID   string `json:"guild_id,omitempty"`   // 群组ID
	ChannelID string `json:"channel_id,omitempty"` // 频道ID

	// Telegram bot
	TGBotToken string `json:"tg_bot_token,omitempty"`
	TGChatID   string `json:"tg_chat_id,omitempty"`

	// SMTP
	SMTPHost string   `json:"smtp_host,omitempty"`
	SMTPPort int      `json:"smtp_port,omitempty"`
	SMTPUser string   `json:"smtp_user,omitempty"`
	SMTPPass string   `json:"smtp_pass,omitempty"`
	SMTPFrom string   `json:"smtp_from,omitempty"`
	SMTPTo   []string `json:"smtp_to,omitempty"`
	SMTPTLS  bool     `json:"smtp_tls,omitempty"` // implicit TLS (465); otherwise STARTTLS

	// Webhook
	WebhookURL     string            `json:"webhook_url,omitempty"`
	WebhookMethod  string            `json:"webhook_method,omitempty"` // default POST
	WebhookHeaders map[string]string `json:"webhook_headers,omitempty"`
}

// SecretFields returns the config keys that must be masked in API responses.
func (NotifierConfig) SecretFields() []string {
	return []string{"bot_token", "tg_bot_token", "smtp_pass"}
}

// ---------------------------------------------------------------------------
// Target — a backup source directory and its policy.
// ---------------------------------------------------------------------------

// Target is a directory to back up together with schedule, destinations,
// retention and notification settings.
type Target struct {
	ID string `json:"id"` // internal UUID (DB primary key, API routes)
	// Key is a user-chosen unique identifier, immutable once created. It is the
	// top-level storage directory for this target's archives:
	//   <key>/<YYYY-MM-DD>/<HH-MM-SS>.tar.gz
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	SourcePath  string          `json:"source_path"`
	Enabled     bool            `json:"enabled"`
	Schedule    Schedule        `json:"schedule"`
	Retention   RetentionPolicy `json:"retention"`
	ChannelIDs  []string        `json:"channel_ids"`  // one or more destinations
	NotifierIDs []string        `json:"notifier_ids"` // zero or more notifiers
	Archive     ArchiveOptions  `json:"archive"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Schedule describes when a target runs.
type Schedule struct {
	Mode        string   `json:"mode"`          // "cron" | "times" | "interval"
	Cron        string   `json:"cron"`          // when Mode=="cron" (5 or 6 field)
	Times       []string `json:"times"`         // when Mode=="times": explicit "HH:MM" list
	TimesPerDay int      `json:"times_per_day"` // when Mode=="times" and Times empty: evenly spaced
	IntervalMin int      `json:"interval_min"`  // when Mode=="interval": minutes between runs
	Timezone    string   `json:"timezone"`      // IANA tz, e.g. "Asia/Shanghai"
}

// RetentionPolicy is a tiered retention rule. See SPEC §4.5.
type RetentionPolicy struct {
	Timezone     string   `json:"timezone"`      // policy timezone (day boundaries)
	KeepAllDays  int      `json:"keep_all_days"` // keep EVERY backup within the last N calendar days
	DailyAnchors []string `json:"daily_anchors"` // "HH:MM" anchors kept for older days
	KeepDays     int      `json:"keep_days"`     // total days of daily snapshots to keep
	MaxVersions  int      `json:"max_versions"`  // hard cap on total kept (0 = unlimited)
	MinKeep      int      `json:"min_keep"`      // always keep at least N most-recent (default 1)
}

// ArchiveOptions controls how the source directory is packed.
type ArchiveOptions struct {
	Format      string   `json:"format"`      // "tar.gz" | "zip" (default "tar.gz")
	Compression int      `json:"compression"` // gzip level 0-9 (default 6)
	Include     []string `json:"include"`     // glob patterns (empty => all)
	Exclude     []string `json:"exclude"`     // glob patterns
}

// ---------------------------------------------------------------------------
// BackupRun — a history record of one backup execution.
// ---------------------------------------------------------------------------

type RunStatus string

const (
	RunPending RunStatus = "pending"
	RunRunning RunStatus = "running"
	RunSuccess RunStatus = "success"
	RunPartial RunStatus = "partial" // succeeded to some destinations, failed others
	RunFailed  RunStatus = "failed"
)

// BackupRun records a single execution of a target's backup.
type BackupRun struct {
	ID           string           `json:"id"`
	TargetID     string           `json:"target_id"`
	TargetName   string           `json:"target_name"`
	Status       RunStatus        `json:"status"`
	Trigger      string           `json:"trigger"` // "schedule" | "manual"
	StartedAt    time.Time        `json:"started_at"`
	FinishedAt   *time.Time       `json:"finished_at,omitempty"`
	DurationMs   int64            `json:"duration_ms"`
	ArchiveKey   string           `json:"archive_key"`
	SizeBytes    int64            `json:"size_bytes"`
	FileCount    int              `json:"file_count"`
	Message      string           `json:"message"`
	Destinations []RunDestination `json:"destinations"`
}

// RunDestination is the per-channel result of a run.
type RunDestination struct {
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Key         string `json:"key"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
	Pruned      int    `json:"pruned"` // number of old objects deleted by retention
}

// ---------------------------------------------------------------------------
// Session — an authenticated admin session.
// ---------------------------------------------------------------------------

// Session is a server-side authenticated session created after IAM login.
type Session struct {
	ID          string    `json:"id"` // opaque token stored in cookie
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Picture     string    `json:"picture"`
	Roles       []string  `json:"roles"`
	Permissions []string  `json:"permissions"`
	Groups      []string  `json:"groups"`
	PermVersion string    `json:"perm_version"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// HasPermission reports whether the session grants perm (empty perm => true).
func (s *Session) HasPermission(perm string) bool {
	if perm == "" {
		return true
	}
	for _, p := range s.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// HasRole reports whether the session has role (empty role => true).
func (s *Session) HasRole(role string) bool {
	if role == "" {
		return true
	}
	for _, r := range s.Roles {
		if r == role {
			return true
		}
	}
	return false
}
