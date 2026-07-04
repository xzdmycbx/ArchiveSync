package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"archivesync/internal/models"
	"archivesync/internal/notify"
	"archivesync/internal/scheduler"
	"archivesync/internal/storage"
	"archivesync/internal/store"
	"archivesync/internal/version"
)

// targetKeyRe validates a user-set target key (the immutable storage directory).
var targetKeyRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ---------------------------------------------------------------------------
// Secret masking / merging.
// ---------------------------------------------------------------------------

const secretMask = "" // secrets are blanked on read; blank on write => keep existing

func maskChannel(c *models.Channel) {
	c.Config.SecretAccessKey = secretMask
}

func mergeChannelSecrets(next *models.Channel, old *models.Channel) {
	if next.Config.SecretAccessKey == "" {
		next.Config.SecretAccessKey = old.Config.SecretAccessKey
	}
}

func maskNotifier(n *models.Notifier) {
	n.Config.BotToken = secretMask
	n.Config.TGBotToken = secretMask
	n.Config.SMTPPass = secretMask
	// Webhook headers commonly carry Authorization/bearer tokens; blank them.
	n.Config.WebhookHeaders = nil
}

func mergeNotifierSecrets(next *models.Notifier, old *models.Notifier) {
	if next.Config.BotToken == "" {
		next.Config.BotToken = old.Config.BotToken
	}
	if next.Config.TGBotToken == "" {
		next.Config.TGBotToken = old.Config.TGBotToken
	}
	if next.Config.SMTPPass == "" {
		next.Config.SMTPPass = old.Config.SMTPPass
	}
	if len(next.Config.WebhookHeaders) == 0 {
		next.Config.WebhookHeaders = old.Config.WebhookHeaders
	}
}

func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }

// ---------------------------------------------------------------------------
// Channels.
// ---------------------------------------------------------------------------

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListChannels(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	for i := range items {
		maskChannel(&items[i])
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getChannel(w http.ResponseWriter, r *http.Request) {
	ch, err := s.store.GetChannel(r.Context(), idParam(r))
	if err != nil {
		s.notFoundOr500(w, err)
		return
	}
	maskChannel(ch)
	writeJSON(w, http.StatusOK, ch)
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	var ch models.Channel
	if err := decode(r, &ch); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return
	}
	ch.ID = ""
	if msg := validateChannel(&ch); msg != "" {
		writeErr(w, http.StatusBadRequest, "validation", msg)
		return
	}
	if err := s.store.CreateChannel(r.Context(), &ch); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	maskChannel(&ch)
	writeJSON(w, http.StatusCreated, ch)
}

func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request) {
	old, err := s.store.GetChannel(r.Context(), idParam(r))
	if err != nil {
		s.notFoundOr500(w, err)
		return
	}
	var ch models.Channel
	if err := decode(r, &ch); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return
	}
	ch.ID = old.ID
	ch.CreatedAt = old.CreatedAt
	mergeChannelSecrets(&ch, old)
	if msg := validateChannel(&ch); msg != "" {
		writeErr(w, http.StatusBadRequest, "validation", msg)
		return
	}
	if err := s.store.UpdateChannel(r.Context(), &ch); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	maskChannel(&ch)
	writeJSON(w, http.StatusOK, ch)
}

func (s *Server) deleteChannel(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteChannel(r.Context(), idParam(r)); err != nil {
		s.notFoundOr500(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testChannelByID(w http.ResponseWriter, r *http.Request) {
	ch, err := s.store.GetChannel(r.Context(), idParam(r))
	if err != nil {
		s.notFoundOr500(w, err)
		return
	}
	s.pingChannel(w, r, ch)
}

func (s *Server) testChannelBody(w http.ResponseWriter, r *http.Request) {
	var ch models.Channel
	if err := decode(r, &ch); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return
	}
	// If editing an existing channel with a blank secret, merge the stored one.
	if ch.ID != "" {
		if old, err := s.store.GetChannel(r.Context(), ch.ID); err == nil {
			mergeChannelSecrets(&ch, old)
		}
	}
	if msg := validateChannel(&ch); msg != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": msg})
		return
	}
	s.pingChannel(w, r, &ch)
}

func (s *Server) pingChannel(w http.ResponseWriter, r *http.Request, ch *models.Channel) {
	backend, err := storage.New(*ch)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := backend.Ping(ctx); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func validateChannel(ch *models.Channel) string {
	if ch.Name == "" {
		return "名称不能为空"
	}
	switch ch.Type {
	case models.ChannelS3:
		if ch.Config.Bucket == "" {
			return "S3 渠道需要 Bucket"
		}
		if ch.Config.AccessKeyID == "" || ch.Config.SecretAccessKey == "" {
			return "S3 渠道需要 Access Key 与 Secret Key"
		}
	case models.ChannelLocal:
		if ch.Config.BasePath == "" {
			return "本地渠道需要目录路径"
		}
	default:
		return "未知的渠道类型: " + string(ch.Type)
	}
	return ""
}

// ---------------------------------------------------------------------------
// Notifiers.
// ---------------------------------------------------------------------------

func (s *Server) listNotifiers(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListNotifiers(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	for i := range items {
		maskNotifier(&items[i])
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getNotifier(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.GetNotifier(r.Context(), idParam(r))
	if err != nil {
		s.notFoundOr500(w, err)
		return
	}
	maskNotifier(n)
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) createNotifier(w http.ResponseWriter, r *http.Request) {
	var n models.Notifier
	if err := decode(r, &n); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return
	}
	n.ID = ""
	if msg := validateNotifier(&n); msg != "" {
		writeErr(w, http.StatusBadRequest, "validation", msg)
		return
	}
	if err := s.store.CreateNotifier(r.Context(), &n); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	maskNotifier(&n)
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) updateNotifier(w http.ResponseWriter, r *http.Request) {
	old, err := s.store.GetNotifier(r.Context(), idParam(r))
	if err != nil {
		s.notFoundOr500(w, err)
		return
	}
	var n models.Notifier
	if err := decode(r, &n); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return
	}
	n.ID = old.ID
	n.CreatedAt = old.CreatedAt
	mergeNotifierSecrets(&n, old)
	if msg := validateNotifier(&n); msg != "" {
		writeErr(w, http.StatusBadRequest, "validation", msg)
		return
	}
	if err := s.store.UpdateNotifier(r.Context(), &n); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	maskNotifier(&n)
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) deleteNotifier(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteNotifier(r.Context(), idParam(r)); err != nil {
		s.notFoundOr500(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testNotifierByID(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.GetNotifier(r.Context(), idParam(r))
	if err != nil {
		s.notFoundOr500(w, err)
		return
	}
	s.sendTestNotification(w, r, n)
}

func (s *Server) testNotifierBody(w http.ResponseWriter, r *http.Request) {
	var n models.Notifier
	if err := decode(r, &n); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return
	}
	if n.ID != "" {
		if old, err := s.store.GetNotifier(r.Context(), n.ID); err == nil {
			mergeNotifierSecrets(&n, old)
		}
	}
	if msg := validateNotifier(&n); msg != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": msg})
		return
	}
	s.sendTestNotification(w, r, &n)
}

func (s *Server) sendTestNotification(w http.ResponseWriter, r *http.Request, n *models.Notifier) {
	notifier, err := notify.New(*n)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	ev := notify.Event{
		Type:       models.EventSuccess,
		TargetName: "测试",
		Title:      "ArchiveSync 测试通知",
		Message:    "这是一条来自 ArchiveSync 的测试通知，如果你收到它，说明该通知渠道配置正确。",
		Timestamp:  time.Now(),
		Fields:     map[string]string{"渠道": n.Name, "类型": string(n.Type)},
	}
	if err := notifier.Send(ctx, ev); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func validateNotifier(n *models.Notifier) string {
	if n.Name == "" {
		return "名称不能为空"
	}
	switch n.Type {
	case models.NotifierDiscord:
		if n.Config.BotToken == "" || n.Config.ChannelID == "" {
			return "Discord 需要 Bot Token 与频道 ID"
		}
	case models.NotifierTelegram:
		if n.Config.TGBotToken == "" || n.Config.TGChatID == "" {
			return "Telegram 需要 Bot Token 与 Chat ID"
		}
	case models.NotifierSMTP:
		if n.Config.SMTPHost == "" || n.Config.SMTPPort == 0 || n.Config.SMTPFrom == "" || len(n.Config.SMTPTo) == 0 {
			return "SMTP 需要 Host / Port / 发件人 / 收件人"
		}
	case models.NotifierWebhook:
		if n.Config.WebhookURL == "" {
			return "Webhook 需要 URL"
		}
	default:
		return "未知的通知类型: " + string(n.Type)
	}
	return ""
}

// ---------------------------------------------------------------------------
// Targets.
// ---------------------------------------------------------------------------

func (s *Server) listTargets(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListTargets(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getTarget(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.GetTarget(r.Context(), idParam(r))
	if err != nil {
		s.notFoundOr500(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) createTarget(w http.ResponseWriter, r *http.Request) {
	var t models.Target
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return
	}
	t.ID = ""
	t.Key = strings.TrimSpace(t.Key)
	if msg := validateTarget(&t); msg != "" {
		writeErr(w, http.StatusBadRequest, "validation", msg)
		return
	}
	if t.Key == "" {
		writeErr(w, http.StatusBadRequest, "validation", "唯一标识不能为空")
		return
	}
	if !targetKeyRe.MatchString(t.Key) {
		writeErr(w, http.StatusBadRequest, "validation", "唯一标识只能包含字母、数字、下划线、连字符与点，长度 1-64")
		return
	}
	if t.Key == "." || t.Key == ".." {
		// These would collapse/escape the storage directory the key names.
		writeErr(w, http.StatusBadRequest, "validation", "唯一标识不能为 . 或 ..")
		return
	}
	existing, _ := s.store.ListTargets(r.Context())
	for _, e := range existing {
		if strings.EqualFold(e.Key, t.Key) {
			writeErr(w, http.StatusBadRequest, "validation", "唯一标识「"+t.Key+"」已被占用，请更换")
			return
		}
	}
	if err := s.store.CreateTarget(r.Context(), &t); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.reloadScheduler()
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) updateTarget(w http.ResponseWriter, r *http.Request) {
	old, err := s.store.GetTarget(r.Context(), idParam(r))
	if err != nil {
		s.notFoundOr500(w, err)
		return
	}
	var t models.Target
	if err := decode(r, &t); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "无效的请求体")
		return
	}
	t.ID = old.ID
	t.CreatedAt = old.CreatedAt
	t.Key = old.Key // immutable: the storage directory never changes after creation
	if msg := validateTarget(&t); msg != "" {
		writeErr(w, http.StatusBadRequest, "validation", msg)
		return
	}
	if err := s.store.UpdateTarget(r.Context(), &t); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.reloadScheduler()
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) deleteTarget(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteTarget(r.Context(), idParam(r)); err != nil {
		s.notFoundOr500(w, err)
		return
	}
	s.reloadScheduler()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) runTarget(w http.ResponseWriter, r *http.Request) {
	id := idParam(r)
	if _, err := s.store.GetTarget(r.Context(), id); err != nil {
		s.notFoundOr500(w, err)
		return
	}
	if s.engine.IsRunning(id) {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "该目标正在备份中"})
		return
	}
	go func() {
		if _, err := s.engine.Run(context.Background(), id, "manual"); err != nil {
			s.log.Error("manual backup failed", "target", id, "err", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": "备份已触发"})
}

func validateTarget(t *models.Target) string {
	if t.Name == "" {
		return "名称不能为空"
	}
	if t.SourcePath == "" {
		return "源目录不能为空"
	}
	if len(t.ChannelIDs) == 0 {
		return "至少选择一个备份渠道"
	}
	switch t.Schedule.Mode {
	case "cron":
		if t.Schedule.Cron == "" {
			return "Cron 模式需要填写表达式"
		}
	case "times":
		if len(t.Schedule.Times) == 0 && t.Schedule.TimesPerDay <= 0 {
			return "定时模式需要填写时间点或每日次数"
		}
	case "interval":
		if t.Schedule.IntervalMin <= 0 {
			return "间隔模式需要填写分钟数"
		}
	default:
		return "未知的调度模式"
	}
	// Deep-validate using the same parser the scheduler uses, so an invalid
	// cron / time / timezone is rejected at save time rather than silently
	// dropped by the scheduler (target would never run).
	if err := scheduler.ValidateSchedule(t.Schedule); err != nil {
		return "无效的调度配置: " + err.Error()
	}
	return ""
}

// ---------------------------------------------------------------------------
// Runs.
// ---------------------------------------------------------------------------

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	targetID := r.URL.Query().Get("target")
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	runs, err := s.store.ListRuns(r.Context(), targetID, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.GetRun(r.Context(), idParam(r))
	if err != nil {
		s.notFoundOr500(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// ---------------------------------------------------------------------------
// Status.
// ---------------------------------------------------------------------------

type statusStats struct {
	Targets        int        `json:"targets"`
	EnabledTargets int        `json:"enabled_targets"`
	Channels       int        `json:"channels"`
	Notifiers      int        `json:"notifiers"`
	RunsTotal      int        `json:"runs_total"`
	Success        int        `json:"success"`
	Failed         int        `json:"failed"`
	SuccessRate    float64    `json:"success_rate"`
	LastBackupAt   *time.Time `json:"last_backup_at,omitempty"`
	TotalSize      int64      `json:"total_size"`
}

type targetStatus struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Enabled    bool                   `json:"enabled"`
	SourcePath string                 `json:"source_path"`
	Schedule   models.Schedule        `json:"schedule"`
	Retention  models.RetentionPolicy `json:"retention"`
	NextRun    *time.Time             `json:"next_run,omitempty"`
	LastRun    *models.BackupRun      `json:"last_run,omitempty"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targets, err := s.store.ListTargets(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	channels, _ := s.store.ListChannels(ctx)
	notifiers, _ := s.store.ListNotifiers(ctx)
	window, _ := s.store.ListRuns(ctx, "", 500)

	stats := statusStats{
		Targets:   len(targets),
		Channels:  len(channels),
		Notifiers: len(notifiers),
		RunsTotal: len(window),
	}
	for _, t := range targets {
		if t.Enabled {
			stats.EnabledTargets++
		}
	}
	for i := range window {
		run := window[i]
		switch run.Status {
		case models.RunSuccess, models.RunPartial:
			stats.Success++
			stats.TotalSize += run.SizeBytes
			if stats.LastBackupAt == nil || run.StartedAt.After(*stats.LastBackupAt) {
				t := run.StartedAt
				stats.LastBackupAt = &t
			}
		case models.RunFailed:
			stats.Failed++
		}
	}
	if done := stats.Success + stats.Failed; done > 0 {
		stats.SuccessRate = float64(stats.Success) / float64(done)
	}

	tstatus := make([]targetStatus, 0, len(targets))
	for _, t := range targets {
		ts := targetStatus{
			ID:         t.ID,
			Name:       t.Name,
			Enabled:    t.Enabled,
			SourcePath: t.SourcePath,
			Schedule:   t.Schedule,
			Retention:  t.Retention,
		}
		if last, err := s.store.ListRuns(ctx, t.ID, 1); err == nil && len(last) > 0 {
			ts.LastRun = &last[0]
		}
		if s.sched != nil {
			if nr := s.sched.NextRun(t.ID); !nr.IsZero() {
				ts.NextRun = &nr
			}
		}
		tstatus = append(tstatus, ts)
	}

	recent := window
	if len(recent) > 10 {
		recent = recent[:10]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":    version.String(),
		"started_at": s.startedAt,
		"now":        time.Now(),
		"dev":        s.DevMode(),
		"stats":      stats,
		"targets":    tstatus,
		"recent":     recent,
	})
}

// ---------------------------------------------------------------------------
// Misc.
// ---------------------------------------------------------------------------

func (s *Server) reloadScheduler() {
	if s.sched == nil {
		return
	}
	if err := s.sched.Reload(context.Background()); err != nil {
		s.log.Warn("scheduler reload failed", "err", err)
	}
}

func (s *Server) notFoundOr500(w http.ResponseWriter, err error) {
	if isNotFound(err) {
		writeErr(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	writeErr(w, http.StatusInternalServerError, "internal", err.Error())
}
