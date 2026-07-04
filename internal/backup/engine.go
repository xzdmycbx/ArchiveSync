// Package backup orchestrates a single backup run: it packs the source
// directory, uploads the archive to every destination channel, applies the
// retention policy per channel, records history, and fires notifications.
package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"archivesync/internal/archive"
	"archivesync/internal/config"
	"archivesync/internal/models"
	"archivesync/internal/notify"
	"archivesync/internal/retention"
	"archivesync/internal/storage"
	"archivesync/internal/store"

	"github.com/google/uuid"
)

// newID returns a fresh unique identifier for a run record.
func newID() string { return uuid.NewString() }

// ErrAlreadyRunning is returned when a target is already being backed up.
var ErrAlreadyRunning = errors.New("backup already in progress for this target")

// historyKeep bounds how many run records are retained per target.
const historyKeep = 200

// Engine executes backups. It is safe for concurrent use; a given target runs
// at most once at a time.
type Engine struct {
	store store.Store
	cfg   *config.Config
	log   *slog.Logger

	mu      sync.Mutex
	running map[string]struct{}
}

// NewEngine constructs a backup Engine.
func NewEngine(st store.Store, cfg *config.Config, log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}
	return &Engine{store: st, cfg: cfg, log: log, running: make(map[string]struct{})}
}

// IsRunning reports whether a target currently has a backup in progress.
func (e *Engine) IsRunning(targetID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.running[targetID]
	return ok
}

func (e *Engine) acquire(targetID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.running[targetID]; ok {
		return false
	}
	e.running[targetID] = struct{}{}
	return true
}

func (e *Engine) release(targetID string) {
	e.mu.Lock()
	delete(e.running, targetID)
	e.mu.Unlock()
}

// RunByName looks up a target by name (case-insensitive) or ID and runs it.
func (e *Engine) RunByName(ctx context.Context, nameOrID, trigger string) (*models.BackupRun, error) {
	targets, err := e.store.ListTargets(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		if t.ID == nameOrID || strings.EqualFold(t.Name, nameOrID) {
			return e.Run(ctx, t.ID, trigger)
		}
	}
	return nil, fmt.Errorf("target %q not found", nameOrID)
}

// Run executes a backup for the target identified by targetID. trigger is
// "schedule" or "manual". It records a BackupRun and returns it.
func (e *Engine) Run(ctx context.Context, targetID, trigger string) (*models.BackupRun, error) {
	target, err := e.store.GetTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if !e.acquire(target.ID) {
		return nil, ErrAlreadyRunning
	}
	defer e.release(target.ID)

	start := time.Now()
	run := &models.BackupRun{
		ID:         newID(),
		TargetID:   target.ID,
		TargetName: target.Name,
		Status:     models.RunRunning,
		Trigger:    trigger,
		StartedAt:  start.UTC(),
	}
	if err := e.store.CreateRun(ctx, run); err != nil {
		e.log.Error("create run record", "err", err)
	}

	dispatcher := e.dispatcherFor(ctx, target)
	dispatcher.Dispatch(ctx, notify.Event{
		Type:       models.EventStart,
		TargetName: target.Name,
		Title:      "开始备份 · " + target.Name,
		Message:    fmt.Sprintf("正在备份目录 %s", target.SourcePath),
		Run:        run,
		Timestamp:  time.Now(),
		Fields:     map[string]string{"源目录": target.SourcePath, "触发方式": triggerLabel(trigger)},
	})

	e.execute(ctx, target, run)

	fin := time.Now()
	run.FinishedAt = ptr(fin.UTC())
	run.DurationMs = fin.Sub(start).Milliseconds()
	if err := e.store.UpdateRun(ctx, run); err != nil {
		e.log.Error("update run record", "err", err)
	}
	if err := e.store.PruneRuns(ctx, target.ID, historyKeep); err != nil {
		e.log.Warn("prune run history", "err", err)
	}

	// Final notification.
	evType := models.EventSuccess
	title := "备份成功 · " + target.Name
	if run.Status == models.RunFailed {
		evType = models.EventFailure
		title = "备份失败 · " + target.Name
	} else if run.Status == models.RunPartial {
		evType = models.EventFailure
		title = "备份部分成功 · " + target.Name
	}
	dispatcher.Dispatch(ctx, notify.Event{
		Type:       evType,
		TargetName: target.Name,
		Title:      title,
		Message:    run.Message,
		Run:        run,
		Timestamp:  time.Now(),
		Fields:     map[string]string{"触发方式": triggerLabel(trigger)},
	})

	return run, nil
}

// execute performs the archive + upload + retention work, mutating run in place.
func (e *Engine) execute(ctx context.Context, target *models.Target, run *models.BackupRun) {
	// Validate source.
	info, err := os.Stat(target.SourcePath)
	if err != nil {
		run.Status = models.RunFailed
		run.Message = fmt.Sprintf("源目录不可访问: %v", err)
		return
	}
	if !info.IsDir() {
		run.Status = models.RunFailed
		run.Message = "源路径不是目录: " + target.SourcePath
		return
	}

	// Resolve the target's timezone; the storage path's date/time folders use it.
	loc := targetLocation(target)
	ts := time.Now()
	tsLocal := ts.In(loc)

	// dir is the immutable per-target namespace: the user-set Key (falling back
	// to the internal ID for legacy targets). Objects are stored as
	//   <dir>/<YYYY-MM-DD>/<HH-MM-SS>.<ext>
	// so each target is fully isolated and archives are grouped by day.
	dir := target.Key
	if dir == "" {
		dir = target.ID
	}
	ext := archive.Ext(target.Archive)

	tmpDir := e.cfg.TmpDir()
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		run.Status = models.RunFailed
		run.Message = "创建临时目录失败: " + err.Error()
		return
	}
	// Flat, unique temp filename for the local archive before upload.
	tmpBase := dir + "-" + ts.UTC().Format(retention.TimeLayout)
	res, err := archive.Create(ctx, target.SourcePath, tmpDir, tmpBase, target.Archive)
	if err != nil {
		run.Status = models.RunFailed
		run.Message = "打包失败: " + err.Error()
		return
	}
	defer os.Remove(res.Path)

	run.SizeBytes = res.Size
	run.FileCount = res.FileCount
	key := path.Join(dir, tsLocal.Format(retention.DirDateLayout), tsLocal.Format(retention.FileTimeLayout)+ext)
	run.ArchiveKey = key

	if len(target.ChannelIDs) == 0 {
		run.Status = models.RunFailed
		run.Message = "未配置任何备份渠道"
		return
	}

	// Upload to each destination.
	okCount := 0
	for _, chID := range target.ChannelIDs {
		dest := models.RunDestination{ChannelID: chID}
		ch, err := e.store.GetChannel(ctx, chID)
		if err != nil {
			dest.ChannelName = chID
			dest.Error = "渠道不存在: " + err.Error()
			run.Destinations = append(run.Destinations, dest)
			continue
		}
		dest.ChannelName = ch.Name
		dest.Key = key

		backend, err := storage.New(*ch)
		if err != nil {
			dest.Error = "初始化渠道失败: " + err.Error()
			run.Destinations = append(run.Destinations, dest)
			continue
		}

		if err := e.upload(ctx, backend, key, res.Path); err != nil {
			dest.Error = "上传失败: " + err.Error()
			run.Destinations = append(run.Destinations, dest)
			continue
		}
		dest.Success = true
		okCount++

		// Retention (best-effort; failures don't fail the upload).
		if pruned, err := e.applyRetention(ctx, backend, dir, loc, target.Retention); err != nil {
			e.log.Warn("retention failed", "channel", ch.Name, "target", target.Name, "err", err)
		} else {
			dest.Pruned = pruned
		}
		run.Destinations = append(run.Destinations, dest)
	}

	switch {
	case okCount == 0:
		run.Status = models.RunFailed
		run.Message = "所有渠道上传失败"
	case okCount < len(target.ChannelIDs):
		run.Status = models.RunPartial
		run.Message = fmt.Sprintf("%d/%d 个渠道上传成功", okCount, len(target.ChannelIDs))
	default:
		run.Status = models.RunSuccess
		run.Message = fmt.Sprintf("已上传至 %d 个渠道（%s，%d 个文件）",
			okCount, humanBytes(res.Size), res.FileCount)
	}
}

// upload streams the local archive file to the backend under key.
func (e *Engine) upload(ctx context.Context, backend storage.Backend, key, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	size := int64(-1)
	if st, err := f.Stat(); err == nil {
		size = st.Size()
	}
	return backend.Put(ctx, key, f, size)
}

// applyRetention lists existing archives under the target's directory prefix on
// the backend and deletes those the policy no longer keeps. Returns the count
// deleted. dir is the per-target key namespace (the immutable target ID).
func (e *Engine) applyRetention(ctx context.Context, backend storage.Backend, dir string, loc *time.Location, policy models.RetentionPolicy) (int, error) {
	if !retentionActive(policy) {
		return 0, nil
	}
	prefix := dir + "/"
	objs, err := backend.List(ctx, prefix)
	if err != nil {
		return 0, err
	}
	items := make([]retention.Item, 0, len(objs))
	for _, o := range objs {
		// Prefer the <date>/<time> path layout (parsed in the target tz), then
		// the legacy UTC token, finally the storage object's mtime.
		t, ok := retention.ParsePathTime(o.Key, loc)
		if !ok {
			if t, ok = retention.ParseTime(o.Key); !ok {
				t = o.LastModified
			}
		}
		items = append(items, retention.Item{Key: o.Key, Time: t})
	}
	_, del := retention.Plan(policy, items, time.Now())
	pruned := 0
	for _, k := range del {
		if err := backend.Delete(ctx, k); err != nil {
			e.log.Warn("delete old archive", "key", k, "err", err)
			continue
		}
		pruned++
	}
	return pruned, nil
}

// dispatcherFor loads the target's notifiers and builds a Dispatcher.
func (e *Engine) dispatcherFor(ctx context.Context, target *models.Target) *notify.Dispatcher {
	var ns []models.Notifier
	for _, id := range target.NotifierIDs {
		n, err := e.store.GetNotifier(ctx, id)
		if err != nil {
			e.log.Warn("notifier not found", "id", id, "err", err)
			continue
		}
		ns = append(ns, *n)
	}
	return notify.NewDispatcher(ns, e.log)
}

func retentionActive(p models.RetentionPolicy) bool {
	return p.KeepAllDays > 0 || len(p.DailyAnchors) > 0 || p.KeepDays > 0 || p.MaxVersions > 0
}

func triggerLabel(t string) string {
	if t == "manual" {
		return "手动触发"
	}
	return "定时调度"
}

// targetLocation resolves the target's timezone (retention wins over schedule),
// defaulting to UTC when unset or invalid.
func targetLocation(t *models.Target) *time.Location {
	name := t.Retention.Timezone
	if name == "" {
		name = t.Schedule.Timezone
	}
	if name == "" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	return time.UTC
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func ptr[T any](v T) *T { return &v }
