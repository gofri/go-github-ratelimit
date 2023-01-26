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
	"github.com/stretchr/testify/require"
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

func callback(until time.Time, totalSleepTime time.Duration) {
	log.Printf("Secondary rate limit reached! Sleeping for %.2f seconds [%v --> %v]",
		time.Until(until).Seconds(), time.Now(), until)
}

func setupInjecter(t *testing.T, every time.Duration, sleep time.Duration) http.RoundTripper {
	options := github_ratelimit_test.SecondaryRateLimitInjecterOptions{
		Every: every,
		Sleep: sleep,
	}
	i, err := github_ratelimit_test.NewRateLimitInjecter(&nopServer{}, &options)
	require.Nil(t, err)

	return i
}

func waitForNextSleep(injecter http.RoundTripper) {
	i := injecter.(*github_ratelimit_test.SecondaryRateLimitInjecter)
	time.Sleep(time.Until(i.CurrentSleepEnd()))
	time.Sleep(time.Until(i.NextSleepStart()))
}

func TestSecondaryRateLimit(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	const requests = 5000
	const every = 3 * time.Second
	const sleep = 1 * time.Second

	i := setupInjecter(t, every, sleep)
	c, err := NewRateLimitWaiterClient(i, WithLimitDetectedCallback(callback))
	require.Nil(t, err)

	var gw sync.WaitGroup
	gw.Add(requests)
	for i := 0; i < requests; i++ {
		// sleep some between requests
		sleepTime := upTo1SecDelay() / 100
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
	require.True(t, ok)
	const maxAbuseAttempts = 10
	require.LessOrEqual(t, asInjecter.AbuseAttempts, maxAbuseAttempts)
}

func TestSingleSleepLimit(t *testing.T) {
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(time.Time, time.Duration) {
		slept = true
	}
	exceeded := false
	onLimitExceeded := func(time.Time, time.Duration) {
		exceeded = true
	}

	// test sleep is short enough
	i := setupInjecter(t, every, sleep)
	c, err := NewRateLimitWaiterClient(i,
		WithLimitDetectedCallback(callback),
		WithSingleSleepLimit(5*time.Second, onLimitExceeded))
	require.Nil(t, err)

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit
	_, err = c.Get("/")
	require.Nil(t, err)
	require.True(t, slept)
	require.False(t, exceeded)

	// test sleep is too long
	slept = false
	i = setupInjecter(t, every, sleep)
	c, err = NewRateLimitWaiterClient(i,
		WithLimitDetectedCallback(callback),
		WithSingleSleepLimit(sleep/2, onLimitExceeded))
	require.Nil(t, err)

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit
	resp, err := c.Get("/")
	require.Nil(t, err)
	require.False(t, slept) // expect not to sleep because it is beyond the limit
	require.Equal(t, fmt.Sprintf("%v", sleep.Seconds()), resp.Header.Get("Retry-After"))
	require.True(t, exceeded)
}

func TestTotalSleepLimit(t *testing.T) {
	const every = 1 * time.Second
	const sleep = 1 * time.Second

	slept := false
	callback := func(_ time.Time, total time.Duration) {
		slept = true
	}
	exceeded := false
	onLimitExceeded := func(time.Time, time.Duration) {
		exceeded = true
	}

	// test sleep is short enough
	i := setupInjecter(t, every, sleep)
	c, err := NewRateLimitWaiterClient(i,
		WithLimitDetectedCallback(callback),
		WithTotalSleepLimit(time.Second+time.Second/2, onLimitExceeded))
	require.Nil(t, err)

	// initialize injecter timing
	_, _ = c.Get("/")
	waitForNextSleep(i)

	// attempt during rate limit - sleep is still short enough
	_, err = c.Get("/")
	require.Nil(t, err)
	require.True(t, slept)
	require.False(t, exceeded)

	// test sleep is too long
	slept = false
	waitForNextSleep(i)
	resp, err := c.Get("/")
	require.Nil(t, err)
	require.False(t, slept) // expect not to sleep because it is beyond the limit now
	require.True(t, exceeded)
	require.Equal(t, fmt.Sprintf("%v", sleep.Seconds()), resp.Header.Get("Retry-After"))
}
