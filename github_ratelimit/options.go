package github_ratelimit

import "time"

type Option func(*SecondaryRateLimitWaiter)

// OnLimitDetected is a callback to be called when a new rate limit is detected (before the sleep)
// The totalSleepTime includes the sleep time for the upcoming sleep
// Note: called while holding the lock.
type OnLimitDetected func(sleepUntil time.Time, totalSleepTime time.Duration)

// OnSingleLimitPassed is a callback to be called when a rate limit is exceeding the limit for a single sleep.
// The sleepUntil represents the end of sleep time if the limit was not exceeded.
// The totalSleepTime does not include the sleep (that is not going to happen).
// Note: called while holding the lock.
type OnSingleLimitExceeded func(sleepUntil time.Time, totalSleepTime time.Duration)

// OnTotalLimitExceeded is a callback to be called when a rate limit is exceeding the limit for the total sleep.
// The sleepUntil represents the end of sleep time if the limit was not exceeded.
// The totalSleepTime does not include the sleep (that is not going to happen).
// Note: called while holding the lock.
type OnTotalLimitExceeded func(sleepUntil time.Time, totalSleepTime time.Duration)

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
