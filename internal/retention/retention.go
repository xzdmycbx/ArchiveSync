// Package retention implements ArchiveSync's tiered retention engine.
//
// Given a models.RetentionPolicy and a set of backup objects (each carrying the
// UTC timestamp embedded in its key), Plan decides which object keys to keep and
// which to delete. All "day" reasoning happens in the policy timezone.
//
// The package depends only on internal/models and the standard library.
package retention

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"archivesync/internal/models"
)

// TimeLayout is the UTC timestamp format embedded in object keys, e.g.
// "20060102T150405Z". Timestamps are always rendered in UTC.
const TimeLayout = "20060102T150405Z"

// Item is a single backup object considered by the retention engine. Time is
// the moment the backup was taken (normally parsed from Key via ParseTime by
// the caller); the engine treats Time as authoritative.
type Item struct {
	// Key is the storage object key (unique within its channel).
	Key string
	// Time is the backup instant. Callers should populate it, typically with
	// ParseTime(Key); the engine uses it as-is.
	Time time.Time
}

// tokenRE matches the "20060102T150405Z" timestamp token inside a key.
var tokenRE = regexp.MustCompile(`[0-9]{8}T[0-9]{6}Z`)

// FormatTimestamp renders t as the UTC timestamp token used in object keys.
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(TimeLayout)
}

// ParseTime extracts the "20060102T150405Z" timestamp token from a key or
// basename and parses it as a UTC time. The second result reports whether a
// valid token was found.
func ParseTime(key string) (time.Time, bool) {
	tok := tokenRE.FindString(key)
	if tok == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(TimeLayout, tok)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// Directory / filename layouts for the storage key scheme
//
//	<targetKey>/<YYYY-MM-DD>/<HH-MM-SS>.tar.gz
//
// The date and time are rendered in the target's timezone (loc).
const (
	DirDateLayout  = "2006-01-02"
	FileTimeLayout = "15-04-05"
)

var keyExts = []string{".tar.gz", ".tgz", ".zip", ".gz", ".tar"}

// ParsePathTime reconstructs the backup instant from a key whose last two path
// segments are "<YYYY-MM-DD>/<HH-MM-SS>.<ext>", interpreted in loc. It reports
// false when the key does not match this layout.
func ParsePathTime(key string, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.UTC
	}
	parts := strings.Split(key, "/")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	datePart := parts[len(parts)-2]
	base := parts[len(parts)-1]
	for _, ext := range keyExts {
		base = strings.TrimSuffix(base, ext)
	}
	d, err := time.ParseInLocation(DirDateLayout, datePart, loc)
	if err != nil {
		return time.Time{}, false
	}
	c, err := time.ParseInLocation(FileTimeLayout, base, loc)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(d.Year(), d.Month(), d.Day(), c.Hour(), c.Minute(), c.Second(), 0, loc), true
}

// loadLocation resolves an IANA timezone name, defaulting to UTC when the name
// is empty or cannot be loaded.
func loadLocation(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// parseAnchor converts an "HH:MM" anchor into minutes-since-local-midnight.
// Invalid anchors are reported via ok=false and skipped by callers.
func parseAnchor(s string) (int, bool) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	hh, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || hh < 0 || hh > 24 {
		return 0, false
	}
	mm, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || mm < 0 || mm > 59 {
		return 0, false
	}
	// "24:00" is a common way to express midnight (end of day); normalize it to
	// 0 minutes so it matches the day's midnight backup. "24:30" is invalid.
	if hh == 24 {
		if mm != 0 {
			return 0, false
		}
		return 0, true
	}
	return hh*60 + mm, true
}

// entry is the engine's internal, pre-computed view of an Item.
type entry struct {
	it      Item
	date    time.Time // local civil date at midnight, in policy tz
	dateKey string    // "20060102" grouping key for the local date
	minutes int       // minutes since local midnight
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Plan computes the retention decision for items under policy, evaluated as of
// now. It returns the keys to keep and the keys to delete (their union is every
// input key). Both slices are sorted deterministically by time then key.
//
// The semantics follow SPEC §4.5: an item is kept if ANY keep rule applies
// (MinKeep newest, keep-all recent days, per-day anchors within KeepDays), and
// MaxVersions then trims the oldest kept items down to the cap without ever
// dropping below MinKeep.
func Plan(policy models.RetentionPolicy, items []Item, now time.Time) (keep []string, del []string) {
	loc := loadLocation(policy.Timezone)

	minKeep := policy.MinKeep
	if minKeep <= 0 {
		minKeep = 1
	}
	keepAll := policy.KeepAllDays
	if keepAll < 0 {
		keepAll = 0
	}
	// tiered is true when a day-based policy is configured. Without one ("simple
	// mode"), MaxVersions means "keep the newest N versions" and the per-day
	// collapse of Rule 3 must not apply (it would reduce a day to one backup).
	tiered := keepAll >= 1 || len(policy.DailyAnchors) > 0 || policy.KeepDays > 0

	// today = the local civil date of now, at midnight.
	n := now.In(loc)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)

	entries := make([]entry, len(items))
	for i, it := range items {
		t := it.Time.In(loc)
		d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		entries[i] = entry{
			it:      it,
			date:    d,
			dateKey: d.Format("20060102"),
			minutes: t.Hour()*60 + t.Minute(),
		}
	}

	kept := make([]bool, len(entries))

	// order holds entry indices sorted newest-first (ties: smaller key first).
	order := make([]int, len(entries))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		ia, ib := order[a], order[b]
		ta, tb := entries[ia].it.Time, entries[ib].it.Time
		if !ta.Equal(tb) {
			return ta.After(tb)
		}
		return entries[ia].it.Key < entries[ib].it.Key
	})

	// Rule 1: always keep the newest minKeep items overall.
	for i := 0; i < minKeep && i < len(order); i++ {
		kept[order[i]] = true
	}

	// Window boundaries.
	var kaLow time.Time
	if keepAll >= 1 {
		// keep-all covers dates >= today-(keepAll-1).
		kaLow = today.AddDate(0, 0, -(keepAll - 1))
	}
	// Anchor window upper bound: the day just older than the keep-all window.
	anchorHigh := today.AddDate(0, 0, -keepAll)
	hasLow := policy.KeepDays > 0
	var anchorLow time.Time
	if hasLow {
		anchorLow = today.AddDate(0, 0, -(policy.KeepDays - 1))
	}

	// Rule 2: keep every item within the keep-all window.
	if keepAll >= 1 {
		for i := range entries {
			if !entries[i].date.Before(kaLow) {
				kept[i] = true
			}
		}
	}

	// Rule 3 (tiered mode only): per-day anchors for days older than the keep-all
	// window but within KeepDays. Group candidate entries by local date.
	if tiered {
		anchors := make([]int, 0, len(policy.DailyAnchors))
		for _, a := range policy.DailyAnchors {
			if m, ok := parseAnchor(a); ok {
				anchors = append(anchors, m)
			}
		}

		groups := make(map[string][]int)
		for i := range entries {
			d := entries[i].date
			inWindow := !d.After(anchorHigh) && (!hasLow || !d.Before(anchorLow))
			if inWindow {
				groups[entries[i].dateKey] = append(groups[entries[i].dateKey], i)
			}
		}

		for _, idxs := range groups {
			if len(anchors) > 0 {
				for _, anc := range anchors {
					best := -1
					for _, i := range idxs {
						if best == -1 || anchorBetter(entries[i], entries[best], anc) {
							best = i
						}
					}
					if best >= 0 {
						kept[best] = true
					}
				}
			} else {
				// No anchors: keep the single latest item of the day.
				best := -1
				for _, i := range idxs {
					if best == -1 || latestBetter(entries[i], entries[best]) {
						best = i
					}
				}
				if best >= 0 {
					kept[best] = true
				}
			}
		}
	} else if policy.MaxVersions > 0 {
		// Simple mode: no day-based policy, so MaxVersions means "keep the newest
		// N versions" outright (the MaxVersions cap below then trims any extras).
		for i := 0; i < policy.MaxVersions && i < len(order); i++ {
			kept[order[i]] = true
		}
	}

	// Rule 5: MaxVersions hard cap — drop oldest kept items, never below minKeep.
	if policy.MaxVersions > 0 {
		keptIdx := make([]int, 0, len(entries))
		for i := range entries {
			if kept[i] {
				keptIdx = append(keptIdx, i)
			}
		}
		if len(keptIdx) > policy.MaxVersions {
			target := policy.MaxVersions
			if target < minKeep {
				target = minKeep
			}
			sort.SliceStable(keptIdx, func(a, b int) bool {
				ia, ib := keptIdx[a], keptIdx[b]
				ta, tb := entries[ia].it.Time, entries[ib].it.Time
				if !ta.Equal(tb) {
					return ta.After(tb)
				}
				return entries[ia].it.Key < entries[ib].it.Key
			})
			for i := target; i < len(keptIdx); i++ {
				kept[keptIdx[i]] = false
			}
		}
	}

	// Partition and sort results deterministically (oldest first, then key).
	var keepIdx, delIdx []int
	for i := range entries {
		if kept[i] {
			keepIdx = append(keepIdx, i)
		} else {
			delIdx = append(delIdx, i)
		}
	}
	less := func(a, b int) bool {
		ta, tb := entries[a].it.Time, entries[b].it.Time
		if !ta.Equal(tb) {
			return ta.Before(tb)
		}
		return entries[a].it.Key < entries[b].it.Key
	}
	sort.SliceStable(keepIdx, func(a, b int) bool { return less(keepIdx[a], keepIdx[b]) })
	sort.SliceStable(delIdx, func(a, b int) bool { return less(delIdx[a], delIdx[b]) })

	keep = make([]string, len(keepIdx))
	for i, idx := range keepIdx {
		keep[i] = entries[idx].it.Key
	}
	del = make([]string, len(delIdx))
	for i, idx := range delIdx {
		del[i] = entries[idx].it.Key
	}
	return keep, del
}

// anchorBetter reports whether candidate c is a strictly better match for the
// anchor (minutes since midnight) than the current best b: nearer in absolute
// minute distance, ties broken by earlier time then smaller key.
func anchorBetter(c, b entry, anchor int) bool {
	dc := absInt(c.minutes - anchor)
	db := absInt(b.minutes - anchor)
	if dc != db {
		return dc < db
	}
	if !c.it.Time.Equal(b.it.Time) {
		return c.it.Time.Before(b.it.Time)
	}
	return c.it.Key < b.it.Key
}

// latestBetter reports whether candidate c is a later item than best b (ties
// broken by smaller key).
func latestBetter(c, b entry) bool {
	if !c.it.Time.Equal(b.it.Time) {
		return c.it.Time.After(b.it.Time)
	}
	return c.it.Key < b.it.Key
}
