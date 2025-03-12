package main

import (
	"context"
	"fmt"

	"github.com/gofri/go-github-pagination/githubpagination"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_primary_ratelimit"
	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit/github_secondary_ratelimit"
	"github.com/google/go-github/v69/github"
)

func main() {
	rateLimiter := github_ratelimit.New(nil,
		github_primary_ratelimit.WithLimitDetectedCallback(func(ctx *github_primary_ratelimit.CallbackContext) {
			fmt.Printf("Primary rate limit detected: category %s, reset time: %v\n", ctx.Category, ctx.ResetTime)
		}),
		github_secondary_ratelimit.WithLimitDetectedCallback(func(ctx *github_secondary_ratelimit.CallbackContext) {
			fmt.Printf("Secondary rate limit detected: reset time: %v, total sleep time: %v\n", ctx.ResetTime, ctx.TotalSleepTime)
		}),
	)

	paginator := githubpagination.NewClient(rateLimiter,
		githubpagination.WithPerPage(100), // default to 100 results per page
	)
	client := github.NewClient(paginator) // .WithAuthToken("your personal access token")

	// disable go-github's built-in rate limiting
	ctx := context.WithValue(context.Background(), github.BypassRateLimitCheck, true)

	// list repository tags
	tags, _, err := client.Repositories.ListTags(ctx, "gofri", "go-github-ratelimit", nil)
	if err != nil {
		panic(err)
	}

	for _, tag := range tags {
		fmt.Printf("- %v\n", *tag.Name)
	}
}
