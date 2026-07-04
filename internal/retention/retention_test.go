package retention

import (
	"sort"
	"testing"
	"time"

	// Embed the IANA tz database so time.LoadLocation("Asia/Shanghai") resolves
	// in any test environment (Windows/CI without system zoneinfo). This affects
	// only the test binary, not the production package.
	_ "time/tzdata"

	"archivesync/internal/models"
)

// ---------------------------------------------------------------------------
// Timestamp helpers
// ---------------------------------------------------------------------------

func TestFormatParseRoundTrip(t *testing.T) {
	orig := time.Date(2026, 7, 4, 15, 30, 0, 0, time.UTC)
	tok := FormatTimestamp(orig)
	if tok != "20260704T153000Z" {
		t.Fatalf("FormatTimestamp = %q, want %q", tok, "20260704T153000Z")
	}
	got, ok := ParseTime("backups/app/app-" + tok + ".tar.gz")
	if !ok {
		t.Fatalf("ParseTime failed to find token")
	}
	if !got.Equal(orig) {
		t.Fatalf("ParseTime = %v, want %v", got, orig)
	}
	if got.Location() != time.UTC {
		t.Fatalf("ParseTime location = %v, want UTC", got.Location())
	}
}

func TestFormatUsesUTC(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// 2026-07-04 08:00 +08:00 == 2026-07-04 00:00 UTC.
	local := time.Date(2026, 7, 4, 8, 0, 0, 0, loc)
	if got := FormatTimestamp(local); got != "20260704T000000Z" {
		t.Fatalf("FormatTimestamp = %q, want %q", got, "20260704T000000Z")
	}
}

func TestParseTimeNoToken(t *testing.T) {
	cases := []string{
		"",
		"backups/app/app.tar.gz",
		"20260704-153000.tar.gz", // wrong shape
		"1234567T123456Z",        // 7 date digits
	}
	for _, c := range cases {
		if _, ok := ParseTime(c); ok {
			t.Errorf("ParseTime(%q) = ok, want not ok", c)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers for building fixtures
// ---------------------------------------------------------------------------

// item builds an Item whose key embeds the UTC timestamp of tm.
func item(tm time.Time) Item {
	return Item{Key: "app-" + FormatTimestamp(tm) + ".tar.gz", Time: tm}
}

func keySet(ks []string) map[string]bool {
	m := make(map[string]bool, len(ks))
	for _, k := range ks {
		m[k] = true
	}
	return m
}

func sameSet(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]string(nil), a...)
	bs := append([]string(nil), b...)
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Required scenario: Asia/Shanghai, 24 hourly backups/day for several days.
// ---------------------------------------------------------------------------

func TestPlanShanghaiHourly(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// End of day so that all of today's hourly backups are already in the past.
	now := time.Date(2026, 7, 4, 23, 59, 0, 0, loc)

	const days = 10 // today plus 9 older days
	var items []Item
	// dayHour[dayOffset][hour] = key, for assertions.
	dayHour := make([]map[int]string, days)
	for d := 0; d < days; d++ {
		date := now.AddDate(0, 0, -d)
		dayHour[d] = make(map[int]string)
		for h := 0; h < 24; h++ {
			tm := time.Date(date.Year(), date.Month(), date.Day(), h, 0, 0, 0, loc)
			it := item(tm)
			items = append(items, it)
			dayHour[d][h] = it.Key
		}
	}

	// keptHour returns the set of local hours kept for a given day offset.
	keptHours := func(keep []string, dayOffset int) map[int]bool {
		want := dayHour[dayOffset]
		inv := make(map[string]int, len(want))
		for h, k := range want {
			inv[k] = h
		}
		got := make(map[int]bool)
		for _, k := range keep {
			if h, ok := inv[k]; ok {
				got[h] = true
			}
		}
		return got
	}

	t.Run("single anchor 00:00", func(t *testing.T) {
		policy := models.RetentionPolicy{
			Timezone:     "Asia/Shanghai",
			KeepAllDays:  1,
			DailyAnchors: []string{"00:00"},
			KeepDays:     7,
		}
		keep, del := Plan(policy, items, now)

		// today: all 24 kept.
		if got := keptHours(keep, 0); len(got) != 24 {
			t.Fatalf("today kept %d hours, want 24", len(got))
		}
		// older days within KeepDays (offsets 1..6): exactly the 00:00 backup.
		for d := 1; d <= 6; d++ {
			got := keptHours(keep, d)
			if len(got) != 1 || !got[0] {
				t.Fatalf("day -%d kept hours %v, want exactly {0}", d, got)
			}
		}
		// days older than 7 (offsets 7..9): fully deleted.
		for d := 7; d < days; d++ {
			if got := keptHours(keep, d); len(got) != 0 {
				t.Fatalf("day -%d kept hours %v, want none", d, got)
			}
		}
		if wantKeep := 24 + 6; len(keep) != wantKeep {
			t.Fatalf("total kept = %d, want %d", len(keep), wantKeep)
		}
		if len(keep)+len(del) != len(items) {
			t.Fatalf("keep+del = %d, want %d", len(keep)+len(del), len(items))
		}
	})

	t.Run("two anchors 12:00 and 00:00", func(t *testing.T) {
		policy := models.RetentionPolicy{
			Timezone:     "Asia/Shanghai",
			KeepAllDays:  1,
			DailyAnchors: []string{"12:00", "00:00"}, // "24:00" normalized to "00:00"
			KeepDays:     7,
		}
		keep, del := Plan(policy, items, now)

		if got := keptHours(keep, 0); len(got) != 24 {
			t.Fatalf("today kept %d hours, want 24", len(got))
		}
		for d := 1; d <= 6; d++ {
			got := keptHours(keep, d)
			if len(got) != 2 || !got[0] || !got[12] {
				t.Fatalf("day -%d kept hours %v, want exactly {0,12}", d, got)
			}
		}
		for d := 7; d < days; d++ {
			if got := keptHours(keep, d); len(got) != 0 {
				t.Fatalf("day -%d kept hours %v, want none", d, got)
			}
		}
		if wantKeep := 24 + 6*2; len(keep) != wantKeep {
			t.Fatalf("total kept = %d, want %d", len(keep), wantKeep)
		}
		if len(keep)+len(del) != len(items) {
			t.Fatalf("keep+del = %d, want %d", len(keep)+len(del), len(items))
		}
	})
}

// ---------------------------------------------------------------------------
// Table-driven behavioral tests (UTC unless noted).
// ---------------------------------------------------------------------------

func TestPlanTable(t *testing.T) {
	utc := time.UTC
	at := func(y int, mo time.Month, d, h, mi int) time.Time {
		return time.Date(y, mo, d, h, mi, 0, 0, utc)
	}

	type tc struct {
		name   string
		policy models.RetentionPolicy
		items  []Item
		now    time.Time
		want   []Item // expected KEEP set
	}

	// Shared timestamps.
	// "now" reference for most UTC cases: 2026-01-10.
	tcs := []tc{
		{
			name:   "keepall today, latest of older days (unlimited age)",
			policy: models.RetentionPolicy{KeepAllDays: 1, KeepDays: 0},
			now:    at(2026, 1, 10, 12, 0),
			items: []Item{
				item(at(2026, 1, 10, 1, 0)),
				item(at(2026, 1, 10, 5, 0)),
				item(at(2026, 1, 10, 9, 0)),
				item(at(2026, 1, 9, 2, 0)),
				item(at(2026, 1, 9, 20, 0)),
				item(at(2026, 1, 8, 3, 0)),
				item(at(2026, 1, 8, 15, 0)),
			},
			want: []Item{
				item(at(2026, 1, 10, 1, 0)),
				item(at(2026, 1, 10, 5, 0)),
				item(at(2026, 1, 10, 9, 0)),
				item(at(2026, 1, 9, 20, 0)),
				item(at(2026, 1, 8, 15, 0)),
			},
		},
		{
			name:   "keepdays bounds the anchor window",
			policy: models.RetentionPolicy{KeepAllDays: 1, KeepDays: 2},
			now:    at(2026, 1, 10, 12, 0),
			items: []Item{
				item(at(2026, 1, 10, 9, 0)),
				item(at(2026, 1, 9, 2, 0)),
				item(at(2026, 1, 9, 20, 0)),
				item(at(2026, 1, 8, 3, 0)), // older than KeepDays => deleted
				item(at(2026, 1, 8, 15, 0)),
			},
			want: []Item{
				item(at(2026, 1, 10, 9, 0)),
				item(at(2026, 1, 9, 20, 0)),
			},
		},
		{
			name:   "maxversions trims oldest kept",
			policy: models.RetentionPolicy{KeepAllDays: 5, MaxVersions: 3},
			now:    at(2026, 1, 10, 23, 0),
			items: []Item{
				item(at(2026, 1, 10, 1, 0)),
				item(at(2026, 1, 10, 2, 0)),
				item(at(2026, 1, 10, 3, 0)),
				item(at(2026, 1, 10, 4, 0)),
				item(at(2026, 1, 10, 5, 0)),
				item(at(2026, 1, 10, 6, 0)),
			},
			want: []Item{
				item(at(2026, 1, 10, 4, 0)),
				item(at(2026, 1, 10, 5, 0)),
				item(at(2026, 1, 10, 6, 0)),
			},
		},
		{
			name:   "minkeep protects newest when all else deleted",
			policy: models.RetentionPolicy{KeepAllDays: 1, KeepDays: 1, MinKeep: 2},
			now:    at(2026, 1, 10, 12, 0),
			items: []Item{
				item(at(2026, 1, 5, 1, 0)),
				item(at(2026, 1, 5, 2, 0)),
				item(at(2026, 1, 5, 3, 0)),
			},
			want: []Item{
				item(at(2026, 1, 5, 2, 0)),
				item(at(2026, 1, 5, 3, 0)),
			},
		},
		{
			name:   "minkeep floors maxversions",
			policy: models.RetentionPolicy{KeepAllDays: 5, MaxVersions: 1, MinKeep: 3},
			now:    at(2026, 1, 10, 23, 0),
			items: []Item{
				item(at(2026, 1, 10, 1, 0)),
				item(at(2026, 1, 10, 2, 0)),
				item(at(2026, 1, 10, 3, 0)),
				item(at(2026, 1, 10, 4, 0)),
			},
			want: []Item{
				item(at(2026, 1, 10, 2, 0)),
				item(at(2026, 1, 10, 3, 0)),
				item(at(2026, 1, 10, 4, 0)),
			},
		},
		{
			name:   "anchor tie prefers earlier",
			policy: models.RetentionPolicy{KeepAllDays: 1, KeepDays: 3, DailyAnchors: []string{"12:00"}},
			now:    at(2026, 1, 10, 12, 0),
			items: []Item{
				item(at(2026, 1, 10, 10, 0)),
				item(at(2026, 1, 9, 11, 0)), // 60 min before anchor
				item(at(2026, 1, 9, 13, 0)), // 60 min after anchor -> tie, earlier wins
			},
			want: []Item{
				item(at(2026, 1, 10, 10, 0)),
				item(at(2026, 1, 9, 11, 0)),
			},
		},
		{
			name:   "empty timezone defaults to UTC day boundaries",
			policy: models.RetentionPolicy{Timezone: "", KeepAllDays: 1, KeepDays: 1},
			now:    at(2026, 1, 10, 0, 30),
			items: []Item{
				item(at(2026, 1, 9, 23, 30)), // yesterday in UTC -> deleted
				item(at(2026, 1, 10, 0, 10)), // today in UTC -> kept
			},
			want: []Item{
				item(at(2026, 1, 10, 0, 10)),
			},
		},
		{
			name:   "invalid timezone defaults to UTC",
			policy: models.RetentionPolicy{Timezone: "Not/AZone", KeepAllDays: 1, KeepDays: 1},
			now:    at(2026, 1, 10, 0, 30),
			items: []Item{
				item(at(2026, 1, 9, 23, 30)),
				item(at(2026, 1, 10, 0, 10)),
			},
			want: []Item{
				item(at(2026, 1, 10, 0, 10)),
			},
		},
		{
			name:   "empty input yields empty output",
			policy: models.RetentionPolicy{KeepAllDays: 1},
			now:    at(2026, 1, 10, 12, 0),
			items:  nil,
			want:   nil,
		},
	}

	for _, c := range tcs {
		t.Run(c.name, func(t *testing.T) {
			keep, del := Plan(c.policy, c.items, c.now)

			wantKeep := make([]string, len(c.want))
			for i, it := range c.want {
				wantKeep[i] = it.Key
			}
			if !sameSet(keep, wantKeep) {
				t.Errorf("keep = %v, want %v", keep, wantKeep)
			}

			// del must be exactly the complement of keep over the inputs.
			keepM := keySet(keep)
			var wantDel []string
			for _, it := range c.items {
				if !keepM[it.Key] {
					wantDel = append(wantDel, it.Key)
				}
			}
			if !sameSet(del, wantDel) {
				t.Errorf("del = %v, want %v", del, wantDel)
			}
			if len(keep)+len(del) != len(c.items) {
				t.Errorf("keep+del = %d, want %d", len(keep)+len(del), len(c.items))
			}
		})
	}
}

// TestPlanDeterministicOrder verifies keep/del come back sorted oldest-first.
func TestPlanDeterministicOrder(t *testing.T) {
	utc := time.UTC
	items := []Item{
		item(time.Date(2026, 1, 10, 3, 0, 0, 0, utc)),
		item(time.Date(2026, 1, 10, 1, 0, 0, 0, utc)),
		item(time.Date(2026, 1, 10, 2, 0, 0, 0, utc)),
	}
	policy := models.RetentionPolicy{KeepAllDays: 5}
	keep, _ := Plan(policy, items, time.Date(2026, 1, 10, 23, 0, 0, 0, utc))
	if !sort.SliceIsSorted(keep, func(i, j int) bool { return keep[i] < keep[j] }) {
		t.Fatalf("keep not deterministically sorted: %v", keep)
	}
	// Keys embed timestamps, so lexicographic order == chronological order here.
	want := []string{
		"app-20260110T010000Z.tar.gz",
		"app-20260110T020000Z.tar.gz",
		"app-20260110T030000Z.tar.gz",
	}
	for i := range want {
		if keep[i] != want[i] {
			t.Fatalf("keep[%d] = %q, want %q", i, keep[i], want[i])
		}
	}
}
