package github_secondary_ratelimit

import (
	"time"
)

type Option func(*Config)

// WithLimitDetectedCallback adds a callback to be called when a new active rate limit is detected.
func WithLimitDetectedCallback(callback OnLimitDetected) Option {
	return func(c *Config) {
		c.onLimitDetected = callback
	}
}

// WithSingleSleepLimit adds a limit to the duration allowed to wait for a single sleep (rate limit).
// The callback parameter is nillable.
func WithSingleSleepLimit(limit time.Duration, callback OnSingleLimitExceeded) Option {
	return func(c *Config) {
		c.singleSleepLimit = &limit
		c.onSingleLimitExceeded = callback
	}
}

// WithNoSleep avoid sleeping during secondary rate limits.
// it can be used to detect the limit but handle it out-of-band.
// It is a helper function around WithSingleSleepLimit.
// The callback parameter is nillable.
func WithNoSleep(callback OnSingleLimitExceeded) Option {
	return func(c *Config) {
		WithSingleSleepLimit(0, callback)
	}
}

// WithTotalSleepLimit adds a limit to the accumulated duration allowed to wait for all sleeps (one or more rate limits).
// The callback parameter is nillable.
func WithTotalSleepLimit(limit time.Duration, callback OnTotalLimitExceeded) Option {
	return func(c *Config) {
		c.totalSleepLimit = &limit
		c.onTotalLimitExceeded = callback
	}
}
