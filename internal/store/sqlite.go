package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"archivesync/internal/crypto"
	"archivesync/internal/models"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// timeLayout is a fixed-width RFC3339 layout (always UTC, nanosecond padding)
// so that timestamps stored as TEXT sort correctly lexicographically.
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// scanner is implemented by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// sqliteStore is a Store backed by an SQLite database (modernc.org/sqlite,
// pure Go, no CGO). Secret config JSON is encrypted at rest via cipher.
type sqliteStore struct {
	db     *sql.DB
	cipher *crypto.Cipher
}

// Open opens (creating if needed) the SQLite database at path and returns a
// Store. cipher may be nil, in which case config blobs are stored as plaintext.
func Open(path string, cipher *crypto.Cipher) (Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("store: create data dir %q: %w", dir, err)
		}
	}

	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}
	// A single connection sidesteps "database is locked" with modernc/WAL and
	// serializes all access, satisfying the Store concurrency contract.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping sqlite: %w", err)
	}

	s := &sqliteStore{db: db, cipher: cipher}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Best-effort: the DB holds encrypted secrets and plaintext session tokens,
	// so restrict it (and its WAL/SHM sidecars) to the owner.
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		_ = os.Chmod(p, 0o600)
	}
	return s, nil
}

// migrate runs idempotent schema creation.
func (s *sqliteStore) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS channels (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			type       TEXT NOT NULL,
			config     TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS notifiers (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			type       TEXT NOT NULL,
			enabled    INTEGER NOT NULL DEFAULT 0,
			events     TEXT NOT NULL DEFAULT 'null',
			config     TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS targets (
			id           TEXT PRIMARY KEY,
			"key"        TEXT NOT NULL DEFAULT '',
			name         TEXT NOT NULL,
			source_path  TEXT NOT NULL,
			enabled      INTEGER NOT NULL DEFAULT 0,
			schedule     TEXT NOT NULL DEFAULT 'null',
			retention    TEXT NOT NULL DEFAULT 'null',
			channel_ids  TEXT NOT NULL DEFAULT 'null',
			notifier_ids TEXT NOT NULL DEFAULT 'null',
			archive      TEXT NOT NULL DEFAULT 'null',
			created_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS runs (
			id           TEXT PRIMARY KEY,
			target_id    TEXT NOT NULL,
			target_name  TEXT NOT NULL DEFAULT '',
			status       TEXT NOT NULL DEFAULT '',
			"trigger"    TEXT NOT NULL DEFAULT '',
			started_at   TEXT NOT NULL,
			finished_at  TEXT,
			duration_ms  INTEGER NOT NULL DEFAULT 0,
			archive_key  TEXT NOT NULL DEFAULT '',
			size_bytes   INTEGER NOT NULL DEFAULT 0,
			file_count   INTEGER NOT NULL DEFAULT 0,
			message      TEXT NOT NULL DEFAULT '',
			destinations TEXT NOT NULL DEFAULT 'null'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_target_started ON runs (target_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_started ON runs (started_at DESC)`,
		`CREATE TABLE IF NOT EXISTS settings (
			"key"   TEXT PRIMARY KEY,
			"value" TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			data       TEXT NOT NULL,
			expires_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions (expires_at)`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("store: migrate: %w", err)
		}
	}

	// Idempotently add the target "key" column to pre-existing databases.
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE targets ADD COLUMN "key" TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("store: migrate target key: %w", err)
		}
	}
	// Enforce case-insensitive uniqueness of non-empty target keys (the API
	// pre-check uses EqualFold; the DB is the atomic enforcer, closing the
	// check-then-insert race). Replaces the older case-sensitive index.
	if _, err := s.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_targets_key_lower ON targets(lower("key")) WHERE "key" <> ''`); err != nil {
		return fmt.Errorf("store: migrate target key index: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_targets_key`); err != nil {
		return fmt.Errorf("store: drop old target key index: %w", err)
	}
	return nil
}

// Close closes the underlying database.
func (s *sqliteStore) Close() error { return s.db.Close() }

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// encConfig marshals v to JSON and encrypts it (pass-through when cipher==nil).
func (s *sqliteStore) encConfig(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("store: marshal config: %w", err)
	}
	enc, err := s.cipher.EncryptString(string(b))
	if err != nil {
		return "", fmt.Errorf("store: encrypt config: %w", err)
	}
	return enc, nil
}

// decConfig decrypts enc and unmarshals the JSON into v.
func (s *sqliteStore) decConfig(enc string, v any) error {
	plain, err := s.cipher.DecryptString(enc)
	if err != nil {
		return fmt.Errorf("store: decrypt config: %w", err)
	}
	if err := json.Unmarshal([]byte(plain), v); err != nil {
		return fmt.Errorf("store: unmarshal config: %w", err)
	}
	return nil
}

func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("store: marshal: %w", err)
	}
	return string(b), nil
}

func unmarshalJSON(s string, v any) error {
	if s == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(s), v); err != nil {
		return fmt.Errorf("store: unmarshal: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Channels
// ---------------------------------------------------------------------------

const channelColumns = `id, name, type, config, created_at, updated_at`

func (s *sqliteStore) scanChannel(sc scanner) (models.Channel, error) {
	var (
		c                      models.Channel
		typ, cfg, created, upd string
	)
	if err := sc.Scan(&c.ID, &c.Name, &typ, &cfg, &created, &upd); err != nil {
		return models.Channel{}, err
	}
	c.Type = models.ChannelType(typ)
	if err := s.decConfig(cfg, &c.Config); err != nil {
		// A decrypt failure (typically a changed/lost master key) must not break
		// listing. Return the row with an empty config so the admin can re-enter
		// its credentials or delete it from the panel.
		slog.Warn("channel config decrypt failed; returning empty config", "channel", c.ID, "err", err)
		c.Config = models.ChannelConfig{}
	}
	var err error
	if c.CreatedAt, err = parseTime(created); err != nil {
		return models.Channel{}, fmt.Errorf("store: parse created_at: %w", err)
	}
	if c.UpdatedAt, err = parseTime(upd); err != nil {
		return models.Channel{}, fmt.Errorf("store: parse updated_at: %w", err)
	}
	return c, nil
}

func (s *sqliteStore) ListChannels(ctx context.Context) ([]models.Channel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+channelColumns+` FROM channels ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list channels: %w", err)
	}
	defer rows.Close()

	var out []models.Channel
	for rows.Next() {
		c, err := s.scanChannel(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan channel: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list channels: %w", err)
	}
	return out, nil
}

func (s *sqliteStore) GetChannel(ctx context.Context, id string) (*models.Channel, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+channelColumns+` FROM channels WHERE id = ?`, id)
	c, err := s.scanChannel(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get channel: %w", err)
	}
	return &c, nil
}

func (s *sqliteStore) CreateChannel(ctx context.Context, c *models.Channel) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now
	cfg, err := s.encConfig(c.Config)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO channels (`+channelColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, string(c.Type), cfg, formatTime(c.CreatedAt), formatTime(c.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create channel: %w", err)
	}
	return nil
}

func (s *sqliteStore) UpdateChannel(ctx context.Context, c *models.Channel) error {
	c.UpdatedAt = time.Now().UTC()
	cfg, err := s.encConfig(c.Config)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE channels SET name = ?, type = ?, config = ?, updated_at = ? WHERE id = ?`,
		c.Name, string(c.Type), cfg, formatTime(c.UpdatedAt), c.ID)
	if err != nil {
		return fmt.Errorf("store: update channel: %w", err)
	}
	return checkAffected(res, "update channel")
}

func (s *sqliteStore) DeleteChannel(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM channels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete channel: %w", err)
	}
	return checkAffected(res, "delete channel")
}

// ---------------------------------------------------------------------------
// Notifiers
// ---------------------------------------------------------------------------

const notifierColumns = `id, name, type, enabled, events, config, created_at, updated_at`

func (s *sqliteStore) scanNotifier(sc scanner) (models.Notifier, error) {
	var (
		n                              models.Notifier
		typ, events, cfg, created, upd string
	)
	if err := sc.Scan(&n.ID, &n.Name, &typ, &n.Enabled, &events, &cfg, &created, &upd); err != nil {
		return models.Notifier{}, err
	}
	n.Type = models.NotifierType(typ)
	if err := unmarshalJSON(events, &n.Events); err != nil {
		return models.Notifier{}, err
	}
	if err := s.decConfig(cfg, &n.Config); err != nil {
		slog.Warn("notifier config decrypt failed; returning empty config", "notifier", n.ID, "err", err)
		n.Config = models.NotifierConfig{}
	}
	var err error
	if n.CreatedAt, err = parseTime(created); err != nil {
		return models.Notifier{}, fmt.Errorf("store: parse created_at: %w", err)
	}
	if n.UpdatedAt, err = parseTime(upd); err != nil {
		return models.Notifier{}, fmt.Errorf("store: parse updated_at: %w", err)
	}
	return n, nil
}

func (s *sqliteStore) ListNotifiers(ctx context.Context) ([]models.Notifier, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+notifierColumns+` FROM notifiers ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list notifiers: %w", err)
	}
	defer rows.Close()

	var out []models.Notifier
	for rows.Next() {
		n, err := s.scanNotifier(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan notifier: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list notifiers: %w", err)
	}
	return out, nil
}

func (s *sqliteStore) GetNotifier(ctx context.Context, id string) (*models.Notifier, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+notifierColumns+` FROM notifiers WHERE id = ?`, id)
	n, err := s.scanNotifier(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get notifier: %w", err)
	}
	return &n, nil
}

func (s *sqliteStore) CreateNotifier(ctx context.Context, n *models.Notifier) error {
	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	n.CreatedAt = now
	n.UpdatedAt = now
	events, err := marshalJSON(n.Events)
	if err != nil {
		return err
	}
	cfg, err := s.encConfig(n.Config)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO notifiers (`+notifierColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Name, string(n.Type), n.Enabled, events, cfg,
		formatTime(n.CreatedAt), formatTime(n.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create notifier: %w", err)
	}
	return nil
}

func (s *sqliteStore) UpdateNotifier(ctx context.Context, n *models.Notifier) error {
	n.UpdatedAt = time.Now().UTC()
	events, err := marshalJSON(n.Events)
	if err != nil {
		return err
	}
	cfg, err := s.encConfig(n.Config)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE notifiers SET name = ?, type = ?, enabled = ?, events = ?, config = ?, updated_at = ? WHERE id = ?`,
		n.Name, string(n.Type), n.Enabled, events, cfg, formatTime(n.UpdatedAt), n.ID)
	if err != nil {
		return fmt.Errorf("store: update notifier: %w", err)
	}
	return checkAffected(res, "update notifier")
}

func (s *sqliteStore) DeleteNotifier(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notifiers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete notifier: %w", err)
	}
	return checkAffected(res, "delete notifier")
}

// ---------------------------------------------------------------------------
// Targets
// ---------------------------------------------------------------------------

const targetColumns = `id, "key", name, source_path, enabled, schedule, retention, channel_ids, notifier_ids, archive, created_at, updated_at`

func (s *sqliteStore) scanTarget(sc scanner) (models.Target, error) {
	var (
		t                                                        models.Target
		schedule, retention, chIDs, ntIDs, archive, created, upd string
	)
	if err := sc.Scan(&t.ID, &t.Key, &t.Name, &t.SourcePath, &t.Enabled,
		&schedule, &retention, &chIDs, &ntIDs, &archive, &created, &upd); err != nil {
		return models.Target{}, err
	}
	if err := unmarshalJSON(schedule, &t.Schedule); err != nil {
		return models.Target{}, err
	}
	if err := unmarshalJSON(retention, &t.Retention); err != nil {
		return models.Target{}, err
	}
	if err := unmarshalJSON(chIDs, &t.ChannelIDs); err != nil {
		return models.Target{}, err
	}
	if err := unmarshalJSON(ntIDs, &t.NotifierIDs); err != nil {
		return models.Target{}, err
	}
	if err := unmarshalJSON(archive, &t.Archive); err != nil {
		return models.Target{}, err
	}
	var err error
	if t.CreatedAt, err = parseTime(created); err != nil {
		return models.Target{}, fmt.Errorf("store: parse created_at: %w", err)
	}
	if t.UpdatedAt, err = parseTime(upd); err != nil {
		return models.Target{}, fmt.Errorf("store: parse updated_at: %w", err)
	}
	return t, nil
}

func (s *sqliteStore) ListTargets(ctx context.Context) ([]models.Target, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+targetColumns+` FROM targets ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list targets: %w", err)
	}
	defer rows.Close()

	var out []models.Target
	for rows.Next() {
		t, err := s.scanTarget(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan target: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list targets: %w", err)
	}
	return out, nil
}

func (s *sqliteStore) GetTarget(ctx context.Context, id string) (*models.Target, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+targetColumns+` FROM targets WHERE id = ?`, id)
	t, err := s.scanTarget(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get target: %w", err)
	}
	return &t, nil
}

func (s *sqliteStore) targetJSONFields(t *models.Target) (schedule, retention, chIDs, ntIDs, archive string, err error) {
	if schedule, err = marshalJSON(t.Schedule); err != nil {
		return
	}
	if retention, err = marshalJSON(t.Retention); err != nil {
		return
	}
	if chIDs, err = marshalJSON(t.ChannelIDs); err != nil {
		return
	}
	if ntIDs, err = marshalJSON(t.NotifierIDs); err != nil {
		return
	}
	archive, err = marshalJSON(t.Archive)
	return
}

func (s *sqliteStore) CreateTarget(ctx context.Context, t *models.Target) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	schedule, retention, chIDs, ntIDs, archive, err := s.targetJSONFields(t)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO targets (`+targetColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Key, t.Name, t.SourcePath, t.Enabled, schedule, retention, chIDs, ntIDs, archive,
		formatTime(t.CreatedAt), formatTime(t.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create target: %w", err)
	}
	return nil
}

func (s *sqliteStore) UpdateTarget(ctx context.Context, t *models.Target) error {
	t.UpdatedAt = time.Now().UTC()
	schedule, retention, chIDs, ntIDs, archive, err := s.targetJSONFields(t)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE targets SET name = ?, source_path = ?, enabled = ?, schedule = ?, retention = ?,
			channel_ids = ?, notifier_ids = ?, archive = ?, updated_at = ? WHERE id = ?`,
		t.Name, t.SourcePath, t.Enabled, schedule, retention, chIDs, ntIDs, archive,
		formatTime(t.UpdatedAt), t.ID)
	if err != nil {
		return fmt.Errorf("store: update target: %w", err)
	}
	return checkAffected(res, "update target")
}

func (s *sqliteStore) DeleteTarget(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM targets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete target: %w", err)
	}
	return checkAffected(res, "delete target")
}

// ---------------------------------------------------------------------------
// Runs
// ---------------------------------------------------------------------------

const runColumns = `id, target_id, target_name, status, "trigger", started_at, finished_at, duration_ms, archive_key, size_bytes, file_count, message, destinations`

func (s *sqliteStore) scanRun(sc scanner) (models.BackupRun, error) {
	var (
		r            models.BackupRun
		status, trig string
		started      string
		finished     sql.NullString
		destinations string
	)
	if err := sc.Scan(&r.ID, &r.TargetID, &r.TargetName, &status, &trig,
		&started, &finished, &r.DurationMs, &r.ArchiveKey, &r.SizeBytes,
		&r.FileCount, &r.Message, &destinations); err != nil {
		return models.BackupRun{}, err
	}
	r.Status = models.RunStatus(status)
	r.Trigger = trig
	var err error
	if r.StartedAt, err = parseTime(started); err != nil {
		return models.BackupRun{}, fmt.Errorf("store: parse started_at: %w", err)
	}
	if finished.Valid && finished.String != "" {
		ft, err := parseTime(finished.String)
		if err != nil {
			return models.BackupRun{}, fmt.Errorf("store: parse finished_at: %w", err)
		}
		r.FinishedAt = &ft
	}
	if err := unmarshalJSON(destinations, &r.Destinations); err != nil {
		return models.BackupRun{}, err
	}
	return r, nil
}

func (s *sqliteStore) CreateRun(ctx context.Context, r *models.BackupRun) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now().UTC()
	}
	dest, err := marshalJSON(r.Destinations)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO runs (`+runColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.TargetID, r.TargetName, string(r.Status), r.Trigger,
		formatTime(r.StartedAt), nullTime(r.FinishedAt), r.DurationMs, r.ArchiveKey,
		r.SizeBytes, r.FileCount, r.Message, dest)
	if err != nil {
		return fmt.Errorf("store: create run: %w", err)
	}
	return nil
}

func (s *sqliteStore) UpdateRun(ctx context.Context, r *models.BackupRun) error {
	dest, err := marshalJSON(r.Destinations)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE runs SET target_id = ?, target_name = ?, status = ?, "trigger" = ?, started_at = ?,
			finished_at = ?, duration_ms = ?, archive_key = ?, size_bytes = ?, file_count = ?,
			message = ?, destinations = ? WHERE id = ?`,
		r.TargetID, r.TargetName, string(r.Status), r.Trigger, formatTime(r.StartedAt),
		nullTime(r.FinishedAt), r.DurationMs, r.ArchiveKey, r.SizeBytes, r.FileCount,
		r.Message, dest, r.ID)
	if err != nil {
		return fmt.Errorf("store: update run: %w", err)
	}
	return checkAffected(res, "update run")
}

func (s *sqliteStore) GetRun(ctx context.Context, id string) (*models.BackupRun, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+runColumns+` FROM runs WHERE id = ?`, id)
	r, err := s.scanRun(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get run: %w", err)
	}
	return &r, nil
}

func (s *sqliteStore) ListRuns(ctx context.Context, targetID string, limit int) ([]models.BackupRun, error) {
	if limit <= 0 {
		limit = 100
	}
	var (
		rows *sql.Rows
		err  error
	)
	if targetID == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+runColumns+` FROM runs ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+runColumns+` FROM runs WHERE target_id = ? ORDER BY started_at DESC, id DESC LIMIT ?`,
			targetID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list runs: %w", err)
	}
	defer rows.Close()

	var out []models.BackupRun
	for rows.Next() {
		r, err := s.scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan run: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list runs: %w", err)
	}
	return out, nil
}

func (s *sqliteStore) RecentRuns(ctx context.Context, limit int) ([]models.BackupRun, error) {
	return s.ListRuns(ctx, "", limit)
}

func (s *sqliteStore) PruneRuns(ctx context.Context, targetID string, keep int) error {
	if keep < 0 {
		keep = 0
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM runs WHERE target_id = ? AND id NOT IN (
			SELECT id FROM runs WHERE target_id = ? ORDER BY started_at DESC, id DESC LIMIT ?
		)`, targetID, targetID, keep)
	if err != nil {
		return fmt.Errorf("store: prune runs: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

func (s *sqliteStore) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT "value" FROM settings WHERE "key" = ?`, key).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("store: get setting: %w", err)
	}
	return v, nil
}

func (s *sqliteStore) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings ("key", "value") VALUES (?, ?)
			ON CONFLICT("key") DO UPDATE SET "value" = excluded."value"`,
		key, value)
	if err != nil {
		return fmt.Errorf("store: set setting: %w", err)
	}
	return nil
}

func (s *sqliteStore) AllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT "key", "value" FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("store: all settings: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("store: scan setting: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: all settings: %w", err)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

func (s *sqliteStore) CreateSession(ctx context.Context, sess *models.Session) error {
	if sess.ID == "" {
		sess.ID = uuid.NewString()
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	}
	data, err := marshalJSON(sess)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, data, expires_at) VALUES (?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET data = excluded.data, expires_at = excluded.expires_at`,
		sess.ID, data, formatTime(sess.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	return nil
}

func (s *sqliteStore) GetSession(ctx context.Context, id string) (*models.Session, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM sessions WHERE id = ?`, id).Scan(&data)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get session: %w", err)
	}
	var sess models.Session
	if err := json.Unmarshal([]byte(data), &sess); err != nil {
		return nil, fmt.Errorf("store: unmarshal session: %w", err)
	}
	return &sess, nil
}

func (s *sqliteStore) DeleteSession(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return checkAffected(res, "delete session")
}

func (s *sqliteStore) PruneExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("store: prune expired sessions: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

// nullTime converts an optional time pointer to a NULL-able TEXT value.
func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

// checkAffected turns a zero-rows-affected result into ErrNotFound.
func checkAffected(res sql.Result, op string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: %s: %w", op, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
