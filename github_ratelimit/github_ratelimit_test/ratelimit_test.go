package github_ratelimit_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit"
)

type nopServer struct {
}

func upTo1SecDelay() time.Duration {
	return time.Duration(int(time.Millisecond) * (rand.Int() % 1000))
}

func (n *nopServer) RoundTrip(r *http.Request) (*http.Response, error) {
	time.Sleep(upTo1SecDelay() / 100)
	return &http.Response{
		Body:   io.NopCloser(strings.NewReader("some response")),
		Header: http.Header{},
	}, nil
}

func setupSecondaryLimitInjecter(t *testing.T, every time.Duration, sleep time.Duration) http.RoundTripper {
	options := SecondaryRateLimitInjecterOptions{
		Every: every,
		Sleep: sleep,
	}
	return setupInjecterWithOptions(t, options)
}

func setupInjecterWithOptions(t *testing.T, options SecondaryRateLimitInjecterOptions) http.RoundTripper {
	i, err := NewRateLimitInjecter(&nopServer{}, &options)
	if err != nil {
		t.Fatal(err)
	}

	return i
}

func waitForNextSleep(injecter http.RoundTripper) {
	i := injecter.(*SecondaryRateLimitInjecter)
	time.Sleep(time.Until(i.CurrentSleepEnd()))
	time.Sleep(time.Until(i.NextSleepStart()))
}

func TestSecondaryRateLimit(t *testing.T) {
	t.Parallel()
	rand.Seed(time.Now().UnixNano())
	const requests = 10000
	const every = 5 * time.Second
	const sleep = 1 * time.Second

	print := func(context *github_ratelimit.CallbackContext) {
		log.Printf("Secondary rate limit reached! Sleeping for %.2f seconds [%v --> %v]",
			time.Until(*context.SleepUntil).Seconds(), time.Now(), *context.SleepUntil)
	}

	i := setupSecondaryLimitInjecter(t, every, sleep)
	c, err := github_ratelimit.NewRateLimitWaiterClient(i, github_ratelimit.WithLimitDetectedCallback(print))
	if err != nil {
		t.Fatal(err)
	}

	var gw sync.WaitGroup
	gw.Add(requests)
	for i := 0; i < requests; i++ {
		// sleep some time between requests
		sleepTime := upTo1SecDelay() / 150
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
	asInjecter, ok := i.(*SecondaryRateLimitInjecter)
	if !ok {
		t.Fatal()
	}
	const maxAbuseAttempts = requests / 200 // 0.5% sounds good
	if real, max := asInjecter.AbuseAttempts, maxAbuseAttempts; real > max {
		t.Fatal(real, max)
	}
	abusePrecent := float64(asInjecter.AbuseAttempts) / requests * 100
	log.Printf("abuse requests: %v/%v (%v%%)\n", asInjecter.AbuseAttempts, requests, abusePrecent)
}

func TestSecondaryRateLimitBody(t *testing.T) {
	t.Parallel()
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(*github_ratelimit.CallbackContext) {
		slept = true
	}

	// test documentation URL
	i := setupInjecterWithOptions(t, SecondaryRateLimitInjecterOptions{
		Every:                        every,
		Sleep:                        sleep,
		UseAlternateDocumentationURL: false,
	})
	c, err := github_ratelimit.NewRateLimitWaiterClient(i, github_ratelimit.WithLimitDetectedCallback(callback))
	if err != nil {
		t.Fatal(err)
	}

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit
	_, err = c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if !slept {
		t.Fatal(slept)
	}

	// test alternate documentation URL
	slept = false
	i = setupInjecterWithOptions(t, SecondaryRateLimitInjecterOptions{
		Every:                        every,
		Sleep:                        sleep,
		UseAlternateDocumentationURL: true,
	})
	c, err = github_ratelimit.NewRateLimitWaiterClient(i, github_ratelimit.WithLimitDetectedCallback(callback))
	if err != nil {
		t.Fatal(err)
	}

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit
	_, err = c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if !slept {
		t.Fatal(slept)
	}
}

func TestSingleSleepLimit(t *testing.T) {
	t.Parallel()
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(*github_ratelimit.CallbackContext) {
		slept = true
	}
	exceeded := false
	onLimitExceeded := func(*github_ratelimit.CallbackContext) {
		exceeded = true
	}

	// test sleep is short enough
	i := setupSecondaryLimitInjecter(t, every, sleep)
	c, err := github_ratelimit.NewRateLimitWaiterClient(i,
		github_ratelimit.WithLimitDetectedCallback(callback),
		github_ratelimit.WithSingleSleepLimit(5*time.Second, onLimitExceeded))
	if err != nil {
		t.Fatal(err)
	}

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit
	_, err = c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if !slept || exceeded {
		t.Fatal(slept, exceeded)
	}

	// test sleep is too long
	slept = false
	i = setupSecondaryLimitInjecter(t, every, sleep)
	c, err = github_ratelimit.NewRateLimitWaiterClient(i,
		github_ratelimit.WithLimitDetectedCallback(callback),
		github_ratelimit.WithSingleSleepLimit(sleep/2, onLimitExceeded))
	if err != nil {
		t.Fatal(err)
	}

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit
	resp, err := c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if slept || !exceeded {
		t.Fatal(err)
	}
	if got, want := resp.Header.Get(github_ratelimit.HeaderRetryAfter), fmt.Sprintf("%v", sleep.Seconds()); got != want {
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
	callback := func(*github_ratelimit.CallbackContext) {
		slept = true
	}
	exceeded := false
	onLimitExceeded := func(*github_ratelimit.CallbackContext) {
		exceeded = true
	}

	// test sleep is short enough
	i := setupSecondaryLimitInjecter(t, every, sleep)
	c, err := github_ratelimit.NewRateLimitWaiterClient(i,
		github_ratelimit.WithLimitDetectedCallback(callback),
		github_ratelimit.WithTotalSleepLimit(time.Second+time.Second/2, onLimitExceeded))
	if err != nil {
		t.Fatal(err)
	}

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit - sleep is still short enough
	_, err = c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if !slept || exceeded {
		t.Fatal(slept, exceeded)
	}

	// test sleep is too long
	slept = false
	waitForNextSleep(i)
	resp, err := c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if slept || !exceeded {
		t.Fatal(slept, exceeded)
	}
	if got, want := resp.Header.Get(github_ratelimit.HeaderRetryAfter), fmt.Sprintf("%v", sleep.Seconds()); got != want {
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
	callback := func(*github_ratelimit.CallbackContext) {
		slept = true
	}

	// test sleep is short enough
	i := setupInjecterWithOptions(t, SecondaryRateLimitInjecterOptions{
		Every:         every,
		Sleep:         sleep,
		UseXRateLimit: true,
	})
	c, err := github_ratelimit.NewRateLimitWaiterClient(i, github_ratelimit.WithLimitDetectedCallback(callback))
	if err != nil {
		t.Fatal(err)
	}

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit
	_, err = c.Get("/")
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
	callback := func(*github_ratelimit.CallbackContext) {
		slept = true
	}

	// test sleep is short enough
	i := setupInjecterWithOptions(t, SecondaryRateLimitInjecterOptions{
		Every:               every,
		Sleep:               sleep,
		UsePrimaryRateLimit: true,
	})
	c, err := github_ratelimit.NewRateLimitWaiterClient(i, github_ratelimit.WithLimitDetectedCallback(callback))
	if err != nil {
		t.Fatal(err)
	}

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit
	_, err = c.Get("/")
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
	callback := func(*github_ratelimit.CallbackContext) {
		slept = true
	}

	// test sleep is short enough
	i := setupInjecterWithOptions(t, SecondaryRateLimitInjecterOptions{
		Every:       every,
		Sleep:       sleep,
		InvalidBody: true,
	})

	c, err := github_ratelimit.NewRateLimitWaiterClient(i, github_ratelimit.WithLimitDetectedCallback(callback))
	if err != nil {
		t.Fatal(err)
	}

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit (using invalid body, so the injection is of HTTP Forbidden)
	resp, err := c.Get("/")
	if err != nil {
		t.Fatal(err)
	}
	if slept {
		t.Fatal(slept)
	}

	if invalidBody, err := IsInvalidBody(resp); err != nil {
		t.Fatal(err)
	} else if !invalidBody {
		t.Fatalf("expected invalid body")
	}
}

func TestCallbackContext(t *testing.T) {
	t.Parallel()
	const every = 1 * time.Second
	const sleep = 1 * time.Second
	i := setupSecondaryLimitInjecter(t, every, sleep)

	ctxKey := struct{}{}
	ctxVal := 10
	userContext := context.WithValue(context.Background(), ctxKey, ctxVal)
	var roundTripper *github_ratelimit.SecondaryRateLimitWaiter = nil
	var requestNum atomic.Int64
	requestNum.Add(1)
	requestsCycle := 1

	callback := func(ctx *github_ratelimit.CallbackContext) {
		val := (*ctx.UserContext).Value(ctxKey).(int)
		if val != ctxVal {
			t.Fatalf("user ctx mismatch: %v != %v", val, ctxVal)
		}
		if got, want := ctx.RoundTripper, roundTripper; got != want {
			t.Fatalf("roundtripper mismatch: %v != %v", got, want)
		}
		if ctx.Request == nil || ctx.Response == nil {
			t.Fatalf("missing request / response: %v / %v:", ctx.Request, ctx.Response)
		}
		if got, min, max := time.Until(*ctx.SleepUntil), time.Duration(0), sleep*time.Duration(requestNum.Load()); got <= min || got > max {
			t.Fatalf("unexpected sleep until time: %v < %v <= %v", min, got, max)
		}
		if got, want := *ctx.TotalSleepTime, sleep*time.Duration(requestsCycle); got != want {
			t.Fatalf("unexpected total sleep time: %v != %v", got, want)
		}
		requestNum.Add(1)
	}

	r, err := github_ratelimit.NewRateLimitWaiter(i,
		github_ratelimit.WithUserContext(userContext),
		github_ratelimit.WithLimitDetectedCallback(callback),
	)
	if err != nil {
		t.Fatal(err)
	}
	roundTripper = r
	c := &http.Client{
		Transport: r,
	}

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit
	_, err = c.Get("/")
	if err != nil {
		t.Fatal(err)
	}

	waitForNextSleep(i)
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
