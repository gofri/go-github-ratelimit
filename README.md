# go-github-ratelimit

[![Go Report Card](https://goreportcard.com/badge/github.com/gofri/go-github-ratelimit)](https://goreportcard.com/report/github.com/gofri/go-github-ratelimit)

Package `go-github-ratelimit` provides an http.RoundTripper implementation that handles [secondary rate limit](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28#about-secondary-rate-limits) for the GitHub API.  
The RoundTripper waits for the secondary rate limit to finish in a blocking mode and then issues/retries requests.

`go-github-ratelimit` can be used with any HTTP client communicating with GitHub API.    
It is meant to complement [go-github](https://github.com/google/go-github), but there is no association between this repository and the go-github repository nor Google.  
  
## Installation

```go get github.com/gofri/go-github-ratelimit```

## Usage Example (with [go-github](https://github.com/google/go-github))

```go
import "github.com/google/go-github/v58/github"
import "github.com/gofri/go-github-ratelimit/github_ratelimit"

func main() {
  rateLimiter, err := github_ratelimit.NewRateLimitWaiterClient(nil)
  if err != nil {
    panic(err)
  }
  client := github.NewClient(rateLimiter).WithAuthToken("your personal access token")

  // now use the client as you please
}
```

## Options

The RoundTripper accepts a set of options to configure its behavior and set callbacks. nil callbacks are treated as no-op.  
The options are:

- `WithLimitDetectedCallback(callback)`: the callback is triggered before a sleep.
- `WithSingleSleepLimit(duration, callback)`: limit the sleep duration for a single secondary rate limit & trigger a callback when the limit is exceeded.
- `WithTotalSleepLimit`: limit the accumulated sleep duration for all secondary rate limits & trigger a callback when the limit is exceeded.
  
_Note_: to detect secondary rate limits without sleeping, use `WithSingleSleepLimit(0, your_callback_or_nil)`.

## Per-Request Options

Use `WithOverrideConfig(opts...)` to override the configuration for a specific request (using the request context).
Per-request overrides may be useful for special cases of user requests,
as well as fine-grained policy control (e.g., for a sophisticated pagination mechanism).

## License

This package is distributed under the MIT license found in the LICENSE file.  
Contribution and feedback is welcome.
