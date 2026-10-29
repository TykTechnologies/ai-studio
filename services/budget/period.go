package budget

import "time"

// Period returns the monthly budget period containing now, anchored on the
// day of month of ref (the 1st when ref is nil), in now's location. start is
// inclusive and next, the start of the following period, exclusive. Anchor
// days past the end of a short month fall on its last day: an anchor on the
// 31st starts periods on Feb 28 (or 29), Mar 31, Apr 30.
func Period(ref *time.Time, now time.Time) (start, next time.Time) {
	day := 1
	if ref != nil {
		day = ref.Day()
	}
	start = anchorIn(now.Year(), now.Month(), day, now.Location())
	if now.Before(start) {
		prev := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -1, 0)
		start = anchorIn(prev.Year(), prev.Month(), day, now.Location())
	}
	following := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, 1, 0)
	return start, anchorIn(following.Year(), following.Month(), day, now.Location())
}

// PeriodStart returns the start of the budget period containing now (see
// Period).
func PeriodStart(ref *time.Time, now time.Time) time.Time {
	start, _ := Period(ref, now)
	return start
}

// SyncPeriod is the budget period the hub's budget sync and the edges key
// spend by: Period, with end the last second of the period (inclusive) rather
// than the next start. When ref falls inside the period (a budget reset part
// way through it), the period starts at ref itself, truncated to the second,
// so usage from before the reset is not counted.
func SyncPeriod(ref *time.Time, now time.Time) (start, end time.Time) {
	start, next := Period(ref, now)
	end = next.Add(-time.Second)
	if ref != nil && ref.After(start) && ref.Before(end) {
		start = ref.Truncate(time.Second)
	}
	return start, end
}

// anchorIn is midnight on day of the month, or on the month's last day when
// it has fewer days.
func anchorIn(year int, month time.Month, day int, loc *time.Location) time.Time {
	if last := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day(); day > last {
		day = last
	}
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}
