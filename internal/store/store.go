// Package store defines the persistence interface for ArchiveSync and its
// sqlite-backed implementation. Secret config fields are encrypted at rest via
// crypto.Cipher inside the implementation.
package store

import (
	"context"
	"errors"

	"archivesync/internal/models"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// Store is the persistence layer. All methods are safe for concurrent use.
type Store interface {
	// Channels
	ListChannels(ctx context.Context) ([]models.Channel, error)
	GetChannel(ctx context.Context, id string) (*models.Channel, error)
	CreateChannel(ctx context.Context, c *models.Channel) error
	UpdateChannel(ctx context.Context, c *models.Channel) error
	DeleteChannel(ctx context.Context, id string) error

	// Notifiers
	ListNotifiers(ctx context.Context) ([]models.Notifier, error)
	GetNotifier(ctx context.Context, id string) (*models.Notifier, error)
	CreateNotifier(ctx context.Context, n *models.Notifier) error
	UpdateNotifier(ctx context.Context, n *models.Notifier) error
	DeleteNotifier(ctx context.Context, id string) error

	// Targets
	ListTargets(ctx context.Context) ([]models.Target, error)
	GetTarget(ctx context.Context, id string) (*models.Target, error)
	CreateTarget(ctx context.Context, t *models.Target) error
	UpdateTarget(ctx context.Context, t *models.Target) error
	DeleteTarget(ctx context.Context, id string) error

	// Runs (history)
	CreateRun(ctx context.Context, r *models.BackupRun) error
	UpdateRun(ctx context.Context, r *models.BackupRun) error
	GetRun(ctx context.Context, id string) (*models.BackupRun, error)
	// ListRuns returns runs for a target (or all if targetID==""), newest first.
	ListRuns(ctx context.Context, targetID string, limit int) ([]models.BackupRun, error)
	// RecentRuns returns the newest runs across all targets.
	RecentRuns(ctx context.Context, limit int) ([]models.BackupRun, error)
	// PruneRuns deletes history rows for a target beyond the newest keep count.
	PruneRuns(ctx context.Context, targetID string, keep int) error

	// Settings (key/value strings)
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
	AllSettings(ctx context.Context) (map[string]string, error)

	// Sessions
	CreateSession(ctx context.Context, s *models.Session) error
	GetSession(ctx context.Context, id string) (*models.Session, error)
	DeleteSession(ctx context.Context, id string) error
	PruneExpiredSessions(ctx context.Context) error

	Close() error
}
