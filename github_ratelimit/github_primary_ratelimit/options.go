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
func WithSleepUntilReset() Option {
	return WithLimitDetectedCallback(func(ctx *CallbackContext) {
		time.Sleep(time.Until(*ctx.ResetTime))
	})
}
