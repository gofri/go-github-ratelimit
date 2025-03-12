package github_secondary_ratelimit

import (
	"net/http"
	"time"
)

// CallbackContext is passed to all callbacks.
// Fields might be nillable, depending on the specific callback and field.
type CallbackContext struct {
	RoundTripper   *SecondaryRateLimiter
	ResetTime      *time.Time
	TotalSleepTime *time.Duration
	Request        *http.Request
	Response       *http.Response
}

// OnLimitDetected is a callback to be called when a new rate limit is detected (before the sleep).
// The totalSleepTime includes the sleep duration for the upcoming sleep.
// Note: called while holding the lock.
type OnLimitDetected func(*CallbackContext)

// OnSingleLimitPassed is a callback to be called when a rate limit is exceeding the limit for a single sleep.
// The ResetTime represents the end of sleep duration if the limit was not exceeded.
// The totalSleepTime does not include the sleep (that is not going to happen).
// Note: called while holding the lock.
type OnSingleLimitExceeded func(*CallbackContext)

// OnTotalLimitExceeded is a callback to be called when a rate limit is exceeding the limit for the total sleep.
// The ResetTime represents the end of sleep duration if the limit was not exceeded.
// The totalSleepTime does not include the sleep (that is not going to happen).
// Note: called while holding the lock.
type OnTotalLimitExceeded func(*CallbackContext)
