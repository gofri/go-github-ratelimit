module github.com/gofri/go-github-ratelimit/v2/example

replace github.com/gofri/go-github-ratelimit/v2 => ../

go 1.23.1

require (
	github.com/gofri/go-github-pagination v1.0.0
	github.com/gofri/go-github-ratelimit/v2 v2.0.1
	github.com/google/go-github/v69 v69.2.0
)

require github.com/google/go-querystring v1.1.0 // indirect
