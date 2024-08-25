package github_primary_ratelimit

import (
	"net/http"
	"time"
)

// CallbackContext is passed to all callbacks.
// Fields might be nillable, depending on the specific callback and field.
type CallbackContext struct {
	RoundTripper *PrimaryRateLimiter
	Request      *http.Request
	Response     *http.Response
	ResetTime    *time.Time
	Category     ResourceCategory
}

// OnLimitReached is called when a new rate limit is detected.
type OnLimitReached func(*CallbackContext)

// OnRequestPrevented is called when an existing rate limit is detected,
// such that the current request is not sent.
type OnRequestPrevented func(*CallbackContext)

// OnLimitReset is called when a rate limit reset time is reached,
// which means that the category is available for use again.
type OnLimitReset func(*CallbackContext)

// OnUnknownCategory is called when an unknown category is detected at the response,
// which means that the rate limiter does not handle it.
type OnUnknownCategory func(*CallbackContext)

func (ctx *CallbackContext) AsError() *RateLimitReachedError {
	return &RateLimitReachedError{
		Request:   ctx.Request,
		Response:  ctx.Response,
		Category:  ctx.Category,
		ResetTime: ctx.ResetTime,
	}
}
