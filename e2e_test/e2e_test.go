package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_ratelimit_test"
	"github.com/gofri/go-github-ratelimit/github_ratelimit/github_secondary_ratelimit"
	"github.com/google/go-github/v64/github"
)

type orgLister struct {
}

func (o *orgLister) GetOrgName() string {
	return "org"
}

func (o *orgLister) RoundTrip(r *http.Request) (*http.Response, error) {
	org := github.Organization{
		Login: github.String(o.GetOrgName()),
	}

	body, err := json.Marshal([]*github.Organization{&org})
	if err != nil {
		return nil, err
	}

	return &http.Response{
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{},
		StatusCode: http.StatusOK,
	}, nil
}

// TestGoGithubClient is a test that uses the go-github client.
func TestGoGithubClientCompatability(t *testing.T) {
	t.Parallel()
	const every = 5 * time.Second
	const sleep = 1 * time.Second

	print := func(context *github_secondary_ratelimit.CallbackContext) {
		t.Logf("Secondary rate limit reached! Sleeping for %.2f seconds [%v --> %v]",
			time.Until(*context.SleepUntil).Seconds(), time.Now(), *context.SleepUntil)
	}

	orgLister := &orgLister{}
	options := github_ratelimit_test.RateLimitInjecterOptions{
		Every:             every,
		InjectionDuration: sleep,
	}

	i := github_ratelimit_test.SetupInjecterWithOptions(t, options, orgLister)
	rateLimiter := github_ratelimit_test.NewSecondaryClient(i, github_secondary_ratelimit.WithLimitDetectedCallback(print))

	client := github.NewClient(rateLimiter)
	orgs, resp, err := client.Organizations.List(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("unexpected error response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %v", resp.StatusCode)
	}

	if len(orgs) != 1 {
		t.Fatalf("unexpected number of orgs: %v", len(orgs))
	}

	if orgs[0].GetLogin() != orgLister.GetOrgName() {
		t.Fatalf("unexpected org name: %v", orgs[0].GetLogin())
	}

	// TODO add tests for:
	// - WithSingleSleepLimit(0, ...) => expect AbuseError
	// - WithSingleSleepLimit(>0, ...) => expect sleeping
}
