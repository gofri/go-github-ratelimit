package github_secondary_ratelimit

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// SecondaryRateLimiter is a RoundTripper for handling GitHub secondary rate limits.
type SecondaryRateLimiter struct {
	Base           http.RoundTripper
	resetTime      *time.Time
	lock           sync.RWMutex
	totalSleepTime time.Duration
	config         *Config
}

// New creates a new SecondaryRateLimiter with the given base RoundTripper and options.
// see optins.go for available options.
// see RoundTrip() for the actual rate limit handling.
func New(base http.RoundTripper, opts ...Option) *SecondaryRateLimiter {
	if base == nil {
		base = http.DefaultTransport
	}

	waiter := SecondaryRateLimiter{
		Base:   base,
		config: newConfig(opts...),
	}

	return &waiter
}

// RoundTrip handles the secondary rate limit by waiting for it to finish before issuing new requests.
// If a request got a secondary rate limit error as a response, we retry the request after waiting.
// Issuing more requests during a secondary rate limit may cause a ban from the server side,
// so we want to prevent these requests, not just for the sake of cpu/network utilization.
// Nonetheless, there is no way to prevent subtle race conditions without completely serializing the requests,
// so we prefer to let some slip in case of a race condition, i.e.,
// after a retry-after response is received and before it is processed,
// a few other (concurrent) requests may be issued.
func (t *SecondaryRateLimiter) RoundTrip(request *http.Request) (*http.Response, error) {
	t.waitForRateLimit(request.Context())

	resp, err := t.Base.RoundTrip(request)
	if err != nil {
		return resp, err
	}

	secondaryLimit := parseSecondaryLimitTime(resp)
	if secondaryLimit == nil {
		return resp, nil
	}

	callbackContext := CallbackContext{
		Request:  request,
		Response: resp,
	}

	shouldRetry := t.updateRateLimit(*secondaryLimit, &callbackContext)
	if !shouldRetry {
		return resp, nil
	}

	return t.RoundTrip(request)
}

func (t *SecondaryRateLimiter) getRequestConfig(request *http.Request) *Config {
	overrides := GetConfigOverrides(request.Context())
	if overrides == nil {
		// no config override - use the default config (zero-copy)
		return t.config
	}
	reqConfig := *t.config
	reqConfig.ApplyOptions(overrides...)
	return &reqConfig
}

// waitForRateLimit waits for the cooldown time to finish if a secondary rate limit is active.
func (t *SecondaryRateLimiter) waitForRateLimit(ctx context.Context) {
	t.lock.RLock()
	sleepDuration := t.currentSleepDurationUnlocked()
	t.lock.RUnlock()

	_ = sleepWithContext(ctx, sleepDuration)
}

// updateRateLimit updates the active rate limit and triggers user callbacks if needed.
// the rate limit is not updated if there is already an active rate limit.
// it never waits because the retry handles sleeping anyway.
// returns whether or not to retry the request.
func (t *SecondaryRateLimiter) updateRateLimit(secondaryLimit time.Time, callbackContext *CallbackContext) (needRetry bool) {
	// quick check without the lock: maybe the secondary limit just passed
	if time.Now().After(secondaryLimit) {
		return true
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	// check before update if there is already an active rate limit
	if t.currentSleepDurationUnlocked() > 0 {
		return true
	}

	// check if the secondary rate limit happened to have passed while we waited for the lock
	sleepDuration := time.Until(secondaryLimit)
	if sleepDuration <= 0 {
		return true
	}

	config := t.getRequestConfig(callbackContext.Request)

	// do not sleep in case it is above the single sleep limit
	if config.IsAboveSingleSleepLimit(sleepDuration) {
		t.triggerCallback(config.onSingleLimitExceeded, callbackContext, secondaryLimit)
		return false
	}

	// do not sleep in case it is above the total sleep limit
	if config.IsAboveTotalSleepLimit(sleepDuration, t.totalSleepTime) {
		t.triggerCallback(config.onTotalLimitExceeded, callbackContext, secondaryLimit)
		return false
	}

	// a legitimate new limit
	t.resetTime = &secondaryLimit
	t.totalSleepTime += smoothSleepTime(sleepDuration)
	t.triggerCallback(config.onLimitDetected, callbackContext, secondaryLimit)

	return true
}

func (t *SecondaryRateLimiter) currentSleepDurationUnlocked() time.Duration {
	if t.resetTime == nil {
		return 0
	}
	return time.Until(*t.resetTime)
}

func (t *SecondaryRateLimiter) triggerCallback(callback func(*CallbackContext), callbackContext *CallbackContext, newResetTime time.Time) {
	if callback == nil {
		return
	}

	callbackContext.RoundTripper = t
	callbackContext.ResetTime = &newResetTime
	callbackContext.TotalSleepTime = &t.totalSleepTime

	callback(callbackContext)
}

// smoothSleepTime rounds up the sleep duration to whole seconds.
// github only uses seconds to indicate the time to sleep,
// but we sleep for less time because internal processing delay is taken into account.
// round up the duration to get the original value.
func smoothSleepTime(sleepTime time.Duration) time.Duration {
	if sleepTime.Milliseconds() == 0 {
		return sleepTime
	} else {
		seconds := sleepTime.Seconds() + 1
		return time.Duration(seconds) * time.Second
	}
}
