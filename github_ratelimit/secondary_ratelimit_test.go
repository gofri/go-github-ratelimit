package github_ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_ratelimit_test"
	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_secondary_ratelimit"
)

func TestSecondaryRateLimit(t *testing.T) {
	t.Parallel()
	const requests = 10000
	const every = 5 * time.Second
	const sleep = 1 * time.Second

	print := func(context *github_secondary_ratelimit.CallbackContext) {
		t.Logf("Secondary rate limit reached! Sleeping for %.2f seconds [%v --> %v]",
			time.Until(*context.ResetTime).Seconds(), time.Now(), *context.ResetTime)
	}

	i := github_ratelimit_test.SetupSecondaryInjecter(t, every, sleep)
	c := github_ratelimit_test.NewSecondaryClient(i, github_secondary_ratelimit.WithLimitDetectedCallback(print))

	var gw sync.WaitGroup
	gw.Add(requests)
	for i := 0; i < requests; i++ {
		// sleep some time between requests
		sleepTime := github_ratelimit_test.UpTo1SecDelay() / 150
		if sleepTime.Milliseconds()%2 == 0 {
			sleepTime = 0 // bias towards no-sleep for high parallelism
		}
		time.Sleep(sleepTime)

		go func() {
			defer gw.Done()
			_, _ = c.Get("/")
		}()
	}
	gw.Wait()

	// expect a low number of abuse attempts, i.e.,
	// not a lot of "slipped" messages due to race conditions.
	asInjecter, ok := i.(*github_ratelimit_test.RateLimitInjecter)
	if !ok {
		t.Fatal()
	}
	const maxAbuseAttempts = requests / 200 // 0.5% sounds good
	if real, max := asInjecter.AbuseAttempts, maxAbuseAttempts; real > max {
		t.Fatal(real, max)
	}
	abusePrecent := float64(asInjecter.AbuseAttempts) / requests * 100
	t.Logf("abuse requests: %v/%v (%v%%)\n", asInjecter.AbuseAttempts, requests, abusePrecent)
}

func TestSecondaryRateLimitCombinations(t *testing.T) {
	t.Parallel()
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	for i, docURL := range github_ratelimit_test.SecondaryRateLimitDocumentationURLs {
		docURL := docURL
		for j, statusCode := range github_secondary_ratelimit.LimitStatusCodes {
			statusCode := statusCode
			t.Run(fmt.Sprintf("docURL_%d_%d", i, j), func(t *testing.T) {
				t.Parallel()

				slept := false
				callback := func(*github_secondary_ratelimit.CallbackContext) {
					slept = true
				}

				// test documentation URL
				i := github_ratelimit_test.SetupInjecterWithOptions(t, github_ratelimit_test.RateLimitInjecterOptions{
					Every:             every,
					InjectionDuration: sleep,
					DocumentationURL:  docURL,
					HttpStatusCode:    statusCode,
				}, nil)
				c := github_ratelimit_test.NewSecondaryClient(
					i,
					github_secondary_ratelimit.WithLimitDetectedCallback(callback),
				)

				// initialize injecter timing
				_, _ = c.Get("/")
				github_ratelimit_test.WaitForNextSleep(i)

				// attempt during rate limit
				_, err := c.Get("/")
				if err != nil {
					t.Fatal(err)
				}
				if !slept {
					t.Fatal(slept)
				}
			})
		}
	}
}

func TestSingleSleepLimit(t *testing.T) {
	t.Parallel()
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(*github_secondary_ratelimit.CallbackContext) {
		slept = true
	}
	exceeded := false
	onLimitExceeded := func(*github_secondary_ratelimit.CallbackContext) {
		exceeded = true
	}

	// test sleep is short enough
	i := github_ratelimit_test.SetupSecondaryInjecter(t, every, sleep)
	c := github_ratelimit_test.NewSecondaryClient(i,
		github_secondary_ratelimit.WithLimitDetectedCallback(callback),
		github_secondary_ratelimit.WithSingleSleepLimit(5*time.Second, onLimitExceeded))

	// initialize injecter timing
	_, _ = c.Get("/")
	github_ratelimit_test.WaitForNextSleep(i)

	// attempt during rate limit
	_, err := c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if !slept || exceeded {
		t.Fatal(slept, exceeded)
	}

	// test sleep is too long
	slept = false
	i = github_ratelimit_test.SetupSecondaryInjecter(t, every, sleep)
	c = github_ratelimit_test.NewSecondaryClient(i,
		github_secondary_ratelimit.WithLimitDetectedCallback(callback),
		github_secondary_ratelimit.WithSingleSleepLimit(sleep/2, onLimitExceeded))

	// initialize injecter timing
	_, _ = c.Get("/")
	github_ratelimit_test.WaitForNextSleep(i)

	// attempt during rate limit
	resp, err := c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if slept || !exceeded {
		t.Fatal(err)
	}
	if got, want := resp.Header.Get(github_secondary_ratelimit.HeaderRetryAfter), fmt.Sprintf("%v", sleep.Seconds()); got != want {
		t.Fatal(got, want)
	}
	// try again - make sure that triggering the callback does not cause it to sleep next time
	tBefore := time.Now()
	_, err = c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	tAfter := time.Now()
	// choose sleep 2 arbitrarily - should be much less (but should be close to almost sleep if error)
	if got, limit := tAfter.Sub(tBefore), sleep/2; got >= limit {
		t.Fatal(got, limit)
	}
}

func TestTotalSleepLimit(t *testing.T) {
	t.Parallel()
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(*github_secondary_ratelimit.CallbackContext) {
		slept = true
	}
	exceeded := false
	onLimitExceeded := func(*github_secondary_ratelimit.CallbackContext) {
		exceeded = true
	}

	// test sleep is short enough
	i := github_ratelimit_test.SetupSecondaryInjecter(t, every, sleep)
	c := github_ratelimit_test.NewSecondaryClient(i,
		github_secondary_ratelimit.WithLimitDetectedCallback(callback),
		github_secondary_ratelimit.WithTotalSleepLimit(time.Second+time.Second/2, onLimitExceeded))

	// initialize injecter timing
	_, _ = c.Get("/")
	github_ratelimit_test.WaitForNextSleep(i)

	// attempt during rate limit - sleep is still short enough
	_, err := c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if !slept || exceeded {
		t.Fatal(slept, exceeded)
	}

	// test sleep is too long
	slept = false
	github_ratelimit_test.WaitForNextSleep(i)
	resp, err := c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if slept || !exceeded {
		t.Fatal(slept, exceeded)
	}
	if got, want := resp.Header.Get(github_secondary_ratelimit.HeaderRetryAfter), fmt.Sprintf("%v", sleep.Seconds()); got != want {
		t.Fatal(got, want)
	}
	// try again - make sure that triggering the callback does not cause it to sleep next time
	tBefore := time.Now()
	_, err = c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	tAfter := time.Now()
	// choose sleep 2 arbitrarily - should be much less (but should be close to almost sleep if error)
	if got, limit := tAfter.Sub(tBefore), sleep/2; got >= limit {
		t.Fatal(got, limit)
	}
}

func TestXRateLimit(t *testing.T) {
	t.Parallel()
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(*github_secondary_ratelimit.CallbackContext) {
		slept = true
	}

	// test sleep is short enough
	i := github_ratelimit_test.SetupInjecterWithOptions(t, github_ratelimit_test.RateLimitInjecterOptions{
		Every:             every,
		InjectionDuration: sleep,
		UseXRateLimit:     true,
	}, nil)
	c := github_ratelimit_test.NewSecondaryClient(i, github_secondary_ratelimit.WithLimitDetectedCallback(callback))

	// initialize injecter timing
	_, _ = c.Get("/")
	github_ratelimit_test.WaitForNextSleep(i)

	// attempt during rate limit
	_, err := c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if !slept {
		t.Fatal(slept)
	}
}

func TestPrimaryRateLimitIgnored(t *testing.T) {
	t.Parallel()
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(*github_secondary_ratelimit.CallbackContext) {
		slept = true
	}

	// test sleep is short enough
	i := github_ratelimit_test.SetupPrimaryInjecter(t, every, sleep, getRandomCategory())
	c := github_ratelimit_test.NewSecondaryClient(i, github_secondary_ratelimit.WithLimitDetectedCallback(callback))

	// initialize injecter timing
	_, _ = c.Get("/")
	github_ratelimit_test.WaitForNextSleep(i)

	// attempt during rate limit
	_, err := c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if slept {
		t.Fatal(slept)
	}
}

func TestHTTPForbiddenIgnored(t *testing.T) {
	t.Parallel()
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(*github_secondary_ratelimit.CallbackContext) {
		slept = true
	}

	// test sleep is short enough
	i := github_ratelimit_test.SetupInjecterWithOptions(t, github_ratelimit_test.RateLimitInjecterOptions{
		Every:             every,
		InjectionDuration: sleep,
		InvalidBody:       true,
	}, nil)

	c := github_ratelimit_test.NewSecondaryClient(i, github_secondary_ratelimit.WithLimitDetectedCallback(callback))

	// initialize injecter timing
	_, _ = c.Get("/")
	github_ratelimit_test.WaitForNextSleep(i)

	// attempt during rate limit (using invalid body, so the injection is of HTTP Forbidden)
	resp, err := c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if slept {
		t.Fatal(slept)
	}

	if invalidBody, err := github_ratelimit_test.IsInvalidBody(resp); err != nil {
		t.Fatal(err)
	} else if !invalidBody {
		t.Fatalf("expected invalid body")
	}
}

func TestCallbackContext(t *testing.T) {
	t.Parallel()
	const every = 1 * time.Second
	const sleep = 1 * time.Second
	i := github_ratelimit_test.SetupSecondaryInjecter(t, every, sleep)

	var roundTripper http.RoundTripper = nil
	var requestNum atomic.Int64
	requestNum.Add(1)
	requestsCycle := 1

	callback := func(ctx *github_secondary_ratelimit.CallbackContext) {
		if got, want := ctx.RoundTripper, roundTripper; got != want {
			t.Fatalf("roundtripper mismatch: %v != %v", got, want)
		}
		if ctx.Request == nil || ctx.Response == nil {
			t.Fatalf("missing request / response: %v / %v:", ctx.Request, ctx.Response)
		}
		if got, min, max := time.Until(*ctx.ResetTime), time.Duration(0), sleep*time.Duration(requestNum.Load()); got <= min || got > max {
			t.Fatalf("unexpected sleep until time: %v < %v <= %v", min, got, max)
		}
		if got, want := *ctx.TotalSleepTime, sleep*time.Duration(requestsCycle); got != want {
			t.Fatalf("unexpected total sleep duration: %v != %v", got, want)
		}
		requestNum.Add(1)
	}

	c := github_ratelimit_test.NewSecondaryClient(i,
		github_secondary_ratelimit.WithLimitDetectedCallback(callback),
	)
	roundTripper = c.Transport

	// initialize injecter timing
	_, _ = c.Get("/")
	github_ratelimit_test.WaitForNextSleep(i)

	// attempt during rate limit
	_, err := c.Get("/")
	if err != nil {
		t.Fatal(err)
	}

	github_ratelimit_test.WaitForNextSleep(i)
	requestsCycle++
	errChan := make(chan error)
	parallelReqs := 10
	for index := 0; index < parallelReqs; index++ {
		go func() {
			_, err := c.Get("/")
			errChan <- err
		}()
	}
	for index := 0; index < parallelReqs; index++ {
		if err := <-errChan; err != nil {
			t.Fatal(err)
		}
	}
	close(errChan)
}

func TestRequestConfigOverride(t *testing.T) {
	t.Parallel()
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(*github_secondary_ratelimit.CallbackContext) {
		slept = true
	}
	exceeded := false
	onLimitExceeded := func(*github_secondary_ratelimit.CallbackContext) {
		exceeded = true
	}

	// test sleep is short enough
	i := github_ratelimit_test.SetupSecondaryInjecter(t, every, sleep)
	c := github_ratelimit_test.NewSecondaryClient(i,
		github_secondary_ratelimit.WithLimitDetectedCallback(callback),
		github_secondary_ratelimit.WithSingleSleepLimit(5*time.Second, onLimitExceeded))

	// initialize injecter timing
	_, _ = c.Get("/")

	// prepare an override - force sleep duration to be 0,
	// so that it will not sleep at all regardless of the original config.
	limit := github_secondary_ratelimit.WithSingleSleepLimit(0, onLimitExceeded)
	ctx := github_secondary_ratelimit.WithOverrideConfig(context.Background(), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// wait for next sleep to kick in, but issue the request with the override
	github_ratelimit_test.WaitForNextSleep(i)

	// attempt during rate limit
	slept = false
	exceeded = false
	_, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	// expect no sleep because the override is set to 0
	if slept || !exceeded {
		t.Fatal(slept, exceeded)
	}

	// prepare an override with a different nature (extra safety check)
	exceeded = false
	usedAltCallback := false
	onSleepAlt := func(*github_secondary_ratelimit.CallbackContext) {
		usedAltCallback = true
	}

	limit = github_secondary_ratelimit.WithSingleSleepLimit(10*time.Second, onLimitExceeded)
	sleepCB := github_secondary_ratelimit.WithLimitDetectedCallback(onSleepAlt)
	ctx = github_secondary_ratelimit.WithOverrideConfig(context.Background(), limit, sleepCB)
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// attempt during rate limit
	_, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if !usedAltCallback || exceeded {
		t.Fatal(slept, exceeded)
	}

}
