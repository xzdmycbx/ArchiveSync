package scheduler

import (
	"testing"
	"time"

	"archivesync/internal/models"
)

// TestComputeNextCronFields verifies both 5-field and 6-field (seconds) cron
// expressions are accepted (regression for 6-field specs being rejected).
func TestComputeNextCronFields(t *testing.T) {
	from := time.Date(2026, 7, 4, 1, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		sc   models.Schedule
		hour int
		min  int
	}{
		{"5-field", models.Schedule{Mode: "cron", Cron: "30 3 * * *"}, 3, 30},
		{"6-field-seconds", models.Schedule{Mode: "cron", Cron: "0 30 3 * * *"}, 3, 30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			next := ComputeNext(c.sc, from)
			if next.IsZero() {
				t.Fatalf("ComputeNext returned zero for %q", c.sc.Cron)
			}
			if next.Hour() != c.hour || next.Minute() != c.min || next.Second() != 0 {
				t.Errorf("got %v, want hour=%d min=%d sec=0", next, c.hour, c.min)
			}
		})
	}
}

// TestComputeNextTimesTZ verifies a times schedule with a timezone produces a
// valid next run.
func TestComputeNextTimesTZ(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	from := time.Date(2026, 7, 4, 6, 0, 0, 0, loc)
	sc := models.Schedule{Mode: "times", Times: []string{"00:00", "12:00"}, Timezone: "Asia/Shanghai"}
	next := ComputeNext(sc, from)
	if next.IsZero() {
		t.Fatal("expected a next run, got zero")
	}
	// From 06:00, the next anchor is 12:00 local.
	if got := next.In(loc).Hour(); got != 12 {
		t.Errorf("next run hour = %d, want 12 (local)", got)
	}
}

// TestValidateSchedule accepts valid schedules and rejects invalid cron / time
// / timezone so the API can reject them at save time (regression for silently
// dropped targets).
func TestValidateSchedule(t *testing.T) {
	ok := []models.Schedule{
		{Mode: "cron", Cron: "0 3 * * *", Timezone: "Asia/Shanghai"},
		{Mode: "cron", Cron: "0 0 3 * * *"}, // 6-field
		{Mode: "times", TimesPerDay: 24, Timezone: "UTC"},
		{Mode: "times", Times: []string{"00:00", "12:00"}},
		{Mode: "interval", IntervalMin: 30},
	}
	for _, sc := range ok {
		if err := ValidateSchedule(sc); err != nil {
			t.Errorf("expected valid, got error for %+v: %v", sc, err)
		}
	}
	bad := []models.Schedule{
		{Mode: "cron", Cron: "not-a-cron"},
		{Mode: "times", Times: []string{"25:99"}},
		{Mode: "times", TimesPerDay: 24, Timezone: "Mars/Phobos"},
		{Mode: "interval", IntervalMin: 0},
		{Mode: "bogus"},
	}
	for _, sc := range bad {
		if err := ValidateSchedule(sc); err == nil {
			t.Errorf("expected error for %+v, got nil", sc)
		}
	}
}

// TestEvenlySpacedTimes verifies "N times per day" distributes from 00:00.
func TestEvenlySpacedTimes(t *testing.T) {
	got := evenlySpacedTimes(24)
	if len(got) != 24 {
		t.Fatalf("want 24 times, got %d", len(got))
	}
	if got[0] != "00:00" || got[1] != "01:00" || got[23] != "23:00" {
		t.Errorf("unexpected distribution: %v", got[:3])
	}
	four := evenlySpacedTimes(4)
	want := []string{"00:00", "06:00", "12:00", "18:00"}
	for i := range want {
		if four[i] != want[i] {
			t.Errorf("4/day: got %v want %v", four, want)
			break
		}
	}
}
