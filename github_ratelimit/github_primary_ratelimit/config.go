package github_primary_ratelimit

import "context"

// Config is the configuration for the rate limiter.
// It is used internally and generated from the options.
// It holds the state of the rate limiter in order to enable state sharing.
type Config struct {
	state       *RateLimitState
	bypassLimit bool

	// callbacks
	onLimitReached     OnLimitReached
	onReuqestPrevented OnRequestPrevented
	onLimitReset       OnLimitReset
	onUnknownCategory  OnUnknownCategory
}

// newConfig creates a new config with the given options.
func newConfig(opts ...Option) *Config {
	var config Config
	config.ApplyOptions(opts...)

	if config.state == nil {
		config.state = NewRateLimitState(GetAllCategories())
	}

	return &config
}

// ApplyOptions applies the options to the config.
func (c *Config) ApplyOptions(opts ...Option) {
	for _, o := range opts {
		if o == nil {
			continue
		}
		o(c)
	}
}

type ConfigOverridesKey struct{}

// WithOverrideConfig adds config overrides to the context.
// The overrides are applied on top of the existing config.
// Allows for request-specific overrides.
func WithOverrideConfig(ctx context.Context, opts ...Option) context.Context {
	return context.WithValue(ctx, ConfigOverridesKey{}, opts)
}

// GetConfigOverrides returns the config overrides from the context, if any.
func GetConfigOverrides(ctx context.Context) []Option {
	cfg := ctx.Value(ConfigOverridesKey{})
	if cfg == nil {
		return nil
	}
	return cfg.([]Option)
}

func (c *Config) TriggerLimitReached(ctx *CallbackContext) {
	if c.onLimitReached == nil {
		return
	}
	c.onLimitReached(ctx)
}

func (c *Config) TriggerRequestPrevented(ctx *CallbackContext) {
	if c.onReuqestPrevented == nil {
		return
	}
	c.onReuqestPrevented(ctx)
}

func (c *Config) TriggerLimitReset(ctx *CallbackContext) {
	if c.onLimitReset == nil {
		return
	}
	c.onLimitReset(ctx)
}

func (c *Config) TriggerUnknownCategory(ctx *CallbackContext) {
	if c.onUnknownCategory == nil {
		return
	}
	c.onUnknownCategory(ctx)
}
