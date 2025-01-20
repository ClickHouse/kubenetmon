package collector

import "time"

// ClockInterface is a dummy interface used to generate a mock "clock"
// implementation used in unit tests. This lets us dynamically test timestamped
// Prometheus metrics.
type ClockInterface interface {
	Now() time.Time
}

// Clock is like time.Now(), except it can be mocked.
type Clock struct{}

func (c Clock) Now() time.Time {
	return time.Now()
}
