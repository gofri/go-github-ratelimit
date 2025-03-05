package github_ratelimit_test

import (
	"io"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_secondary_ratelimit"
)

func NewSecondaryClient(base http.RoundTripper, opts ...github_secondary_ratelimit.Option) *http.Client {
	w := github_secondary_ratelimit.New(base, opts...)
	return &http.Client{
		Transport: w,
	}
}

func NewPrimaryClient(base http.RoundTripper, opts ...github_primary_ratelimit.Option) *http.Client {
	w := github_primary_ratelimit.New(base, opts...)
	return &http.Client{
		Transport: w,
	}
}

type nopServer struct {
}

func UpTo1SecDelay() time.Duration {
	return time.Duration(int(time.Millisecond) * (rand.Int() % 1000))
}

func (n *nopServer) RoundTrip(r *http.Request) (*http.Response, error) {
	time.Sleep(UpTo1SecDelay() / 100)
	return &http.Response{
		Body:   io.NopCloser(strings.NewReader("some response")),
		Header: http.Header{},
	}, nil
}

func SetupSecondaryInjecter(t *testing.T, every time.Duration, sleep time.Duration) http.RoundTripper {
	options := RateLimitInjecterOptions{
		Every:             every,
		InjectionDuration: sleep,
	}
	return SetupInjecterWithOptions(t, options, nil)
}

func SetupPrimaryInjecter(t *testing.T, every time.Duration, sleep time.Duration, category github_primary_ratelimit.ResourceCategory) http.RoundTripper {
	options := RateLimitInjecterOptions{
		Every:                    every,
		InjectionDuration:        sleep,
		UsePrimaryRateLimit:      true,
		PrimaryRateLimitCategory: category,
	}
	return SetupInjecterWithOptions(t, options, nil)
}

func SetupInjecterWithOptions(t *testing.T, options RateLimitInjecterOptions, roundTrippger http.RoundTripper) http.RoundTripper {
	if roundTrippger == nil {
		roundTrippger = &nopServer{}
	}
	i, err := NewRateLimitInjecter(roundTrippger, &options)
	if err != nil {
		t.Fatal(err)
	}

	return i
}

func WaitForNextSleep(injecter http.RoundTripper) {
	injecter.(*RateLimitInjecter).WaitForNextInjection()
}
