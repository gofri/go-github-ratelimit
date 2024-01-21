module github.com/gofri/go-github-ratelimit/github-ratelimit-test

// make sure to test the local version of the ratelimit package
replace github.com/gofri/go-github-ratelimit => ../..

go 1.19

require (
	github.com/gofri/go-github-ratelimit v1.1.0
	github.com/google/go-github/v58 v58.0.0
)

require github.com/google/go-querystring v1.1.0 // indirect
