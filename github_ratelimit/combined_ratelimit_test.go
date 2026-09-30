package github_ratelimit

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_ratelimit_test"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_secondary_ratelimit"
)

func TestCombinedRateLimiter(t *testing.T) {
	t.Parallel()

	everySecondary := 300 * time.Millisecond
	everyPrimary := 500 * time.Millisecond
	sleepTime := 100 * time.Millisecond

	injecter := github_ratelimit_test.SetupSecondaryInjecter(t, everySecondary, sleepTime).(*github_ratelimit_test.RateLimitInjecter)
	injecter.Base = github_ratelimit_test.SetupPrimaryInjecter(t,
		everyPrimary, sleepTime,
		github_primary_ratelimit.ResourceCategoryCore,
	)

	primaryCalled := false
	secondaryCalled := false
	c := &http.Client{
		Transport: New(injecter,
			github_primary_ratelimit.WithLimitDetectedCallback(func(context *github_primary_ratelimit.CallbackContext) {
				t.Logf("primary rate limit reached!")
				primaryCalled = true
			}),
			github_secondary_ratelimit.WithLimitDetectedCallback(func(context *github_secondary_ratelimit.CallbackContext) {
				t.Logf("secondary rate limit reached!")
				secondaryCalled = true
			}),
			github_secondary_ratelimit.WithNoSleep(nil),
		),
	}

	_, err := c.Get("/initial-no-called")
	if err != nil {
		t.Fatalf("expecting first request to succeed, got %v", err)
	}
	if primaryCalled || secondaryCalled {
		t.Fatalf("expecting primary=false and secondary=false, got primary=%v, secondary=%v", primaryCalled, secondaryCalled)
	}

	// wait until secondary rate limit is triggered
	injecter.WaitForNextInjection()
	_, err = c.Get("/only-secondary")
	if primaryCalled || !secondaryCalled {
		t.Fatalf("expecting primary=false, and secondary=true, got primary=%v, secondary=%v. err: %v", primaryCalled, secondaryCalled, err)
	}

	// wait until primary rate limit is triggered
	injecter.Base.(*github_ratelimit_test.RateLimitInjecter).WaitForNextInjection()
	_, err = c.Get("/only-primary")
	if !primaryCalled {
		t.Fatalf("expecting primary=true, got primary=%v. err: %v", primaryCalled, err)
	}
}

// secondaryWithZeroRemaining answers the first request with a secondary rate
// limit that also carries `x-ratelimit-remaining: 0`, which GitHub does, and a
// reset an hour out. Everything after it succeeds.
type secondaryWithZeroRemaining struct {
	calls atomic.Int64
}

func (s *secondaryWithZeroRemaining) RoundTrip(_ *http.Request) (*http.Response, error) {
	if s.calls.Add(1) > 1 {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("some response")),
		}, nil
	}

	header := http.Header{}
	header.Set(github_secondary_ratelimit.HeaderXRateLimitRemaining, "0")
	header.Set(github_secondary_ratelimit.HeaderXRateLimitReset, strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	header.Set(github_secondary_ratelimit.HeaderRetryAfter, "1")

	body := `{"message":"You have exceeded a secondary rate limit. Please wait a few minutes before you try again.",` +
		`"documentation_url":"https://docs.github.com/en/rest/overview/resources-in-the-rest-api#secondary-rate-limits"}`

	return &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

// A secondary rate limit that carries `x-ratelimit-remaining: 0` must be handled
// as a secondary limit. Treating it as a primary one arms the whole category
// until the hourly reset, for a limit that asks to be waited out in seconds.
func TestSecondaryRateLimitWithZeroRemainingIsNotPrimary(t *testing.T) {
	t.Parallel()

	var primaryDetected atomic.Bool
	base := &secondaryWithZeroRemaining{}
	c := &http.Client{
		Transport: New(base,
			github_primary_ratelimit.WithLimitDetectedCallback(func(*github_primary_ratelimit.CallbackContext) {
				primaryDetected.Store(true)
			}),
		),
	}

	start := time.Now()
	resp, err := c.Get("/anything")
	if err != nil {
		t.Fatalf("expecting the secondary limit to be waited out, got %v", err)
	}
	defer resp.Body.Close()

	if primaryDetected.Load() {
		t.Fatal("expecting the primary limiter to leave a secondary rate limit alone")
	}
	if got := base.calls.Load(); got != 2 {
		t.Fatalf("expecting the request to be retried once, it was sent %d time(s)", got)
	}
	// It waited out `retry-after: 1` rather than the hour the reset header names.
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Fatalf("expecting a short secondary backoff, waited %v", elapsed)
	}
}
