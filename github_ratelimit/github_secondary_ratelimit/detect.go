package github_secondary_ratelimit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderRetryAfter          = "retry-after"
	HeaderXRateLimitReset     = "x-ratelimit-reset"
	HeaderXRateLimitRemaining = "x-ratelimit-remaining"
)

type SecondaryRateLimitBody struct {
	Message     string `json:"message"`
	DocumentURL string `json:"documentation_url"`
}

const (
	SecondaryRateLimitMessage = `You have exceeded a secondary rate limit`
)

var DocumentationSuffixes = []string{
	`secondary-rate-limits`,
	`#abuse-rate-limits`,
}

func HasSecondaryRateLimitSuffix(documentation_url string) bool {
	return slices.ContainsFunc(DocumentationSuffixes, func(suffix string) bool {
		return strings.HasSuffix(documentation_url, suffix)
	})
}

// IsSecondaryRateLimit checks whether the response is a legitimate secondary rate limit.
// It checks the prefix of the message and the suffix of the documentation URL in the response body in case
// the message or documentation URL is modified in the future.
// https://docs.github.com/en/rest/overview/rate-limits-for-the-rest-api#about-secondary-rate-limits
func (s SecondaryRateLimitBody) IsSecondaryRateLimit() bool {
	return strings.HasPrefix(s.Message, SecondaryRateLimitMessage) ||
		HasSecondaryRateLimitSuffix(s.DocumentURL)
}

// see https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api#exceeding-the-rate-limit
var LimitStatusCodes = []int{
	http.StatusForbidden,
	http.StatusTooManyRequests,
}

// isSecondaryRateLimit checks whether the response is a legitimate secondary rate limit.
func isSecondaryRateLimit(resp *http.Response) bool {
	if !slices.Contains(LimitStatusCodes, resp.StatusCode) {
		return false
	}

	if resp.Header == nil {
		return false
	}

	// a primary rate limit
	if remaining, ok := httpHeaderIntValue(resp.Header, HeaderXRateLimitRemaining); ok && remaining == 0 {
		return false
	}

	// an authentic HTTP response (not a primary rate limit)
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false // unexpected error
	}

	// restore original body
	resp.Body = io.NopCloser(bytes.NewReader(rawBody))

	var body SecondaryRateLimitBody
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return false // unexpected error
	}
	if !body.IsSecondaryRateLimit() {
		return false
	}

	return true
}

// defaultSleepDuration is how long to wait for a secondary rate limit that has no retry-after header.
const defaultSleepDuration = time.Minute

// parseSecondaryLimitTime parses the GitHub API response header,
// looking for the secondary rate limit as defined by GitHub API documentation.
// https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api#exceeding-the-rate-limit
func parseSecondaryLimitTime(resp *http.Response) *time.Time {
	if !isSecondaryRateLimit(resp) {
		return nil
	}

	if resetTime := parseRetryAfter(resp.Header); resetTime != nil {
		return resetTime
	}

	// per GitHub API docs, wait at least one minute when retry-after is missing.
	// x-ratelimit-reset is not used here: it is the primary rate limit's reset, up to an hour away.
	// XXX: the docs also suggest an exponential backoff if the limit persists.
	resetTime := time.Now().Add(defaultSleepDuration)
	return &resetTime
}

// parseRetryAfter parses the GitHub API response header in case a Retry-After is returned.
func parseRetryAfter(header http.Header) *time.Time {
	retryAfterSeconds, ok := httpHeaderIntValue(header, "retry-after")
	if !ok || retryAfterSeconds <= 0 {
		return nil
	}

	// per GitHub API, the header is set to the number of seconds to wait
	resetTime := time.Now().Add(time.Duration(retryAfterSeconds) * time.Second)

	return &resetTime
}

// httpHeaderIntValue parses an integer value from the given HTTP header.
func httpHeaderIntValue(header http.Header, key string) (int64, bool) {
	val := header.Get(key)
	if val == "" {
		return 0, false
	}
	asInt, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false
	}
	return asInt, true
}
