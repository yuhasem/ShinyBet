package core

import "time"

// Cron is the interface for a Core to periodically run tasks.
type Cron interface {
	// ID returns a string suitable for uniquely identifying this cron. This
	// will be used in database interactions and error messages.
	ID() string
	// After returns the duration to wait before running this cron again.
	After() time.Duration
	// Run runs whatever the cron wants to do. A returned error is printed, but
	// otherwise ignored. Panics are also caught and recovered.
	Run() error
}
