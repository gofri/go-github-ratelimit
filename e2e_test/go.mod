module github.com/gofri/go-github-ratelimit-e2e

replace github.com/gofri/go-github-ratelimit/v2 => ../

go 1.23.1

require (
	github.com/gofri/go-github-ratelimit/v2 v2.0.0-00010101000000-000000000000
	github.com/google/go-github/v64 v64.0.0
)

require github.com/google/go-querystring v1.1.0 // indirect
