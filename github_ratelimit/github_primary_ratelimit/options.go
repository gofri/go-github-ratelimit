package github_primary_ratelimit

import "time"

type Option func(*Config)

// WithLimitDetectedCallback adds a callback to be called when a new active rate limit is detected.
func WithLimitDetectedCallback(callback OnLimitReached) Option {
	return func(c *Config) {
		c.onLimitReached = callback
	}
}

// WithRequestPreventedCallback adds a callback to be called when a request is prevented,
// i.e., when the rate limit is active.
// note: this callback is not called when the limit is first detected.
func WithRequestPreventedCallback(callback OnRequestPrevented) Option {
	return func(c *Config) {
		c.onReuqestPrevented = callback
	}
}

// WithLimitResetCallback adds a callback to be called when a rate limit is reset,
// i.e., when an ongoing rate limit is no longer active.
func WithLimitResetCallback(callback OnLimitReset) Option {
	return func(c *Config) {
		c.onLimitReset = callback
	}
}

// WithUnknownCategoryCallback adds a callback to be called when a response from Github contains an unknown category.
// please open an issue if you encounter this to help improve the handling.
func WithUnknownCategoryCallback(callback OnUnknownCategory) Option {
	return func(c *Config) {
		c.onUnknownCategory = callback
	}
}

// WithSharedState is used to set the rate limiter state from an external source.
// Specifically, it is used to share the state between multiple rate limiters.
// e.g.,
// `rateLimiterB := New(nil, WithSharedState(rateLimiterA.GetState()))`
func WithSharedState(state *RateLimitState) Option {
	return func(c *Config) {
		c.state = state
	}
}

// WithBypassLimit is used to flag that no requests shall be prevented.
// Callbacks are still called regardless of this flag.
// This is useful for testing, out-of-band token switching, etc.
func WithBypassLimit() Option {
	return func(c *Config) {
		c.bypassLimit = true
	}
}

// WithSleepUntilReset is used to flag that the rate limiter shall sleep until the reset time.
// This is useful for testing, long-running offline applications, etc.
// Note: it is using the LimitDetectedCallback, so it will not be otherwise called.
// Note: the request that detected the limit still fails, and concurrent requests
// are prevented without sleeping. Use WithRetryAfterReset to wait and continue.
func WithSleepUntilReset() Option {
	return WithLimitDetectedCallback(func(ctx *CallbackContext) {
		time.Sleep(time.Until(*ctx.ResetTime))
	})
}

// WithRetryAfterReset makes the round tripper wait for the rate limit to reset
// and then send the request, instead of returning an error. It applies both to
// the request that detects the limit and to any request prevented while the
// limit is in force, so a caller with requests in flight sees them all complete
// rather than one sleeping while the rest fail.
//
// The wait honours the request's context: a cancelled or expired context ends
// it and returns the context's error. Give long waits a context deadline if the
// caller has one, since a reset can be an hour away. A request carrying a body
// that cannot be rewound is not retried, since resending it is not safe.
//
// A limit that outlasts several waits is reported as an error rather than
// waited on indefinitely, and WithBypassLimit takes precedence: a caller asking
// for no request to be prevented gets the request sent rather than delayed.
//
// Unlike WithSleepUntilReset it does not occupy the LimitDetectedCallback, so a
// caller can still be told when a limit is reached.
func WithRetryAfterReset() Option {
	return func(c *Config) {
		c.retryAfterReset = true
	}
}
