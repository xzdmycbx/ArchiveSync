// Package scheduler drives targets on their configured schedules using a cron
// engine. It can be reloaded at runtime when targets change.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"archivesync/internal/backup"
	"archivesync/internal/models"
	"archivesync/internal/store"

	"github.com/robfig/cron/v3"
)

// Scheduler schedules and runs backups for enabled targets.
type Scheduler struct {
	store  store.Store
	engine *backup.Engine
	log    *slog.Logger

	mu      sync.Mutex
	cron    *cron.Cron
	entries map[string][]cron.EntryID // targetID -> cron entry IDs
	started bool
}

// New builds a Scheduler.
func New(st store.Store, engine *backup.Engine, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		store:   st,
		engine:  engine,
		log:     log,
		entries: make(map[string][]cron.EntryID),
	}
}

// Start loads all targets and begins the cron loop.
func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.Reload(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	if !s.started {
		s.cron.Start()
		s.started = true
	}
	s.mu.Unlock()
	return nil
}

// Reload rebuilds the schedule from the current set of targets. Safe to call
// while running (e.g. after a target is created/updated/deleted).
func (s *Scheduler) Reload(ctx context.Context) error {
	targets, err := s.store.ListTargets(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Rebuild a fresh cron. Stopping the old one (if any) cancels its jobs.
	if s.cron != nil {
		s.cron.Stop()
	}
	s.cron = cron.New(cron.WithParser(specParser))
	s.entries = make(map[string][]cron.EntryID)

	for i := range targets {
		t := targets[i]
		if !t.Enabled {
			continue
		}
		specs, err := buildSpecs(t.Schedule)
		if err != nil {
			s.log.Warn("invalid schedule; target not scheduled", "target", t.Name, "err", err)
			continue
		}
		for _, spec := range specs {
			id, err := s.cron.AddFunc(spec, s.jobFor(t.ID, t.Name))
			if err != nil {
				s.log.Warn("failed to add schedule", "target", t.Name, "spec", spec, "err", err)
				continue
			}
			s.entries[t.ID] = append(s.entries[t.ID], id)
		}
		s.log.Info("scheduled target", "target", t.Name, "specs", specs)
	}

	if s.started {
		s.cron.Start()
	}
	return nil
}

// jobFor returns the cron job function for a target.
func (s *Scheduler) jobFor(targetID, name string) func() {
	return func() {
		s.log.Info("scheduled backup starting", "target", name)
		if _, err := s.engine.Run(context.Background(), targetID, "schedule"); err != nil {
			s.log.Error("scheduled backup failed", "target", name, "err", err)
		}
	}
}

// NextRun returns the earliest next scheduled run for a target, or zero time if
// the target has no active schedule.
func (s *Scheduler) NextRun(targetID string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron == nil {
		return time.Time{}
	}
	var next time.Time
	for _, id := range s.entries[targetID] {
		e := s.cron.Entry(id)
		if e.Next.IsZero() {
			continue
		}
		if next.IsZero() || e.Next.Before(next) {
			next = e.Next
		}
	}
	return next
}

// Stop halts the cron loop.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		s.cron.Stop()
	}
	s.started = false
}

// specParser parses standard 5-field specs, optional-seconds 6-field specs, and
// descriptors (@every, ...), and honors the CRON_TZ= prefix. The same parser
// backs both the running cron and ComputeNext so they accept identical specs.
var specParser = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ComputeNext returns the earliest next fire time for a schedule after `from`,
// without needing a running cron. Returns zero time if the schedule is invalid.
func ComputeNext(sc models.Schedule, from time.Time) time.Time {
	specs, err := buildSpecs(sc)
	if err != nil {
		return time.Time{}
	}
	var next time.Time
	for _, spec := range specs {
		sched, err := specParser.Parse(spec)
		if err != nil {
			continue
		}
		n := sched.Next(from)
		if n.IsZero() {
			continue
		}
		if next.IsZero() || n.Before(next) {
			next = n
		}
	}
	return next
}

// ValidateSchedule reports an error if a schedule cannot actually be scheduled,
// applying exactly the same parsing the running scheduler uses (build specs +
// parse each). Returns nil when the schedule is valid.
func ValidateSchedule(sc models.Schedule) error {
	specs, err := buildSpecs(sc)
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if _, err := specParser.Parse(spec); err != nil {
			return fmt.Errorf("无法解析调度 %q: %w", spec, err)
		}
	}
	return nil
}

// buildSpecs converts a Schedule into one or more cron specifications.
func buildSpecs(sc models.Schedule) ([]string, error) {
	tz := strings.TrimSpace(sc.Timezone)
	prefix := ""
	if tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return nil, fmt.Errorf("unknown timezone %q: %w", tz, err)
		}
		prefix = "CRON_TZ=" + tz + " "
	}

	switch sc.Mode {
	case "cron":
		spec := strings.TrimSpace(sc.Cron)
		if spec == "" {
			return nil, fmt.Errorf("cron expression is empty")
		}
		return []string{prefix + spec}, nil

	case "interval":
		if sc.IntervalMin <= 0 {
			return nil, fmt.Errorf("interval minutes must be > 0")
		}
		return []string{fmt.Sprintf("@every %dm", sc.IntervalMin)}, nil

	case "times":
		times := sc.Times
		if len(times) == 0 {
			if sc.TimesPerDay <= 0 {
				return nil, fmt.Errorf("times schedule needs times[] or times_per_day")
			}
			times = evenlySpacedTimes(sc.TimesPerDay)
		}
		specs := make([]string, 0, len(times))
		for _, hm := range times {
			h, m, err := parseHM(hm)
			if err != nil {
				return nil, err
			}
			specs = append(specs, fmt.Sprintf("%s%d %d * * *", prefix, m, h))
		}
		return specs, nil

	default:
		return nil, fmt.Errorf("unknown schedule mode %q", sc.Mode)
	}
}

// evenlySpacedTimes returns n "HH:MM" times evenly spread across 24h from 00:00.
func evenlySpacedTimes(n int) []string {
	if n <= 0 {
		return nil
	}
	if n > 1440 {
		n = 1440
	}
	stepMin := (24 * 60) / n
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		total := i * stepMin
		out = append(out, fmt.Sprintf("%02d:%02d", total/60, total%60))
	}
	return out
}

// parseHM parses "HH:MM" (24h). "24:00" is normalized to "0:00".
func parseHM(s string) (int, int, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time %q, want HH:MM", s)
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, 0, fmt.Errorf("invalid hour in %q", s)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, 0, fmt.Errorf("invalid minute in %q", s)
	}
	if h == 24 && m == 0 {
		h = 0
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("time out of range %q", s)
	}
	return h, m, nil
}

// SortedTimes returns the normalized, de-duplicated, sorted HH:MM list a "times"
// schedule will fire at (useful for display/validation).
func SortedTimes(sc models.Schedule) []string {
	times := sc.Times
	if len(times) == 0 && sc.TimesPerDay > 0 {
		times = evenlySpacedTimes(sc.TimesPerDay)
	}
	seen := map[string]bool{}
	var out []string
	for _, hm := range times {
		h, m, err := parseHM(hm)
		if err != nil {
			continue
		}
		v := fmt.Sprintf("%02d:%02d", h, m)
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
