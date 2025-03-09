package github_ratelimit

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_ratelimit_test"
)

var AllCategories = github_primary_ratelimit.GetAllCategories()

func getRandomCategory() github_primary_ratelimit.ResourceCategory {
	return AllCategories[rand.IntN(len(AllCategories))]
}

func TestPrimaryRateLimit(t *testing.T) {
	t.Parallel()
	for _, determenistic := range []bool{true, false} {
		t.Logf("primary with determenistic = %v", determenistic)
		const requests = 1000
		const every = 1 * time.Second
		const sleep = 99999999 * time.Second

		var preventionCount atomic.Bool
		countPrevented := func(ctx *github_primary_ratelimit.CallbackContext) {
			preventionCount.Store(true)
		}

		print := func(context *github_primary_ratelimit.CallbackContext) {
			t.Logf("primary rate limit reached! %v", *context.ResetTime)
		}

		category := github_primary_ratelimit.ResourceCategoryCore
		i := github_ratelimit_test.SetupPrimaryInjecter(t, every, sleep, category)
		c := github_ratelimit_test.NewPrimaryClient(i,
			github_primary_ratelimit.WithLimitDetectedCallback(print),
			github_primary_ratelimit.WithRequestPreventedCallback(countPrevented),
		)

		_, err := c.Get("/trigger-core-category")
		if err != nil {
			t.Fatalf("expecting first request to succeed, got %v", err)
		}

		github_ratelimit_test.WaitForNextSleep(i)
		if determenistic {
			_, err = c.Get("/trigger-core-category")
			if err == nil {
				t.Fatalf("expecting second request to fail, got %v", err)
			}
		}

		var gw sync.WaitGroup
		gw.Add(requests)
		var maxAbuseAttempts = 10
		if determenistic {
			maxAbuseAttempts = 0
		}
		tooQuickSuccesses := 0
		for i := 0; i < requests; i++ {
			// sleep some time between requests
			sleepTime := github_ratelimit_test.UpTo1SecDelay() / 150
			if sleepTime.Milliseconds()%2 == 0 {
				sleepTime = 0 // bias towards no-sleep for high parallelism
			}
			time.Sleep(sleepTime)

			go func(i int) {
				defer gw.Done()
				_, err := c.Get("/trigger-core-category")
				if err == nil {
					tooQuickSuccesses += 1
					if tooQuickSuccesses > maxAbuseAttempts {
						// TODO channel for errors
						t.Logf("expecting error, got nil")
					}
				}
			}(i)
		}
		gw.Wait()

		// expect a low number of abuse attempts, i.e.,
		// not a lot of "slipped" messages due to race conditions.
		asInjecter, ok := i.(*github_ratelimit_test.RateLimitInjecter)
		if !ok {
			t.Fatal()
		}
		if real, max := asInjecter.AbuseAttempts, maxAbuseAttempts; real > max {
			t.Fatalf("got %v abusing requests, expected up to %v", real, max)
		}
		abuseAttempts := asInjecter.AbuseAttempts
		abusePrecent := float64(abuseAttempts) / requests * 100
		t.Logf("abuse requests: %v/%v (%v%%)\n", asInjecter.AbuseAttempts, requests, abusePrecent)

		if !preventionCount.Load() {
			t.Fatal("no prevention was happening")
		}
	}
}

func getChannelErrorNonblocking(errChan chan error) (error, bool) {
	select {
	case err := <-errChan:
		return err, true
	default:
		return nil, false
	}
}

func TestPrimaryCallbacks(t *testing.T) {
	t.Parallel()

	const (
		validCategory   = github_primary_ratelimit.ResourceCategoryCore
		invalidCategory = github_primary_ratelimit.ResourceCategory("rubbish")
		sleep           = 1 * time.Second
	)

	t.Run("limit and prevention", func(t *testing.T) {
		limitChan := make(chan error, 1)
		limitCB := func(ctx *github_primary_ratelimit.CallbackContext) {
			defer close(limitChan)
			if ctx.Request == nil || ctx.Response == nil || ctx.ResetTime == nil {
				limitChan <- fmt.Errorf("expected all fields, got %p, %p, %p",
					ctx.Request, ctx.Response, ctx.ResetTime)
				return
			}
			if ctx.Category != validCategory {
				limitChan <- fmt.Errorf("expected category %v, got %v", validCategory, ctx.Category)
				return
			}
			limitChan <- nil
		}

		preventionChan := make(chan error, 1)
		preventionCB := func(ctx *github_primary_ratelimit.CallbackContext) {
			defer close(preventionChan)
			if ctx.Request == nil || ctx.Response == nil || ctx.ResetTime == nil {
				preventionChan <- fmt.Errorf("expected all fields, got %p, %p, %p",
					ctx.Request, ctx.Response, ctx.ResetTime)
				return
			}
			if ctx.Category != validCategory {
				preventionChan <- fmt.Errorf("expected category %v, got %v", validCategory, ctx.Category)
				return
			}
			if remaining := github_primary_ratelimit.ResponseHeaderKeyRemaining.Get(ctx.Response); remaining != "0" {
				preventionChan <- fmt.Errorf("expected remaining 0, got %v", remaining)
				return
			}
			preventionChan <- nil
		}

		resetChan := make(chan error, 1)
		resetCB := func(ctx *github_primary_ratelimit.CallbackContext) {
			defer close(resetChan)
			if ctx.Request != nil || ctx.Response != nil {
				resetChan <- fmt.Errorf("expected empty request & response, got %v %v",
					ctx.Request, ctx.Response)
				return
			}
			if ctx.ResetTime == nil {
				resetChan <- fmt.Errorf("expected reset time, got nil")
				return
			}
			if ctx.Category != validCategory {
				resetChan <- fmt.Errorf("expected category %v, got %v", validCategory, ctx.Category)
				return
			}
			resetChan <- nil
		}

		every := 500 * time.Millisecond
		i := github_ratelimit_test.SetupPrimaryInjecter(t, every, sleep, validCategory)
		c := github_ratelimit_test.NewPrimaryClient(i,
			github_primary_ratelimit.WithLimitDetectedCallback(limitCB),
			github_primary_ratelimit.WithRequestPreventedCallback(preventionCB),
			github_primary_ratelimit.WithLimitResetCallback(resetCB),
		)

		// initiate to wait for first
		_, _ = c.Get("/")
		time.Sleep(every)

		// limited request
		resp, err := c.Get("/trigger-limit")
		if err == nil {
			t.Fatalf("expecting limit, got nil: %v", resp)
		}
		err, ok := getChannelErrorNonblocking(limitChan)
		if !ok || err != nil {
			t.Fatalf("limit check failed: %v, %v", ok, err)
		}
		err, ok = getChannelErrorNonblocking(preventionChan)
		if ok || err != nil {
			t.Fatalf("not expected prevention, got: %v, %v", ok, err)
		}

		// prevented request
		resp, err = c.Get("/trigger-prevent")
		if err == nil {
			t.Fatalf("expecting request to fail, got nil: %v", resp)
		}
		var typedErr *github_primary_ratelimit.RateLimitReachedError
		if ok := errors.As(err, &typedErr); !ok {
			t.Fatalf("unexpected error type: %T - %v", err, err)
		}
		resetTime := *typedErr.ResetTime
		err, ok = getChannelErrorNonblocking(preventionChan)
		if !ok || err != nil {
			t.Fatalf("prevention check failed: %v, %v", ok, err)
		}

		// reset check
		err, ok = getChannelErrorNonblocking(resetChan)
		if ok || err != nil {
			t.Fatalf("not expected reset, got: %v, %v", ok, err)
		}

		extraTime := 500 * time.Millisecond // make "sure" the timer goes first
		time.Sleep(time.Until(resetTime) + extraTime)
		err, ok = getChannelErrorNonblocking(resetChan)
		if !ok || err != nil {
			t.Fatalf("reset check failed: %v, %v", ok, err)
		}
	})

	t.Run("unknown category", func(t *testing.T) {
		unknownChan := make(chan error, 1)
		unknownCB := func(ctx *github_primary_ratelimit.CallbackContext) {
			defer close(unknownChan)
			if ctx.Request == nil || ctx.Response == nil || ctx.ResetTime == nil {
				unknownChan <- fmt.Errorf("expected all fields, got %p, %p, %p",
					ctx.Request, ctx.Response, ctx.ResetTime)
				return
			}
			if ctx.Category != invalidCategory {
				unknownChan <- fmt.Errorf("expected category %v, got %v", invalidCategory, ctx.Category)
				return
			}
			unknownChan <- nil
		}

		every := 0 * time.Second
		i := github_ratelimit_test.SetupPrimaryInjecter(t, every, sleep, invalidCategory)
		c := github_ratelimit_test.NewPrimaryClient(i,
			github_primary_ratelimit.WithUnknownCategoryCallback(unknownCB),
		)

		_, err := c.Get("/")
		if err != nil {
			t.Fatalf("expecting first request to succeed, got: %v", err)
		}
		err, ok := getChannelErrorNonblocking(unknownChan)
		if ok || err != nil {
			t.Fatalf("unknown category check failed: %v, %v", ok, err)
		}

		_, err = c.Get("/")
		if err != nil {
			t.Fatalf("expecting second request to succeed, got: %v", err)
		}
		err, ok = getChannelErrorNonblocking(unknownChan)
		if !ok || err != nil {
			t.Fatalf("unknown category check failed: %v, %v", ok, err)
		}
	})
}

func TestConfigOverride(t *testing.T) {
	const (
		category = github_primary_ratelimit.ResourceCategoryCore
		sleep    = 1 * time.Second
		every    = 1 * time.Second
	)

	var preOverride, postOverride atomic.Bool

	defaultCB := func(ctx *github_primary_ratelimit.CallbackContext) {
		preOverride.Store(true)
	}

	overrideCB := func(ctx *github_primary_ratelimit.CallbackContext) {
		postOverride.Store(true)
	}

	i := github_ratelimit_test.SetupPrimaryInjecter(t, every, sleep, category)
	c := github_ratelimit_test.NewPrimaryClient(i,
		github_primary_ratelimit.WithLimitDetectedCallback(defaultCB),
	)

	// initiate to wait for first
	_, _ = c.Get("/")
	time.Sleep(every)

	// request with overridden callback
	req, _ := http.NewRequest(http.MethodGet, "/trigger-limit", nil)
	req = req.WithContext(
		github_primary_ratelimit.WithOverrideConfig(
			req.Context(),
			github_primary_ratelimit.WithLimitDetectedCallback(overrideCB),
		),
	)
	_, err := c.Do(req)
	if err == nil {
		t.Fatalf("expecting limit, got nil")
	}

	if !postOverride.Load() {
		t.Fatal("override callback was not called")
	}
	if preOverride.Load() {
		t.Fatal("default callback was called instead of override")
	}
}
