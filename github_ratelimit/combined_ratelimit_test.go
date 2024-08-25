package github_ratelimit

import (
	"net/http"
	"testing"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_ratelimit_test"
	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_secondary_ratelimit"
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
