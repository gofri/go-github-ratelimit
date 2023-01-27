package github_ratelimit

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_ratelimit_test"
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

func setupInjecter(t *testing.T, every time.Duration, sleep time.Duration) http.RoundTripper {
	options := github_ratelimit_test.SecondaryRateLimitInjecterOptions{
		Every: every,
		Sleep: sleep,
	}
	i, err := github_ratelimit_test.NewRateLimitInjecter(&nopServer{}, &options)
	if err != nil {
		t.Fatal(err)
	}

	return i
}

func waitForNextSleep(injecter http.RoundTripper) {
	i := injecter.(*github_ratelimit_test.SecondaryRateLimitInjecter)
	time.Sleep(time.Until(i.CurrentSleepEnd()))
	time.Sleep(time.Until(i.NextSleepStart()))
}

func TestSecondaryRateLimit(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	const requests = 10000
	const every = 5 * time.Second
	const sleep = 1 * time.Second

	print := func(context *CallbackContext) {
		log.Printf("Secondary rate limit reached! Sleeping for %.2f seconds [%v --> %v]",
			time.Until(*context.SleepUntil).Seconds(), time.Now(), *context.SleepUntil)
	}

	i := setupInjecter(t, every, sleep)
	c, err := NewRateLimitWaiterClient(i, WithLimitDetectedCallback(print))
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
	asInjecter, ok := i.(*github_ratelimit_test.SecondaryRateLimitInjecter)
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

func TestSingleSleepLimit(t *testing.T) {
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(*CallbackContext) {
		slept = true
	}
	exceeded := false
	onLimitExceeded := func(*CallbackContext) {
		exceeded = true
	}

	// test sleep is short enough
	i := setupInjecter(t, every, sleep)
	c, err := NewRateLimitWaiterClient(i,
		WithLimitDetectedCallback(callback),
		WithSingleSleepLimit(5*time.Second, onLimitExceeded))
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
	i = setupInjecter(t, every, sleep)
	c, err = NewRateLimitWaiterClient(i,
		WithLimitDetectedCallback(callback),
		WithSingleSleepLimit(sleep/2, onLimitExceeded))
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
	if got, want := resp.Header.Get("Retry-After"), fmt.Sprintf("%v", sleep.Seconds()); got != want {
		t.Fatal(got, want)
	}
}

func TestTotalSleepLimit(t *testing.T) {
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(*CallbackContext) {
		slept = true
	}
	exceeded := false
	onLimitExceeded := func(*CallbackContext) {
		exceeded = true
	}

	// test sleep is short enough
	i := setupInjecter(t, every, sleep)
	c, err := NewRateLimitWaiterClient(i,
		WithLimitDetectedCallback(callback),
		WithTotalSleepLimit(time.Second+time.Second/2, onLimitExceeded))
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
	if got, want := resp.Header.Get("Retry-After"), fmt.Sprintf("%v", sleep.Seconds()); got != want {
		t.Fatal(got, want)
	}
}
