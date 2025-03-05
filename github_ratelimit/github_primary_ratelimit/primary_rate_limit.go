package github_primary_ratelimit

import (
	"fmt"
	"net/http"
	"time"
)

// PrimaryRateLimiter is a RoundTripper for avoiding GitHub primary rate limits.
// see notes @ ratelimit_state.go for design considerations.
type PrimaryRateLimiter struct {
	Base   http.RoundTripper
	config *Config
}

// RateLimitReachedError is an error type for when the primary rate limit is reached.
type RateLimitReachedError struct {
	ResetTime *time.Time
	Request   *http.Request
	Response  *http.Response
	Category  ResourceCategory
}

func (e *RateLimitReachedError) Error() string {
	return fmt.Sprintf(
		"primary rate limit reached on request to %v with category: %s. wait until %v before sending more requests.",
		e.Request.URL,
		e.Category,
		e.ResetTime,
	)
}

func New(base http.RoundTripper, opts ...Option) *PrimaryRateLimiter {
	if base == nil {
		base = http.DefaultTransport
	}
	config := newConfig(opts...)
	return &PrimaryRateLimiter{
		Base:   base,
		config: config,
	}
}

func (l *PrimaryRateLimiter) RoundTrip(request *http.Request) (*http.Response, error) {
	config := l.getRequestConfig(request)
	category := parseRequestCategory(request)
	resetTime := config.state.GetResetTime(category)
	if resetTime != nil {
		resp := NewErrorResponse(request, category)
		ctx := &CallbackContext{
			RoundTripper: l,
			Request:      request,
			Response:     resp,
			Category:     category,
			ResetTime:    resetTime.AsTime(),
		}
		config.TriggerRequestPrevented(ctx)
		if !config.bypassLimit {
			return nil, ctx.AsError()
		}
	}

	resp, err := l.Base.RoundTrip(request)
	if err != nil {
		return resp, err
	}
	callbackContext := &CallbackContext{
		RoundTripper: l,
		Request:      request,
		Response:     resp,
	}

	// update and check
	resetTime = config.state.Update(config, ParsedResponse{resp}, callbackContext)
	if resetTime == nil {
		return resp, nil
	}
	config.TriggerLimitReached(callbackContext)

	return nil, callbackContext.AsError()
}

// GetState can be used to share the primary rate limit knowledge -
// when multiple clients are involved.
// TODO add tests for state sharing
func (l *PrimaryRateLimiter) GetState() *RateLimitState {
	return l.config.state
}

func (r *PrimaryRateLimiter) getRequestConfig(request *http.Request) *Config {
	overrides := GetConfigOverrides(request.Context())
	if overrides == nil {
		// no config override - use the default config (zero-copy)
		return r.config
	}
	reqConfig := *r.config
	reqConfig.ApplyOptions(overrides...)
	return &reqConfig
}
