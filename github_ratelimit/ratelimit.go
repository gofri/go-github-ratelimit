package github_ratelimit

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

type SecondaryRateLimitWaiter struct {
	base       http.RoundTripper
	sleepUntil *time.Time
	lock       sync.RWMutex

	// limits
	totalSleepTime   time.Duration
	singleSleepLimit *time.Duration
	totalSleepLimit  *time.Duration

	// callbacks
	onLimitDetected       OnLimitDetected
	onSingleLimitExceeded OnSingleLimitExceeded
	onTotalLimitExceeded  OnTotalLimitExceeded
}

func NewRateLimitWaiter(base http.RoundTripper, opts ...Option) (*SecondaryRateLimitWaiter, error) {
	waiter := SecondaryRateLimitWaiter{
		base: base,
	}
	applyOptions(&waiter, opts...)

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
// after a retry-after response is received and before it it processed,
// a few other (parallel) requests may be issued.
func (t *SecondaryRateLimitWaiter) RoundTrip(request *http.Request) (*http.Response, error) {
	t.waitForRateLimit()

	resp, err := t.base.RoundTrip(request)
	if err != nil {
		return resp, err
	}

	secondaryLimit := parseSecondaryLimitTime(resp)
	if secondaryLimit == nil {
		return resp, nil
	}

	shouldRetry := t.updateRateLimit(*secondaryLimit)
	if !shouldRetry {
		return resp, nil
	}

	return t.RoundTrip(request)
}

// waitForRateLimit waits for the cooldown time to finish if a secondary rate limit is active.
func (t *SecondaryRateLimitWaiter) waitForRateLimit() {
	t.lock.RLock()
	sleepTime := t.currentSleepTimeUnlocked()
	t.lock.RUnlock()

	time.Sleep(sleepTime)
}

// updateRateLimit updates the active rate limit and prints a message to the user.
// the rate limit is not updated if there's already an active rate limit.
// it never waits because the retry handles sleeping anyway.
// returns whether or not to retry the request.
func (t *SecondaryRateLimitWaiter) updateRateLimit(secondaryLimit time.Time) bool {
	t.lock.Lock()
	defer t.lock.Unlock()

	// check before update if there is already an active rate limit
	if t.currentSleepTimeUnlocked() > 0 {
		return true
	}

	t.sleepUntil = &secondaryLimit

	// check after updating because the secondary rate limit might have passed while we waited for the lock
	sleepTime := t.currentSleepTimeUnlocked()
	if sleepTime <= 0 {
		return true
	}

	// do not sleep in case it's above the single sleep limit
	if t.singleSleepLimit != nil && sleepTime > *t.singleSleepLimit {
		if t.onSingleLimitExceeded != nil {
			t.onSingleLimitExceeded(*t.sleepUntil, t.totalSleepTime)
		}
		return false
	}

	// do not sleep in case it's above the total sleep limit
	if t.totalSleepLimit != nil && t.totalSleepTime+sleepTime > *t.totalSleepLimit {
		if t.onTotalLimitExceeded != nil {
			t.onTotalLimitExceeded(*t.sleepUntil, t.totalSleepTime)
		}
		return false
	}

	// update total time and trigger user callback
	t.totalSleepTime += sleepTime
	if t.onLimitDetected != nil {
		t.onLimitDetected(*t.sleepUntil, t.totalSleepTime)
	}

	return true
}

func (t *SecondaryRateLimitWaiter) currentSleepTimeUnlocked() time.Duration {
	if t.sleepUntil == nil {
		return 0
	}
	return time.Until(*t.sleepUntil)
}

// parseSecondaryLimitTime parses the GitHub API response header,
// looking for the secondary rate limit as defined by GitHub API documentation.
// https://docs.github.com/en/rest/overview/resources-in-the-rest-api#secondary-rate-limits
func parseSecondaryLimitTime(resp *http.Response) *time.Time {
	if resp.StatusCode != http.StatusForbidden {
		return nil
	}

	if resp.Header == nil {
		return nil
	}

	retryHeader, ok := resp.Header["Retry-After"]
	if !ok || len(retryHeader) == 0 {
		return nil
	}

	// per GitHub API, the header is set to the number of seconds to wait
	retryAfterSeconds, err := strconv.ParseInt(retryHeader[0], 10, 64)
	if err != nil {
		return nil
	}

	if retryAfterSeconds <= 0 {
		return nil
	}

	retryAfter := time.Duration(retryAfterSeconds) * time.Second
	sleepUntil := time.Now().Add(retryAfter)

	return &sleepUntil
}
