package github_ratelimit

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_secondary_ratelimit"
)

type PrimaryRateLimiter = github_primary_ratelimit.PrimaryRateLimiter
type PrimaryRateLimiterOption = github_primary_ratelimit.Option

// NewPrimaryLimiter is an alias for github_primary_ratelimit.New.
// Check out options.go @ github_primary_ratelimit for available options.
func NewPrimaryLimiter(base http.RoundTripper, opts ...PrimaryRateLimiterOption) *PrimaryRateLimiter {
	return github_primary_ratelimit.New(base, opts...)
}

type SecondaryRateLimiter = github_secondary_ratelimit.SecondaryRateLimiter
type SecondaryRateLimiterOption = github_secondary_ratelimit.Option

// NewSecondaryLimiter is an alias for github_secondary_ratelimit.New.
// Check out options.go @ github_secondary_ratelimit for available options.
func NewSecondaryLimiter(base http.RoundTripper, opts ...SecondaryRateLimiterOption) *SecondaryRateLimiter {
	return github_secondary_ratelimit.New(base, opts...)
}

// New creates a combined limiter by stacking a SecondaryRateLimiter on top of a PrimaryRateLimiterOption.
// It accepts options of both types and creates the RoundTrippers.
// Check out options.go @ github_primary_ratelimit / github_secondary_ratelimit for available options.
func New(base http.RoundTripper, opts ...any) http.RoundTripper {
	primaryOpts, secondaryOpts := gatherOptions(opts...)
	primary := NewPrimaryLimiter(base, primaryOpts...)
	secondary := NewSecondaryLimiter(primary, secondaryOpts...)

	return secondary
}

// NewClient creates a new HTTP client with the combined rate limiter.
func NewClient(base http.RoundTripper, opts ...any) *http.Client {
	return &http.Client{
		Transport: New(base, opts...),
	}
}

// WithOverrideConfig adds config overrides to the context.
// The overrides are applied on top of the existing config.
// Allows for request-specific overrides.
// It accepts options of both types and overrides accordingly.
func WithOverrideConfig(ctx context.Context, opts ...any) context.Context {
	primaryOpts, secondaryOpts := gatherOptions(opts...)
	if len(primaryOpts) > 0 {
		ctx = github_primary_ratelimit.WithOverrideConfig(ctx, primaryOpts...)
	}
	if len(secondaryOpts) > 0 {
		ctx = github_secondary_ratelimit.WithOverrideConfig(ctx, secondaryOpts...)
	}
	return ctx
}

func gatherOptions(opts ...any) ([]PrimaryRateLimiterOption, []SecondaryRateLimiterOption) {
	primaryOpts := []PrimaryRateLimiterOption{}
	secondaryOpts := []SecondaryRateLimiterOption{}
	for _, opt := range opts {
		if o, ok := opt.(PrimaryRateLimiterOption); ok {
			primaryOpts = append(primaryOpts, o)
		} else if o, ok := opt.(SecondaryRateLimiterOption); ok {
			secondaryOpts = append(secondaryOpts, o)
		} else {
			panic(fmt.Sprintf("unexpected option of type %T: %v", opt, opt))
		}
	}
	return primaryOpts, secondaryOpts
}
