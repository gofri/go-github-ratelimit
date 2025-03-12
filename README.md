# go-github-ratelimit

[![Go Report Card](https://goreportcard.com/badge/github.com/gofri/go-github-ratelimit)](https://goreportcard.com/report/github.com/gofri/go-github-ratelimit)

Package `go-github-ratelimit` provides a middleware (http.RoundTripper) that handles both [Primary Rate Limit](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?#about-primary-rate-limits) and [Secondary Rate Limit](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?#about-secondary-rate-limits) for the GitHub API.

* Primary rate limits are handled by returning a detailed error.  
* Secondary rate limits are handled by waiting in blocking mode (sleep) and then issuing/retrying requests.  
* There is support for callbacks to be triggered when rate limits are detected/exceeded/etc. - see below.  

The module can be used with any HTTP client communicating with GitHub API. It is designed to have low overhead during good path.    
It is meant to complement [go-github](https://github.com/google/go-github), but there is no association between this repository and the go-github repository nor Google.

## Recommended: Pagination Handling

If you like this package, please check out [go-github-pagination](https://github.com/gofri/go-github-pagination).  
It supports pagination out of the box, and plays well with the rate limit round-tripper.  
It is best to stack the pagination round-tripper on top of the rate limit round-tripper.  


## Installation

```go get github.com/gofri/go-github-ratelimit/v2```

## Usage Example (with [go-github](https://github.com/google/go-github))

see [example/basic.go](example/basic.go) for a runnable example.
```go
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
```

## Client Options

Both RoundTrippers support a set of options to configure their behavior and set callbacks.  
nil callbacks are treated as no-op.  

### Primary Rate Limit Options (see [options.go](github_ratelimit/github_primary_ratelimit/options.go)):

- `WithLimitDetectedCallback(callback)`: the callback is triggered when any primary rate limit is detected.
- `WithRequestPreventedCallback(callback)`: the callback is triggered when a request is prevented due to an active rate limit.
- `WithLimitResetCallback(callback)`: the callback is triggered when the rate limit is reset (deactived).
- `WithUnknownCategoryCallback`: the callback is triggered when the rate limit category in the response is unknown. note: please open an issue if it happens.
- `WithSharedState(state)`: share state between multiple clients (e.g., for a single user running concurrently).
- `WithBypassLimit()`: bypass the rate limit mechanism, i.e., do not prevent requests when a rate limit is active.

### Secondary Rate Limit Options (see [options.go](github_ratelimit/github_secondary_ratelimit/options.go)):

- `WithLimitDetectedCallback(callback)`: the callback is triggered before a sleep.
- `WithSingleSleepLimit(duration, callback)`: limit the sleep duration for a single secondary rate limit & trigger a callback when the limit is exceeded.
- `WithTotalSleepLimit(duration, callback)`: limit the accumulated sleep duration for all secondary rate limits & trigger a callback when the limit is exceeded.
- `WithNoSleep(callback)`: disable sleep for secondary rate limits & trigger a callback upon any secondary rate limit.

## Per-Request Options

Use `WithOverrideConfig(opts...)` to override the configuration for a specific request (using the request context).  
Per-request overrides may be useful for special cases of user requests,
as well as fine-grained policy control (e.g., for a sophisticated pagination mechanism).

## Advanced Example

See [example/advanced.go](example/advanced.go) for a runnable example.
```go
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
```

## Migration (V1 => V2)

The migraiton from v1 to v2 is relatively straight-forward once you check out the examples.  
Please open an issue if you have any trouble -  
I'd be glad to help and add documetation per need.

## Github Rate Limit References

- [Primary Rate Limit](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?#about-primary-rate-limits)
- [Secondary Rate Limit](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?#about-secondary-rate-limits)

## License

This package is distributed under the MIT license found in the LICENSE file.  
Contribution and feedback is welcome.
