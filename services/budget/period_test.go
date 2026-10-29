package budget

import (
	"testing"
	"time"
)

func TestPeriodClampsAnchorToMonthEnd(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }
	at := func(y int, m time.Month, day, h int) time.Time { return time.Date(y, m, day, h, 0, 0, 0, time.UTC) }
	jan31, jan30, jan15 := d(2026, 1, 31), d(2026, 1, 30), d(2026, 1, 15)

	cases := []struct {
		name            string
		ref             *time.Time
		now             time.Time
		wantStart, want time.Time
	}{
		{"anchor 31, Mar 1", &jan31, at(2026, 3, 1, 12), d(2026, 2, 28), d(2026, 3, 31)},
		{"anchor 31, Mar 2", &jan31, at(2026, 3, 2, 12), d(2026, 2, 28), d(2026, 3, 31)},
		{"anchor 31, Feb 27", &jan31, at(2026, 2, 27, 12), d(2026, 1, 31), d(2026, 2, 28)},
		{"anchor 31, Feb 28", &jan31, at(2026, 2, 28, 0), d(2026, 2, 28), d(2026, 3, 31)},
		{"anchor 31, leap Feb 29", &jan31, at(2028, 2, 29, 1), d(2028, 2, 29), d(2028, 3, 31)},
		{"anchor 31, Apr 30", &jan31, at(2026, 4, 30, 1), d(2026, 4, 30), d(2026, 5, 31)},
		{"anchor 31, May 1", &jan31, at(2026, 5, 1, 1), d(2026, 4, 30), d(2026, 5, 31)},
		{"anchor 30, Mar 1", &jan30, at(2026, 3, 1, 12), d(2026, 2, 28), d(2026, 3, 30)},
		{"anchor 15, Jan 3", &jan15, at(2026, 1, 3, 1), d(2025, 12, 15), d(2026, 1, 15)},
		{"anchor 31, Dec 31", &jan31, at(2026, 12, 31, 1), d(2026, 12, 31), d(2027, 1, 31)},
		{"no anchor", nil, at(2026, 2, 14, 1), d(2026, 2, 1), d(2026, 3, 1)},
	}
	for _, c := range cases {
		start, next := Period(c.ref, c.now)
		if !start.Equal(c.wantStart) || !next.Equal(c.want) {
			t.Errorf("%s: Period = %v .. %v, want %v .. %v", c.name, start, next, c.wantStart, c.want)
		}
		if c.now.Before(start) || !c.now.Before(next) {
			t.Errorf("%s: now %v outside its period %v .. %v", c.name, c.now, start, next)
		}
	}
}

// Every instant falls in exactly one period: consecutive periods meet with
// no gap or overlap, whatever the anchor day.
func TestPeriodsAreContiguous(t *testing.T) {
	for day := 1; day <= 31; day++ {
		ref := time.Date(2026, 1, day, 9, 30, 0, 0, time.UTC)
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		_, next := Period(&ref, now)
		for i := 0; i < 30; i++ {
			start, following := Period(&ref, next)
			if !start.Equal(next) {
				t.Fatalf("anchor %d: period after %v starts at %v", day, next, start)
			}
			if prevStart, _ := Period(&ref, next.Add(-time.Nanosecond)); !prevStart.Before(start) {
				t.Fatalf("anchor %d: instant before %v is in period %v", day, next, prevStart)
			}
			next = following
		}
	}
}

func TestSyncPeriod(t *testing.T) {
	jan31 := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	start, end := SyncPeriod(&jan31, time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	if want := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Errorf("start = %v, want %v", start, want)
	}
	if want := time.Date(2026, 3, 30, 23, 59, 59, 0, time.UTC); !end.Equal(want) {
		t.Errorf("end = %v, want %v", end, want)
	}

	// A reset part way through the period starts it at the reset.
	reset := time.Date(2026, 3, 10, 14, 5, 7, 500, time.UTC)
	start, end = SyncPeriod(&reset, time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC))
	if want := time.Date(2026, 3, 10, 14, 5, 7, 0, time.UTC); !start.Equal(want) {
		t.Errorf("reset start = %v, want %v", start, want)
	}
	if want := time.Date(2026, 4, 9, 23, 59, 59, 0, time.UTC); !end.Equal(want) {
		t.Errorf("reset end = %v, want %v", end, want)
	}
}
