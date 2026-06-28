package sumup

import "time"

// Additional renewal cadences beyond the Monthly/Yearly defined in sumup.go.
// Interval is an open string type, so apps can also define their own and supply
// the next-date logic; Next here understands these built-ins and falls back to
// monthly for anything unknown.
const (
	Weekly       Interval = "weekly"
	Quarterly    Interval = "quarterly"     // every 3 months
	SemiAnnually Interval = "semi_annually" // every 6 months
)

// nextBuiltin returns the next billing time for the cadences this package knows,
// and ok=false for ones it doesn't (Next then applies the monthly fallback).
func nextBuiltin(i Interval, t time.Time) (time.Time, bool) {
	switch i {
	case Weekly:
		return t.AddDate(0, 0, 7), true
	case Quarterly:
		return t.AddDate(0, 3, 0), true
	case SemiAnnually:
		return t.AddDate(0, 6, 0), true
	case Yearly:
		return t.AddDate(1, 0, 0), true
	case Monthly:
		return t.AddDate(0, 1, 0), true
	}
	return time.Time{}, false
}
