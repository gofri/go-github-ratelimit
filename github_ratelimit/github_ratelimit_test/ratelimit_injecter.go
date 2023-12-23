package github_ratelimit_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit"
)

const (
	InvalidBodyContent = `{"message": "not as expected"}`
)

const (
	SecondaryRateLimitMessage = `You have exceeded a secondary rate limit. Please wait a few minutes before you try again.`
)

var SecondaryRateLimitDocumentationURLs = []string{
	`https://docs.github.com/rest/overview/resources-in-the-rest-api#secondary-rate-limits`,
	`https://docs.github.com/free-pro-team@latest/rest/overview/resources-in-the-rest-api#secondary-rate-limits`,
	`https://docs.github.com/en/free-pro-team@latest/rest/overview/rate-limits-for-the-rest-api#about-secondary-rate-limits`,
}

var SecondaryRateLimitStatusCodes = []int{
	http.StatusForbidden,
	http.StatusTooManyRequests,
}

type SecondaryRateLimitInjecterOptions struct {
	Every               time.Duration
	Sleep               time.Duration
	InvalidBody         bool
	UseXRateLimit       bool
	UsePrimaryRateLimit bool
	DocumentationURL    string
	HttpStatusCode      int
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

type SecondaryRateLimitInjecter struct {
	base          http.RoundTripper
	options       *SecondaryRateLimitInjecterOptions
	blockUntil    time.Time
	lock          sync.Mutex
	AbuseAttempts int
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
		return t.inject(resp)
	}

	nextStart := t.NextSleepStart()

	// start a rate limit period
	if !now.Before(nextStart) {
		t.blockUntil = nextStart.Add(t.options.Sleep)
		return t.inject(resp)
	}

	return resp, nil
}

func (r *SecondaryRateLimitInjecter) CurrentSleepEnd() time.Time {
	return r.blockUntil
}

func (r *SecondaryRateLimitInjecter) NextSleepStart() time.Time {
	return r.blockUntil.Add(r.options.Every)
}

func getSecondaryRateLimitBody(documentationURL string) (io.ReadCloser, error) {
	if len(documentationURL) == 0 {
		documentationURL = SecondaryRateLimitDocumentationURLs[0]
	}

	body := github_ratelimit.SecondaryRateLimitBody{
		Message:     SecondaryRateLimitMessage,
		DocumentURL: documentationURL,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	return io.NopCloser(bytes.NewReader(bodyBytes)), nil
}

func getHttpStatusCode(statusCode int) int {
	if statusCode == 0 {
		return SecondaryRateLimitStatusCodes[0]
	}
	return statusCode
}

func (t *SecondaryRateLimitInjecter) inject(resp *http.Response) (*http.Response, error) {
	if t.options.UsePrimaryRateLimit {
		return t.toPrimaryRateLimitResponse(resp), nil
	} else {
		body, err := getSecondaryRateLimitBody(t.options.DocumentationURL)
		if err != nil {
			return nil, err
		}
		if t.options.InvalidBody {
			body = io.NopCloser(bytes.NewReader([]byte(InvalidBodyContent)))
		}

		resp.StatusCode = getHttpStatusCode(t.options.HttpStatusCode)
		resp.Body = body
		if t.options.UseXRateLimit {
			return t.toXRateLimitResponse(resp), nil
		} else {
			return t.toRetryResponse(resp), nil
		}
	}
}

func (t *SecondaryRateLimitInjecter) toRetryResponse(resp *http.Response) *http.Response {
	secondsToBlock := t.getTimeToBlock()
	httpHeaderSetIntValue(resp, github_ratelimit.HeaderRetryAfter, int(secondsToBlock.Seconds()))
	return resp
}

func (t *SecondaryRateLimitInjecter) toXRateLimitResponse(resp *http.Response) *http.Response {
	endOfBlock := time.Now().Add(t.getTimeToBlock())
	httpHeaderSetIntValue(resp, github_ratelimit.HeaderXRateLimitReset, int(endOfBlock.Unix()))
	return resp
}

func (t *SecondaryRateLimitInjecter) toPrimaryRateLimitResponse(resp *http.Response) *http.Response {
	httpHeaderSetIntValue(resp, github_ratelimit.HeaderXRateLimitRemaining, 0)
	return t.toXRateLimitResponse(resp)
}

func (t *SecondaryRateLimitInjecter) getTimeToBlock() time.Duration {
	timeUntil := time.Until(t.blockUntil)
	if timeUntil.Nanoseconds()%int64(time.Second) > 0 {
		timeUntil += time.Second
	}
	return timeUntil
}

func httpHeaderSetIntValue(resp *http.Response, key string, value int) {
	resp.Header.Set(key, strconv.Itoa(value))
}

func IsInvalidBody(resp *http.Response) (bool, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	return string(body) == InvalidBodyContent, nil
}
