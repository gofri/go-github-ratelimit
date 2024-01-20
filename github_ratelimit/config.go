package github_ratelimit

import (
	"context"
	"time"
)

// SecondaryRateLimitConfig is the config for the secondary rate limit waiter.
// Use the options to set the config.
type SecondaryRateLimitConfig struct {
	// limits
	singleSleepLimit *time.Duration
	totalSleepLimit  *time.Duration

	// callbacks
	onLimitDetected       OnLimitDetected
	onSingleLimitExceeded OnSingleLimitExceeded
	onTotalLimitExceeded  OnTotalLimitExceeded
}

// newConfig creates a new config with the given options.
func newConfig(opts ...Option) *SecondaryRateLimitConfig {
	var config SecondaryRateLimitConfig
	config.ApplyOptions(opts...)
	return &config
}

// ApplyOptions applies the options to the config.
func (c *SecondaryRateLimitConfig) ApplyOptions(opts ...Option) {
	for _, o := range opts {
		if o == nil {
			continue
		}
		o(c)
	}
}

// IsAboveSingleSleepLimit returns true if the single sleep duration is above the limit.
func (c *SecondaryRateLimitConfig) IsAboveSingleSleepLimit(sleepTime time.Duration) bool {
	return c.singleSleepLimit != nil && sleepTime > *c.singleSleepLimit
}

// IsAboveTotalSleepLimit returns true if the total sleep duration is above the limit.
func (c *SecondaryRateLimitConfig) IsAboveTotalSleepLimit(sleepTime time.Duration, totalSleepTime time.Duration) bool {
	return c.totalSleepLimit != nil && totalSleepTime+sleepTime > *c.totalSleepLimit
}

type secondaryRateLimitConfigOverridesKey struct{}

// WithOverrideConfig adds config overrides to the context.
// The overrides are applied on top of the existing config.
// Allows for request-specific overrides.
func WithOverrideConfig(ctx context.Context, opts ...Option) context.Context {
	return context.WithValue(ctx, secondaryRateLimitConfigOverridesKey{}, opts)
}

// GetConfigOverrides returns the config overrides from the context, if any.
func GetConfigOverrides(ctx context.Context) []Option {
	cfg := ctx.Value(secondaryRateLimitConfigOverridesKey{})
	if cfg == nil {
		return nil
	}
	return cfg.([]Option)
}
