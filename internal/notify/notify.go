// Package notify defines the Notifier interface, the notification Event, a
// registry for concrete notifiers, and a Dispatcher that fans an event out to
// a set of configured notifiers with per-event filtering.
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"archivesync/internal/models"
)

// Event is a notification payload describing a backup lifecycle moment.
type Event struct {
	Type       string            // models.EventStart | EventSuccess | EventFailure
	TargetName string            // target display name
	Title      string            // short title, e.g. "备份成功: nginx-conf"
	Message    string            // human readable body
	Run        *models.BackupRun // optional detailed run record
	Timestamp  time.Time
	Fields     map[string]string // extra key/value details for rich formatting
}

// Notifier delivers an Event to a single destination.
type Notifier interface {
	Send(ctx context.Context, ev Event) error
	Kind() string
}

// Factory constructs a Notifier from its definition.
type Factory func(n models.Notifier) (Notifier, error)

var registry = map[models.NotifierType]Factory{}

// Register makes a notifier factory available to New. Called from impls' init().
func Register(t models.NotifierType, f Factory) { registry[t] = f }

// New constructs the notifier for the given definition.
func New(n models.Notifier) (Notifier, error) {
	f, ok := registry[n.Type]
	if !ok {
		return nil, fmt.Errorf("notify: unknown notifier type %q", n.Type)
	}
	return f(n)
}

// Types returns the registered notifier type identifiers.
func Types() []models.NotifierType {
	out := make([]models.NotifierType, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	return out
}

// wants reports whether n subscribes to the given event type.
func wants(n models.Notifier, evType string) bool {
	if !n.Enabled {
		return false
	}
	if len(n.Events) == 0 {
		return true // default: all events
	}
	for _, e := range n.Events {
		if e == evType {
			return true
		}
	}
	return false
}

// Dispatcher fans an Event out to a set of notifier definitions concurrently,
// filtering by each notifier's subscribed events.
type Dispatcher struct {
	notifiers []models.Notifier
	timeout   time.Duration
	log       *slog.Logger
}

// NewDispatcher builds a Dispatcher for the given notifier definitions.
func NewDispatcher(ns []models.Notifier, log *slog.Logger) *Dispatcher {
	if log == nil {
		log = slog.Default()
	}
	return &Dispatcher{notifiers: ns, timeout: 20 * time.Second, log: log}
}

// Dispatch sends ev to every subscribed notifier concurrently. Errors are
// logged, never returned, so notification failures cannot fail a backup.
func (d *Dispatcher) Dispatch(ctx context.Context, ev Event) {
	if ev.Timestamp.IsZero() {
		// caller should set it; leave zero-safe
	}
	var wg sync.WaitGroup
	for _, def := range d.notifiers {
		if !wants(def, ev.Type) {
			continue
		}
		wg.Add(1)
		go func(def models.Notifier) {
			defer wg.Done()
			n, err := New(def)
			if err != nil {
				d.log.Warn("notifier build failed", "notifier", def.Name, "err", err)
				return
			}
			cctx, cancel := context.WithTimeout(ctx, d.timeout)
			defer cancel()
			if err := n.Send(cctx, ev); err != nil {
				d.log.Warn("notification send failed", "notifier", def.Name, "type", def.Type, "err", err)
			}
		}(def)
	}
	wg.Wait()
}
