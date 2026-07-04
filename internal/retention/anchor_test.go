package retention

import (
	"testing"
	"time"

	"archivesync/internal/models"
)

// TestAnchor2400KeepsMidnight verifies the "24:00" anchor is normalized to
// midnight so the day's 00:00 backup is the one kept (regression for the bug
// where 24:00 -> 1440 minutes made 23:00 the "nearest" backup).
func TestAnchor2400KeepsMidnight(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// "now" is today 06:00 local; yesterday is older than the keep-all window.
	now := time.Date(2026, 7, 4, 6, 0, 0, 0, loc)

	mk := func(y, mo, d, h, mi int) Item {
		tt := time.Date(y, time.Month(mo), d, h, mi, 0, 0, loc)
		return Item{Key: "backups/" + FormatTimestamp(tt) + ".tar.gz", Time: tt}
	}
	yMidnight := mk(2026, 7, 3, 0, 0)
	yNoon := mk(2026, 7, 3, 12, 0)
	yLate := mk(2026, 7, 3, 23, 0)
	todayA := mk(2026, 7, 4, 1, 0)

	items := []Item{yMidnight, yNoon, yLate, todayA}
	policy := models.RetentionPolicy{
		Timezone:     "Asia/Shanghai",
		KeepAllDays:  1,
		DailyAnchors: []string{"24:00"},
		KeepDays:     7,
		MinKeep:      1,
	}
	keep, del := Plan(policy, items, now)

	in := func(set []string, key string) bool {
		for _, k := range set {
			if k == key {
				return true
			}
		}
		return false
	}
	if !in(keep, yMidnight.Key) {
		t.Errorf("expected yesterday 00:00 backup kept, keep=%v", keep)
	}
	if !in(del, yNoon.Key) || !in(del, yLate.Key) {
		t.Errorf("expected yesterday 12:00 and 23:00 deleted, del=%v", del)
	}
	if !in(keep, todayA.Key) {
		t.Errorf("expected today's backup kept, keep=%v", keep)
	}
}

// TestSimpleMaxVersions verifies that with no tiered policy, MaxVersions keeps
// exactly the newest N versions even when several fall on the same day
// (regression: the per-day collapse previously reduced them to one).
func TestSimpleMaxVersions(t *testing.T) {
	base := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	mk := func(offsetMin int) Item {
		tt := base.Add(time.Duration(offsetMin) * time.Minute)
		return Item{Key: FormatTimestamp(tt), Time: tt}
	}
	oldest := mk(0)
	mid := mk(30)
	newest := mk(60)
	items := []Item{oldest, mid, newest}

	keep, del := Plan(models.RetentionPolicy{MaxVersions: 2, MinKeep: 1}, items, base.Add(2*time.Hour))
	if len(keep) != 2 || len(del) != 1 {
		t.Fatalf("want keep=2 del=1, got keep=%v del=%v", keep, del)
	}
	if del[0] != oldest.Key {
		t.Errorf("expected oldest deleted, got del=%v", del)
	}
}
