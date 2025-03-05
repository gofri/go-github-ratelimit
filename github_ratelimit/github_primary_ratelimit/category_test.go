package github_primary_ratelimit

import (
	"net/http"
	"testing"
)

func TestCategory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Path     string
		Category ResourceCategory
		Method   string
	}{
		{
			Path:     "/search/code/xxx",
			Category: ResourceCategoryCodeSearch,
			Method:   http.MethodGet,
		},
		{
			Path:     "/search?xxx=yyy",
			Category: ResourceCategorySearch,
			Method:   http.MethodGet,
		},
		{
			Path:     "/graphql?xxx=yyy",
			Category: ResourceCategoryGraphQL,
			Method:   http.MethodGet,
		},
		{
			Path:     "xxx/audit_log",
			Category: ResourceCategoryAuditLog,
			Method:   http.MethodGet,
		},
		{
			Path:     "/app/manfiests/xxx/conversions",
			Category: ResourceCategoryIntegrationManifest,
			Method:   http.MethodPost,
		},
		{
			Path:     "/repos/xxx/dependency-graph/snapshots",
			Category: ResourceCategoryDependencySnapshots,
			Method:   http.MethodPost,
		},
		{
			Path:     "/repos/xxx/code-scanning/sarifs",
			Category: ResourceCategoryCodeScanningUpload,
			Method:   http.MethodPost,
		},
		{
			Path:     "/orgs/xxx/actions/runners",
			Category: ResourceCategoryActionsRunnerRegistration,
			Method:   http.MethodPost,
		},
		{
			Path:     "/scim",
			Category: ResourceCategoryScim,
			Method:   http.MethodPost,
		},
		{
			Path:     "/xxx",
			Category: ResourceCategoryCore,
			Method:   http.MethodPost,
		},
	}

	for _, test := range tests {
		if got, want := parseCategory(test.Method, test.Path), test.Category; got != want {
			t.Fatalf("category mismatch for path: '%v' with method %v: got %v, expected %v", test.Path, test.Method, got, want)
		}
	}
}
