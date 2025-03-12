package main

import (
	"context"
	"fmt"

	"github.com/gofri/go-github-ratelimit/v2/github_ratelimit"
	"github.com/google/go-github/v69/github"
)

func main() {
	// use the plain ratelimiter, without options / callbacks / underlying http.RoundTripper.
	rateLimiter := github_ratelimit.NewClient(nil)
	client := github.NewClient(rateLimiter) // .WithAuthToken("your personal access token")

	// disable go-github's built-in rate limiting
	ctx := context.WithValue(context.Background(), github.BypassRateLimitCheck, true)

	tags, _, err := client.Repositories.ListTags(ctx, "gofri", "go-github-ratelimit", nil)
	if err != nil {
		panic(err)
	}

	for _, tag := range tags {
		fmt.Printf("- %v\n", *tag.Name)
	}
}
