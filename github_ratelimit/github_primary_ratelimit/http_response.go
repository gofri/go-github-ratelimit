package github_primary_ratelimit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"

	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_secondary_ratelimit"
)

// ParsedResponse is a wrapper around http.Response that provides additional functionality.
// It is used to parse the response and extract rate limit information.
type ParsedResponse struct {
	resp *http.Response
}

// https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api#checking-the-status-of-your-rate-limit
type ResponseHeaderKey string

// https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api#exceeding-the-rate-limit
const (
	ResponseHeaderKeyRemaining ResponseHeaderKey = "x-ratelimit-remaining"
	ResponseHeaderKeyReset     ResponseHeaderKey = "x-ratelimit-reset"
	ResponseHeaderKeyCategory  ResponseHeaderKey = "x-ratelimit-resource"
)

func (k ResponseHeaderKey) Get(response *http.Response) string {
	return response.Header.Get(string(k))
}

var PrimaryLimitStatusCodes = []int{
	http.StatusForbidden,
	http.StatusTooManyRequests,
}

func (p ParsedResponse) GetCatgory() ResourceCategory {
	category := p.getHeader(ResponseHeaderKeyCategory)
	return ResourceCategory(category)
}

func (p ParsedResponse) GetResetTime() *SecondsSinceEpoch {
	if !p.limitReached() {
		return nil
	}

	reset := p.getHeader(ResponseHeaderKeyReset)
	seconds, _ := strconv.Atoi(reset)
	s := SecondsSinceEpoch(seconds)
	return &s
}

func (p ParsedResponse) limitReached() bool {
	if !slices.Contains(PrimaryLimitStatusCodes, p.resp.StatusCode) {
		return false
	}
	if remaining := p.getHeader(ResponseHeaderKeyRemaining); remaining != "0" {
		return false
	}
	// GitHub sends `x-ratelimit-remaining: 0` on some secondary rate limits too,
	// so the status and that header together do not tell the two apart. The body
	// does. Arming the category on a secondary limit blocks every request in it
	// until the hourly reset, for a limit that asks for a backoff of seconds.
	// https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api#about-secondary-rate-limits
	if p.isSecondaryRateLimit() {
		return false
	}
	return true
}

// isSecondaryRateLimit reports whether the response says it is a secondary rate
// limit. The body is restored so that whoever handles the response next still
// has it.
func (p ParsedResponse) isSecondaryRateLimit() bool {
	if p.resp.Body == nil {
		return false
	}

	rawBody, err := io.ReadAll(p.resp.Body)
	p.resp.Body.Close()
	p.resp.Body = io.NopCloser(bytes.NewReader(rawBody))
	if err != nil {
		return false
	}

	var body github_secondary_ratelimit.SecondaryRateLimitBody
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return false
	}

	return body.IsSecondaryRateLimit()
}

func (p ParsedResponse) getHeader(key ResponseHeaderKey) string {
	return p.resp.Header.Get(string(key))
}

func NewErrorResponse(request *http.Request, category ResourceCategory) *http.Response {
	header := make(http.Header)
	header.Set(string(ResponseHeaderKeyRemaining), "0")
	header.Set(string(ResponseHeaderKeyCategory), string(category))
	return &http.Response{
		Status:     http.StatusText(http.StatusForbidden),
		StatusCode: http.StatusForbidden,
		Request:    request,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}
}
