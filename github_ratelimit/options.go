package github_ratelimit

import (
	"time"
)

type Option func(*SecondaryRateLimitWaiter)

// WithLimitDetectedCallback adds a callback to be called when a new active rate limit is detected.
func WithLimitDetectedCallback(callback OnLimitDetected) Option {
	return func(t *SecondaryRateLimitWaiter) {
		t.onLimitDetected = callback
	}
}

// WithSingleSleepLimit adds a limit to the duration allowed to wait for a single sleep (rate limit).
// The callback parameter is nillable.
func WithSingleSleepLimit(limit time.Duration, callback OnSingleLimitExceeded) Option {
	return func(t *SecondaryRateLimitWaiter) {
		t.singleSleepLimit = &limit
		t.onSingleLimitExceeded = callback
	}
}

// WithTotalSleepLimit adds a limit to the accumulated duration allowed to wait for all sleeps (one or more rate limits).
// The callback parameter is nillable.
func WithTotalSleepLimit(limit time.Duration, callback OnTotalLimitExceeded) Option {
	return func(t *SecondaryRateLimitWaiter) {
		t.totalSleepLimit = &limit
		t.onTotalLimitExceeded = callback
	}
}

func applyOptions(w *SecondaryRateLimitWaiter, opts ...Option) {
	for _, o := range opts {
		if o == nil {
			continue
		}
		o(w)
	}
}
