package github_ratelimit_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SecondaryRateLimitInjecter struct {
	base          http.RoundTripper
	options       *SecondaryRateLimitInjecterOptions
	blockUntil    time.Time
	lock          sync.Mutex
	AbuseAttempts int
}

const (
	RateLimitInjecterEvery    = "SECONDARY_RATE_LIMIT_INJECTER_EVERY"
	RateLimitInjecterDuration = "SECONDARY_RATE_LIMIT_INJECTER_DURATION"
)

type SecondaryRateLimitInjecterOptions struct {
	Every time.Duration
	Sleep time.Duration
}

func NewRateLimitInjecter(base http.RoundTripper, options *SecondaryRateLimitInjecterOptions) (http.RoundTripper, error) {
	if options.IsNoop() {
		return base, nil
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}

	injecter := &SecondaryRateLimitInjecter{
		base:    base,
		options: options,
	}
	return injecter, nil
}

func (t *SecondaryRateLimitInjecter) RoundTrip(request *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(request)
	if err != nil {
		return resp, err
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	// initialize on first use
	now := time.Now()
	if t.blockUntil.IsZero() {
		t.blockUntil = now
	}

	// on-going rate limit
	if t.blockUntil.After(now) {
		t.AbuseAttempts++
		return t.toRetryResponse(resp), nil
	}

	nextStart := t.NextSleepStart()

	// start a rate limit period
	if !now.Before(nextStart) {
		t.blockUntil = nextStart.Add(t.options.Sleep)
		return t.toRetryResponse(resp), nil
	}

	return resp, nil
}

func (r *SecondaryRateLimitInjecterOptions) IsNoop() bool {
	return r.Every == 0 || r.Sleep == 0
}

func (r *SecondaryRateLimitInjecterOptions) Validate() error {
	if r.Every < 0 {
		return fmt.Errorf("injecter expects a positive trigger interval")
	}
	if r.Sleep < 0 {
		return fmt.Errorf("injecter expects a positive sleep interval")
	}
	return nil
}
func (r *SecondaryRateLimitInjecter) CurrentSleepEnd() time.Time {
	return r.blockUntil
}

func (r *SecondaryRateLimitInjecter) NextSleepStart() time.Time {
	return r.blockUntil.Add(r.options.Every)
}

func (t *SecondaryRateLimitInjecter) toRetryResponse(resp *http.Response) *http.Response {
	resp.StatusCode = http.StatusForbidden
	timeUntil := time.Until(t.blockUntil)
	if timeUntil.Nanoseconds()%int64(time.Second) > 0 {
		timeUntil += time.Second
	}
	resp.Header.Set("Retry-After", fmt.Sprintf("%v", int(timeUntil.Seconds())))
	doc_url := "https://docs.github.com/en/rest/guides/best-practices-for-integrators?apiVersion=2022-11-28#secondary-rate-limits"
	resp.Body = io.NopCloser(strings.NewReader(`{"documentation_url":"` + doc_url + `"}`))
	return resp
}
