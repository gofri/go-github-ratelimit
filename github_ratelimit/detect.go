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
	SecondaryRateLimitMessage           = `You have exceeded a secondary rate limit`
	SecondaryRateLimitDocumentationPath = `/rest/overview/resources-in-the-rest-api#secondary-rate-limits`
)

func (s SecondaryRateLimitBody) IsSecondaryRateLimit() bool {
	return strings.HasPrefix(s.Message, SecondaryRateLimitMessage) && strings.HasSuffix(s.DocumentURL, SecondaryRateLimitDocumentationPath)
}

// isSecondaryRateLimit checks whether the response is a legitimate secondary rate limit.
// it is used to avoid handling primary rate limits and authentic HTTP Forbidden (403) responses.
func isSecondaryRateLimit(resp *http.Response) bool {
	if resp.StatusCode != http.StatusForbidden {
		return false
	}

	if resp.Header == nil {
		return false
	}

	// a primary rate limit
	if remaining, ok := httpHeaderIntValue(resp.Header, HeaderXRateLimitRemaining); ok && remaining == 0 {
		return false
	}

	// an authentic HTTP Forbidden (403) response
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
