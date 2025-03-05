package github_ratelimit

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type SecondaryRateLimitWaiter struct {
	Base           http.RoundTripper
	sleepUntil     *time.Time
	lock           sync.RWMutex
	totalSleepTime time.Duration
	config         *SecondaryRateLimitConfig
}

func NewRateLimitWaiter(base http.RoundTripper, opts ...Option) (*SecondaryRateLimitWaiter, error) {
	if base == nil {
		base = http.DefaultTransport
	}

	waiter := SecondaryRateLimitWaiter{
		Base:   base,
		config: newConfig(opts...),
	}

	return &waiter, nil
}

func NewRateLimitWaiterClient(base http.RoundTripper, opts ...Option) (*http.Client, error) {
	waiter, err := NewRateLimitWaiter(base, opts...)
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Transport: waiter,
	}, nil
}

// RoundTrip handles the secondary rate limit by waiting for it to finish before issuing new requests.
// If a request got a secondary rate limit error as a response, we retry the request after waiting.
// Issuing more requests during a secondary rate limit may cause a ban from the server side,
// so we want to prevent these requests, not just for the sake of cpu/network utilization.
// Nonetheless, there is no way to prevent subtle race conditions without completely serializing the requests,
// so we prefer to let some slip in case of a race condition, i.e.,
// after a retry-after response is received and before it is processed,
// a few other (concurrent) requests may be issued.
func (t *SecondaryRateLimitWaiter) RoundTrip(request *http.Request) (*http.Response, error) {
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

func (t *SecondaryRateLimitWaiter) getRequestConfig(request *http.Request) *SecondaryRateLimitConfig {
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
func (t *SecondaryRateLimitWaiter) waitForRateLimit(ctx context.Context) {
	t.lock.RLock()
	sleepDuration := t.currentSleepDurationUnlocked()
	t.lock.RUnlock()

	sleepWithContext(ctx, sleepDuration)
}

// updateRateLimit updates the active rate limit and triggers user callbacks if needed.
// the rate limit is not updated if there is already an active rate limit.
// it never waits because the retry handles sleeping anyway.
// returns whether or not to retry the request.
func (t *SecondaryRateLimitWaiter) updateRateLimit(secondaryLimit time.Time, callbackContext *CallbackContext) (needRetry bool) {
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
	t.sleepUntil = &secondaryLimit
	t.totalSleepTime += smoothSleepTime(sleepDuration)
	t.triggerCallback(config.onLimitDetected, callbackContext, secondaryLimit)

	return true
}

func (t *SecondaryRateLimitWaiter) currentSleepDurationUnlocked() time.Duration {
	if t.sleepUntil == nil {
		return 0
	}
	return time.Until(*t.sleepUntil)
}

func (t *SecondaryRateLimitWaiter) triggerCallback(callback func(*CallbackContext), callbackContext *CallbackContext, newSleepUntil time.Time) {
	if callback == nil {
		return
	}

	callbackContext.RoundTripper = t
	callbackContext.SleepUntil = &newSleepUntil
	callbackContext.TotalSleepTime = &t.totalSleepTime

	callback(callbackContext)
}

// parseSecondaryLimitTime parses the GitHub API response header,
// looking for the secondary rate limit as defined by GitHub API documentation.
// https://docs.github.com/en/rest/overview/resources-in-the-rest-api#secondary-rate-limits
func parseSecondaryLimitTime(resp *http.Response) *time.Time {
	if !isSecondaryRateLimit(resp) {
		return nil
	}

	if sleepUntil := parseRetryAfter(resp.Header); sleepUntil != nil {
		return sleepUntil
	}

	if sleepUntil := parseXRateLimitReset(resp); sleepUntil != nil {
		return sleepUntil
	}

	// XXX: per GitHub API docs, we should default to a 60 seconds sleep duration in case the header is missing,
	//		with an exponential backoff mechanism.
	//		we may want to implement this in the future (with configurable limits),
	//		but let's avoid it while there are no known cases of missing headers.
	return nil
}

// parseRetryAfter parses the GitHub API response header in case a Retry-After is returned.
func parseRetryAfter(header http.Header) *time.Time {
	retryAfterSeconds, ok := httpHeaderIntValue(header, "retry-after")
	if !ok || retryAfterSeconds <= 0 {
		return nil
	}

	// per GitHub API, the header is set to the number of seconds to wait
	sleepUntil := time.Now().Add(time.Duration(retryAfterSeconds) * time.Second)

	return &sleepUntil
}

// parseXRateLimitReset parses the GitHub API response header in case a x-ratelimit-reset is returned.
// to avoid handling primary rate limits (which are categorized),
// we only handle x-ratelimit-reset in case the primary rate limit is not reached.
func parseXRateLimitReset(resp *http.Response) *time.Time {
	secondsSinceEpoch, ok := httpHeaderIntValue(resp.Header, HeaderXRateLimitReset)
	if !ok || secondsSinceEpoch <= 0 {
		return nil
	}

	// per GitHub API, the header is set to the number of seconds since epoch (UTC)
	sleepUntil := time.Unix(secondsSinceEpoch, 0)

	return &sleepUntil
}

// httpHeaderIntValue parses an integer value from the given HTTP header.
func httpHeaderIntValue(header http.Header, key string) (int64, bool) {
	val := header.Get(key)
	if val == "" {
		return 0, false
	}
	asInt, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false
	}
	return asInt, true
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
