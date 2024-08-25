package github_primary_ratelimit

import (
	"net/http"
	"strings"
)

// General references (note there are some inconsistencies between them):
// https://docs.github.com/en/rest/rate-limit/rate-limit#about-rate-limits
// https://docs.github.com/en/rest/rate-limit/rate-limit#get-rate-limit-status-for-the-authenticated-user
type ResourceCategory string

const (
	// The default category
	// used for all HTTP method/url with no other match.
	ResourceCategoryCore ResourceCategory = "core"

	// https://docs.github.com/en/rest/search/search#about-search
	// * /search (except for /search/code)
	ResourceCategorySearch ResourceCategory = "search"

	// https://docs.github.com/en/rest/search/search#search-code
	// * /search/code
	ResourceCategoryCodeSearch ResourceCategory = "code_search"

	// https://docs.github.com/en/graphql
	// * /graphql
	ResourceCategoryGraphQL ResourceCategory = "graphql"

	// https://docs.github.com/en/rest/migrations/source-imports#start-an-import
	// deprecated endpoint; still applicable
	// * /repos/{OWNER}/{REPO}/import
	ResourceCategorySourceImport ResourceCategory = "source_import"

	// https://docs.github.com/en/enterprise-cloud@latest/rest/enterprise-admin/audit-log#get-the-audit-log-for-an-enterprise
	// * /enterprises/{ENTERPRISE}/audit-log
	ResourceCategoryAuditLog ResourceCategory = "audit_log"

	// https://docs.github.com/en/rest/dependency-graph/dependency-submission
	// POST /app/manfiests/{code}/conversions
	ResourceCategoryIntegrationManifest ResourceCategory = "integration_manifest"

	// https://docs.github.com/en/rest/dependency-graph/dependency-submission#create-a-snapshot-of-dependencies-for-a-repository
	// POST /repos/{OWNER}/{REPO}/dependency-graph/snapshots
	ResourceCategoryDependencySnapshots ResourceCategory = "dependency_snapshots"

	// https://docs.github.com/en/rest/code-scanning/code-scanning#upload-an-analysis-as-sarif-data
	// POST /repos/{OWNER}/{REPO}/code-scanning/sarifs
	ResourceCategoryCodeScanningUpload ResourceCategory = "code_scanning_upload"

	// https://docs.github.com/en/rest/actions/self-hosted-runners#about-self-hosted-runners-in-github-actions
	// "... for registring self-hosted runners"; assuming only POST requests are counted
	// POST /orgs/{ORG}/actions/runners
	ResourceCategoryActionsRunnerRegistration ResourceCategory = "actions_runner_registration"

	// https://docs.github.com/en/enterprise-cloud@latest/rest/scim/scim
	// no explicit documentation; assuming only POST requests are counted
	// POST /scim
	ResourceCategoryScim ResourceCategory = "scim"

	// https://docs.github.com/en/enterprise-cloud@latest/admin/monitoring-activity-in-your-enterprise/reviewing-audit-logs-for-your-enterprise/streaming-the-audit-log-for-your-enterprise
	// no API endpoints
	ResourceCategoryAuditLogStreaming ResourceCategory = "audit_log_streaming"
)

func GetAllCategories() []ResourceCategory {
	return []ResourceCategory{
		ResourceCategoryCore,
		ResourceCategorySearch,
		ResourceCategoryCodeSearch,
		ResourceCategoryGraphQL,
		ResourceCategorySourceImport,
		ResourceCategoryAuditLog,
		ResourceCategoryIntegrationManifest,
		ResourceCategoryDependencySnapshots,
		ResourceCategoryCodeScanningUpload,
		ResourceCategoryActionsRunnerRegistration,
		ResourceCategoryScim,
	}
}

func parseRequestCategory(request *http.Request) ResourceCategory {
	return parseCategory(request.Method, request.URL.RawPath)
}

func parseCategory(method string, path string) ResourceCategory {
	switch { // method-agnostic checks:
	case strings.HasPrefix(path, "/search/code"):
		return ResourceCategoryCodeSearch
	case strings.HasPrefix(path, "/search"):
		return ResourceCategorySearch
	case strings.HasPrefix(path, "/graphql"):
		return ResourceCategoryGraphQL
	case strings.HasPrefix(path, "/repos/") && strings.HasSuffix(path, "/import"):
		return ResourceCategorySourceImport
	case strings.HasSuffix(path, "/audit_log"):
		return ResourceCategoryAuditLog
	}

	if method == http.MethodPost {
		switch {
		case strings.HasPrefix(path, "/app/manfiests/") && strings.HasSuffix(path, "/conversions"):
			return ResourceCategoryIntegrationManifest
		case strings.HasPrefix(path, "/repos/") && strings.HasSuffix(path, "/dependency-graph/snapshots"):
			return ResourceCategoryDependencySnapshots
		case strings.HasPrefix(path, "/repos/") && strings.HasSuffix(path, "/code-scanning/sarifs"):
			return ResourceCategoryCodeScanningUpload
		case strings.HasPrefix(path, "/orgs/") && strings.HasSuffix(path, "/actions/runners"):
			return ResourceCategoryActionsRunnerRegistration
		case strings.HasPrefix(path, "/scim"):
			return ResourceCategoryScim
		}
	}

	// default to core
	return ResourceCategoryCore
}
