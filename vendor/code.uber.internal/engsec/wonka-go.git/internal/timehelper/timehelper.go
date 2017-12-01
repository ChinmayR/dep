package timehelper

import "time"

// WithinClockSkew returns true if a is within time.Time(b) +/- time.Duration(s)
func WithinClockSkew(a, b time.Time, s time.Duration) bool {
	return b.Add(-s).Before(a) && b.Add(s).After(a)
}
