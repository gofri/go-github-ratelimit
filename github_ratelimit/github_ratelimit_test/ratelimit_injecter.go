package github_ratelimit_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_secondary_ratelimit"
	github_ratelimit "github.com/gofri/go-github-ratelimit/github_ratelimit/github_secondary_ratelimit"
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
	`https://docs.github.com/en/some-other-option#abuse-rate-limits`,
}

// RateLimitInjecterOptions provide options for the injection.
// note:
// Every is the interval between the start of two rate limit injections.
// It is first counted from the first request,
// then counted from the end of the last injection.
type RateLimitInjecterOptions struct {
	Every                    time.Duration
	InjectionDuration        time.Duration
	InvalidBody              bool
	UseXRateLimit            bool
	UsePrimaryRateLimit      bool
	DocumentationURL         string
	HttpStatusCode           int
	PrimaryRateLimitCategory github_primary_ratelimit.ResourceCategory
}

func NewRateLimitInjecter(base http.RoundTripper, options *RateLimitInjecterOptions) (http.RoundTripper, error) {
	if options.IsNoop() {
		return base, nil
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}

	injecter := &RateLimitInjecter{
		Base:    base,
		options: options,
	}
	return injecter, nil
}

func (r *RateLimitInjecterOptions) IsNoop() bool {
	return r.InjectionDuration == 0
}

func (r *RateLimitInjecterOptions) Validate() error {
	if r.Every < 0 {
		return fmt.Errorf("injecter expects a positive trigger interval")
	}
	if r.InjectionDuration < 0 {
		return fmt.Errorf("injecter expects a positive sleep interval")
	}
	return nil
}

type RateLimitInjecter struct {
	Base          http.RoundTripper
	options       *RateLimitInjecterOptions
	blockUntil    time.Time
	lock          sync.Mutex
	AbuseAttempts int
}

func (t *RateLimitInjecter) RoundTrip(request *http.Request) (*http.Response, error) {
	resp, err := t.Base.RoundTrip(request)
	if err != nil {
		return resp, err
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	now := time.Now()

	// initialize on first use
	if t.blockUntil.IsZero() {
		t.blockUntil = now
		return resp, nil
	}

	// on-going rate limit
	if t.blockUntil.After(now) {
		t.AbuseAttempts++
		return t.inject(resp)
	}

	nextStart := t.nextInjectionStart(now)

	// no-injection period
	if now.Before(nextStart) {
		return resp, nil
	}

	// start a new injection period
	t.blockUntil = nextStart.Add(t.options.InjectionDuration)

	return t.inject(resp)
}

// nextInjectionStart returns the time when the next injection starts.
// note that we use blockUntil as the origin,
// because we want it to be <Every> after the last injection.
// we need to handle the case of multiple-cycle gap,
// because the user might have waited for a long time between requests.
func (r *RateLimitInjecter) nextInjectionStart(now time.Time) time.Time {
	cycleSize := r.options.Every + r.options.InjectionDuration
	sinceLastBlock := now.Sub(r.blockUntil)

	numOfCyclesGap := int(sinceLastBlock / cycleSize)
	gap := cycleSize * time.Duration(numOfCyclesGap)
	endOfLastBlock := r.blockUntil.Add(gap)

	return endOfLastBlock.Add(r.options.Every)

}

func (r *RateLimitInjecter) WaitForNextInjection() {
	r.lock.Lock()
	defer r.lock.Unlock()
	time.Sleep(time.Until(r.nextInjectionStart(time.Now())))
}

func getSecondaryRateLimitBody(documentationURL string) (io.ReadCloser, error) {
	if len(documentationURL) == 0 {
		documentationURL = SecondaryRateLimitDocumentationURLs[rand.Intn(len(SecondaryRateLimitDocumentationURLs))]
	}

	body := github_secondary_ratelimit.SecondaryRateLimitBody{
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
		// XXX: not perfect, but luckily primary & secondary share status codes,
		// 		so let's keep it at that for now.
		codes := github_secondary_ratelimit.LimitStatusCodes
		return codes[rand.Intn(len(codes))]
	}
	return statusCode
}

func (t *RateLimitInjecter) inject(resp *http.Response) (*http.Response, error) {
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

func (t *RateLimitInjecter) toRetryResponse(resp *http.Response) *http.Response {
	secondsToBlock := t.getTimeToBlock()
	httpHeaderSetIntValue(resp, github_ratelimit.HeaderRetryAfter, int(secondsToBlock.Seconds()))
	return resp
}

func (t *RateLimitInjecter) toXRateLimitResponse(resp *http.Response) *http.Response {
	endOfBlock := time.Now().Add(t.getTimeToBlock())
	httpHeaderSetIntValue(resp, github_ratelimit.HeaderXRateLimitReset, int(endOfBlock.Unix()))
	return resp
}

func (t *RateLimitInjecter) toPrimaryRateLimitResponse(resp *http.Response) *http.Response {
	httpHeaderSetIntValue(resp, github_ratelimit.HeaderXRateLimitRemaining, 0)
	if category := t.options.PrimaryRateLimitCategory; category != "" {
		resp.Header.Set(string(github_primary_ratelimit.ResponseHeaderKeyCategory), string(category))
	}
	resp.StatusCode = GetRandomStatusCode()
	return t.toXRateLimitResponse(resp)
}

func (t *RateLimitInjecter) getTimeToBlock() time.Duration {
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

func GetRandomStatusCode() int {
	codes := github_primary_ratelimit.PrimaryLimitStatusCodes
	return codes[rand.Intn(len(codes))]
}
