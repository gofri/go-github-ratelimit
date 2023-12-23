package github_ratelimit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type SecondaryRateLimitBody struct {
	Message     string `json:"message"`
	DocumentURL string `json:"documentation_url"`
}

const (
	SecondaryRateLimitMessage                 = `You have exceeded a secondary rate limit`
	SecondaryRateLimitDocumentationPathSuffix = `secondary-rate-limits`
)

// IsSecondaryRateLimit checks whether the response is a legitimate secondary rate limit.
// It checks the prefix of the message and the suffix of the documentation URL in the response body in case
// the message or documentation URL is modified in the future.
// https://docs.github.com/en/rest/overview/rate-limits-for-the-rest-api#about-secondary-rate-limits
func (s SecondaryRateLimitBody) IsSecondaryRateLimit() bool {
	return strings.HasPrefix(s.Message, SecondaryRateLimitMessage) ||
		strings.HasSuffix(s.DocumentURL, SecondaryRateLimitDocumentationPathSuffix)
}

// isRateLimitStatus checks whether the status code is a rate limit status code.
// see https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api#exceeding-the-rate-limit
func isRateLimitStatus(statusCode int) bool {
	return statusCode == http.StatusForbidden || statusCode == http.StatusTooManyRequests
}

// isSecondaryRateLimit checks whether the response is a legitimate secondary rate limit.
func isSecondaryRateLimit(resp *http.Response) bool {
	if !isRateLimitStatus(resp.StatusCode) {
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
