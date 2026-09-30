package github_primary_ratelimit

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// maxAttempts bounds the number of requests a single RoundTrip may send when
// WithRetryAfterReset is set. Waiting is only worth doing while the limit is
// expected to lift: a limit that survives several waits will not be waited out,
// and a reset time that is missing or already past makes every wait return at
// once, which without a bound would retry in a tight loop.
const maxAttempts = 4

func (l *PrimaryRateLimiter) RoundTrip(request *http.Request) (*http.Response, error) {
	attempt := request
	attempts := 0
	for {
		config := l.getRequestConfig(attempt)
		category := parseRequestCategory(attempt)
		resetTime := config.state.GetResetTime(category)
		if resetTime != nil {
			resp := NewErrorResponse(attempt, category)
			ctx := &CallbackContext{
				RoundTripper: l,
				Request:      attempt,
				Response:     resp,
				Category:     category,
				ResetTime:    resetTime.AsTime(),
			}
			config.TriggerRequestPrevented(ctx)
			switch {
			case config.bypassLimit:
				// the caller asked for no request to be prevented, so send it.
			case config.retryAfterReset && attempts < maxAttempts:
				next, err := waitForReset(request, *resetTime.AsTime())
				if err != nil {
					return nil, err
				}
				attempt = next
				continue
			default:
				return nil, ctx.AsError()
			}
		}

		attempts++
		resp, err := l.Base.RoundTrip(attempt)
		if err != nil {
			return resp, err
		}
		callbackContext := &CallbackContext{
			RoundTripper: l,
			Request:      attempt,
			Response:     resp,
		}

		// update and check
		resetTime = config.state.Update(config, ParsedResponse{resp}, callbackContext)
		if resetTime == nil {
			return resp, nil
		}
		config.TriggerLimitReached(callbackContext)

		if config.retryAfterReset && attempts < maxAttempts {
			// the limit this response records is now in the state, so let the
			// top of the loop wait it out.
			drainAndClose(resp)
			next, err := rewind(request)
			if err != nil {
				return nil, err
			}
			attempt = next
			continue
		}

		return nil, callbackContext.AsError()
	}
}

// ErrBodyNotRewindable is returned when a request cannot be retried because its
// body has already been consumed and there is no way to produce it again.
var ErrBodyNotRewindable = errors.New("github_primary_ratelimit: request body cannot be rewound, so the request cannot be retried")

// waitForReset sleeps until the limit resets and returns a request that can be
// sent again.
func waitForReset(request *http.Request, until time.Time) (*http.Request, error) {
	if err := sleepUntil(request.Context(), until); err != nil {
		return nil, err
	}
	return rewind(request)
}

// rewind returns a copy of the request that can be sent again, with a body read
// from the start. The original is left untouched, since a RoundTripper may not
// modify the request it was given.
func rewind(request *http.Request) (*http.Request, error) {
	next := request.Clone(request.Context())
	if request.Body == nil || request.Body == http.NoBody {
		return next, nil
	}
	if request.GetBody == nil {
		return nil, ErrBodyNotRewindable
	}
	body, err := request.GetBody()
	if err != nil {
		return nil, fmt.Errorf("rewinding request body: %w", err)
	}
	next.Body = body

	return next, nil
}

// sleepUntil waits for the deadline or for the context to end, whichever comes
// first. A deadline already past returns immediately.
func sleepUntil(ctx context.Context, deadline time.Time) error {
	wait := time.Until(deadline)
	if wait <= 0 {
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// drainAndClose frees the connection behind a response that is being discarded.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
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
