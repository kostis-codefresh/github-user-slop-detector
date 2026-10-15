package analysis

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAPI serves a canned GraphQL response and records the requests it saw.
func stubAPI(t *testing.T, body string) (*Client, *[]map[string]any) {
	t.Helper()
	var requests []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err, "could not read the request body")

		var payload struct {
			Variables map[string]any `json:"variables"`
		}
		require.NoError(t, json.Unmarshal(raw, &payload), "request was not valid JSON")
		requests = append(requests, payload.Variables)

		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"), "the token should be sent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	client := NewClient("test-token")
	client.Endpoint = server.URL
	return client, &requests
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err, "could not read fixture %s", name)
	return string(raw)
}

func TestFetch(t *testing.T) {
	since := time.Date(2025, 3, 23, 0, 0, 0, 0, time.UTC)
	client, requests := stubAPI(t, readFixture(t, "response.json"))

	c, err := client.Fetch(context.Background(), "octocat", since)
	require.NoError(t, err, "fetching should succeed")

	t.Run("Maps the profile", func(t *testing.T) {
		assert.Equal(t, "octocat", c.Login, "login mismatch")
		assert.Equal(t, "I write software", c.Bio, "bio mismatch")
		assert.Equal(t, "Acme", c.Company, "company mismatch")
		assert.Equal(t, 26, c.Followers, "follower count mismatch")
		assert.Equal(t, 13, c.PublicRepos, "public repo count mismatch")
		assert.Equal(t, 4, c.ReviewsGiven, "review count mismatch")
	})

	t.Run("Filters PRs to the window", func(t *testing.T) {
		// The fixture's fourth PR was opened in 2020, well before the window.
		require.Len(t, c.PullRequests, 3, "only PRs created inside the window should survive")
		for _, pr := range c.PullRequests {
			assert.False(t, pr.CreatedAt.Before(since), "a PR from before the window was kept")
		}
	})

	t.Run("Tolerates a deleted repository", func(t *testing.T) {
		// GitHub returns a null repository once it has been deleted or made
		// private; that must not lose the PR or panic.
		assert.Equal(t, "/", c.PullRequests[2].Repo(), "a null repository should decode to empty owner and name")
		assert.True(t, c.PullRequests[2].Merged, "the PR itself is still a merge")
	})

	t.Run("Tolerates a deleted merger", func(t *testing.T) {
		assert.Empty(t, c.PullRequests[1].MergedBy, "a null mergedBy should decode to an empty login")
	})

	t.Run("Collects the active months", func(t *testing.T) {
		assert.Equal(t, map[string]bool{"2025-06": true, "2025-07": true}, c.ActiveMonths,
			"only days with contributions should mark their month active")
	})

	t.Run("Keeps comments unfiltered", func(t *testing.T) {
		assert.Len(t, c.IssueComments, 2, "both comments should be kept")
	})

	t.Run("Skips non-issue search results", func(t *testing.T) {
		// The search connection returns empty objects for nodes that are not
		// issues; those must not become phantom issues.
		require.Len(t, c.Issues, 1, "only the real issue should be kept")
		assert.Equal(t, 3, c.Issues[0].Comments, "issue comment count mismatch")
	})

	t.Run("Records the rate limit budget", func(t *testing.T) {
		assert.Equal(t, 4900, c.RateLimit.Remaining, "remaining points mismatch")
		assert.Equal(t, 100, c.RateLimit.Used, "used points mismatch")
		assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), c.RateLimit.ResetAt, "reset time mismatch")
	})

	t.Run("Sends the expected search query", func(t *testing.T) {
		require.NotEmpty(t, *requests, "at least one request should have been made")
		assert.Equal(t, "author:octocat is:issue created:>=2025-03-23", (*requests)[0]["issueSearchQuery"],
			"the issue search query should scope to the author and window")
	})
}

func TestFetchPaginates(t *testing.T) {
	const (
		profileBody = `{"data":{"user":{"login":"octocat","createdAt":"2020-01-01T00:00:00Z",
			"followers":{"totalCount":1},"repositories":{"totalCount":1},
			"contributionsCollection":{"contributionCalendar":{"weeks":[]},
			"pullRequestReviewContributions":{"totalCount":0}}},
			"rateLimit":{"remaining":100,"resetAt":"2026-01-01T00:00:00Z","used":1}}}`

		// A first PR page that claims more data, then a page that ends it.
		prPage1 = `{"data":{"user":{"login":"octocat","pullRequests":{"totalCount":2,
			"nodes":[{"state":"OPEN","createdAt":"2025-06-01T00:00:00Z",
			"repository":{"owner":{"login":"org"},"name":"a","stargazerCount":1}}],
			"pageInfo":{"hasNextPage":true,"endCursor":"CURSOR-1"}}},
			"rateLimit":{"remaining":100,"resetAt":"2026-01-01T00:00:00Z","used":1}}}`

		prPage2 = `{"data":{"user":{"login":"octocat","pullRequests":{"totalCount":2,
			"nodes":[{"state":"OPEN","createdAt":"2025-07-01T00:00:00Z",
			"repository":{"owner":{"login":"org"},"name":"b","stargazerCount":1}}],
			"pageInfo":{"hasNextPage":false,"endCursor":""}}},
			"rateLimit":{"remaining":99,"resetAt":"2026-01-01T00:00:00Z","used":2}}}`

		emptyBody = `{"data":{"user":{"login":"octocat"},"issueSearch":{"issueCount":0,"nodes":[]},
			"rateLimit":{"remaining":99,"resetAt":"2026-01-01T00:00:00Z","used":2}}}`
	)

	// Each request now asks for one section, so the stub answers on the @include
	// flags and the cursor rather than on how many requests it has seen.
	var prRequests, totalRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err, "could not read the request body")
		var payload struct {
			Variables struct {
				WithProfile bool   `json:"withProfile"`
				WithPRs     bool   `json:"withPRs"`
				PRCursor    string `json:"prCursor"`
			} `json:"variables"`
		}
		require.NoError(t, json.Unmarshal(raw, &payload), "request was not valid JSON")

		totalRequests++
		switch {
		case payload.Variables.WithProfile:
			_, _ = io.WriteString(w, profileBody)
		case payload.Variables.WithPRs:
			prRequests++
			if payload.Variables.PRCursor == "CURSOR-1" {
				_, _ = io.WriteString(w, prPage2)
				return
			}
			_, _ = io.WriteString(w, prPage1)
		default:
			_, _ = io.WriteString(w, emptyBody)
		}
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.Endpoint = server.URL

	c, err := client.Fetch(context.Background(), "octocat", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err, "fetching should succeed")

	assert.Equal(t, 2, prRequests, "the second PR page should have been requested")
	assert.Len(t, c.PullRequests, 2, "PRs from both pages should be kept")
	assert.Equal(t, 99, c.RateLimit.Remaining, "the budget should come from the last response, not the first")

	// profile + 2 PR pages + comments + search: the sections are separate requests
	// so that none of them has to finish inside GitHub's 10s limit together.
	assert.Equal(t, 5, totalRequests, "each section should be its own request")
}

// TestFetchStopsPagingBeforeTheWindow pins the early stop: PRs come newest
// first, so a page that already reaches before the window ends the pagination
// even when GitHub says there is more.
func TestFetchStopsPagingBeforeTheWindow(t *testing.T) {
	const prPage = `{"data":{"user":{"login":"octocat","createdAt":"2020-01-01T00:00:00Z",
		"followers":{"totalCount":1},"repositories":{"totalCount":1},
		"contributionsCollection":{"contributionCalendar":{"weeks":[]},
		"pullRequestReviewContributions":{"totalCount":0}},
		"pullRequests":{"totalCount":3,
		"nodes":[{"state":"OPEN","createdAt":"2025-06-01T00:00:00Z"},
		{"state":"OPEN","createdAt":"2024-06-01T00:00:00Z"}],
		"pageInfo":{"hasNextPage":true,"endCursor":"CURSOR-1"}}},
		"issueSearch":{"issueCount":0,"nodes":[]},
		"rateLimit":{"remaining":100,"resetAt":"2026-01-01T00:00:00Z","used":1}}}`

	client, requests := stubAPI(t, prPage)

	c, err := client.Fetch(context.Background(), "octocat", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err, "fetching should succeed")

	var prRequests int
	for _, vars := range *requests {
		if vars["withPRs"] == true {
			prRequests++
		}
	}
	assert.Equal(t, 1, prRequests, "a page reaching before the window should end the PR pagination")
	assert.Len(t, c.PullRequests, 1, "only the PR inside the window should be kept")
}

// TestFetchRequestsOneSectionPerRequest pins the split itself: a section's
// @include flag must be on for exactly the request that needs it, otherwise the
// combined document comes back and the 10s timeout returns.
func TestFetchRequestsOneSectionPerRequest(t *testing.T) {
	client, requests := stubAPI(t, readFixture(t, "response.json"))

	_, err := client.Fetch(context.Background(), "octocat", time.Date(2025, 3, 23, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err, "fetching should succeed")

	flags := []string{"withProfile", "withPRs", "withComments", "withSearch"}
	require.Len(t, *requests, len(flags), "one request per section")

	for i, want := range flags {
		for _, flag := range flags {
			assert.Equal(t, flag == want, (*requests)[i][flag],
				"request %d should set only %s, but %s was %v", i, want, flag, (*requests)[i][flag])
		}
	}
}

// TestFetchStopsOnRateLimit pins the no-retry contract: a rate-limited response
// must fail the run on the first request rather than spend more of a budget that
// is already gone. A case that took seconds would mean the retry loop still runs.
func TestFetchStopsOnRateLimit(t *testing.T) {
	reset := time.Now().Add(42 * time.Minute).UTC().Truncate(time.Second)

	testCases := []struct {
		name        string
		status      int
		headers     map[string]string
		body        string
		expectedErr string
	}{
		{
			name:    "Primary limit as HTTP 429",
			status:  http.StatusTooManyRequests,
			headers: map[string]string{"Retry-After": "60"},
			body:    `{"message":"API rate limit exceeded"}`,
			// Retry-After is a delta, so only the "in ..." half is predictable here.
			expectedErr: "GitHub API rate limit exceeded (not retrying)",
		},
		{
			name:   "Primary limit as HTTP 403 with the count zeroed",
			status: http.StatusForbidden,
			headers: map[string]string{
				"X-RateLimit-Remaining": "0",
				"X-RateLimit-Reset":     strconv.FormatInt(reset.Unix(), 10),
			},
			body:        `{"message":"API rate limit exceeded for user ID 1."}`,
			expectedErr: "0 points remaining, resets at " + reset.Format(time.RFC3339),
		},
		{
			name:        "Secondary limit as HTTP 403 with no headers",
			status:      http.StatusForbidden,
			body:        `{"message":"You have exceeded a secondary rate limit"}`,
			expectedErr: "You have exceeded a secondary rate limit",
		},
		{
			name:        "RATE_LIMITED GraphQL error",
			status:      http.StatusOK,
			body:        `{"errors":[{"message":"API rate limit exceeded for user ID 1."}]}`,
			expectedErr: "GitHub API rate limit exceeded (not retrying)",
		},
		{
			name:        "A plain 403 is still a token rejection",
			status:      http.StatusForbidden,
			body:        `{"message":"Bad credentials"}`,
			expectedErr: "GitHub rejected the token",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.Endpoint = server.URL

			_, err := client.Fetch(context.Background(), "octocat", time.Now())

			require.Error(t, err, "the rate limit should fail the run")
			assert.Contains(t, err.Error(), tc.expectedErr, "error message mismatch")
			assert.Equal(t, 1, calls, "the request must not be retried")
		})
	}
}

// TestFetchStopsOnQueryTimeout pins the split between the two kinds of 5xx: a
// 502/504 is GitHub giving up on a query that ran past its 10s limit, and each
// one deducts extra points from the hourly budget, so retrying the same
// too-expensive document is not worth paying for. Other 5xx stay retryable.
func TestFetchStopsOnQueryTimeout(t *testing.T) {
	// The nginx body is what GitHub actually returned in practice; the GraphQL
	// JSON form is what the docs describe. Both must be reported.
	timeoutBody := `<html>` + "\n" + `<head><title>502 Bad Gateway</title></head>` + "\n" + `</html>`

	testCases := []struct {
		name        string
		status      int
		expectCalls int
		expectedErr string
	}{
		{
			name:        "A 502 is a query timeout and is not retried",
			status:      http.StatusBadGateway,
			expectCalls: 1,
			expectedErr: "could not answer the query within its 10s limit: HTTP 502",
		},
		{
			name:        "A 504 is the same",
			status:      http.StatusGatewayTimeout,
			expectCalls: 1,
			expectedErr: "could not answer the query within its 10s limit: HTTP 504",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("X-GitHub-Request-Id", "B1A2:3C4D:5E6F")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, timeoutBody)
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.Endpoint = server.URL

			_, err := client.Fetch(context.Background(), "octocat", time.Now())

			require.Error(t, err, "the failure should be surfaced")
			assert.Contains(t, err.Error(), tc.expectedErr, "error message mismatch")
			require.ErrorIs(t, err, ErrQueryTimeout, "the caller needs to tell a timeout apart to shrink the window")
			assert.Contains(t, err.Error(), "request B1A2:3C4D:5E6F",
				"the request ID is the only handle GitHub support has on a recurring 5xx")
			assert.NotContains(t, err.Error(), "\n", "the body should be flattened onto one progress line")
			assert.Equal(t, tc.expectCalls, calls, "retry count mismatch")
		})
	}
}

// TestDoKeepsOtherServerErrorsRetryable guards the other half of that split. It
// calls do rather than Fetch so the retry loop's 2s+4s backoff never runs.
func TestDoKeepsOtherServerErrorsRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.Endpoint = server.URL

	_, err := client.do(context.Background(), map[string]any{})

	require.Error(t, err, "a 503 should be an error")
	assert.True(t, isTransient(err), "a 503 is a blip, not a query too expensive to retry")
	assert.Contains(t, err.Error(), "GitHub returned HTTP 503", "the status should be named")
}

func TestFetchErrors(t *testing.T) {
	testCases := []struct {
		name        string
		status      int
		body        string
		expectedErr string
		expectedIs  error
	}{
		{
			name:        "Unknown user",
			status:      http.StatusOK,
			body:        `{"data":{"user":null,"issueSearch":{"nodes":[]}}}`,
			expectedErr: "user not found: ghost",
			expectedIs:  ErrUserNotFound,
		},
		{
			name:        "Unknown user reported as a GraphQL error",
			status:      http.StatusOK,
			body:        `{"data":{"user":null},"errors":[{"type":"NOT_FOUND","path":["user"],"message":"Could not resolve to a User with the login of 'ghost'."}]}`,
			expectedErr: "Could not resolve to a User",
			expectedIs:  ErrUserNotFound,
		},
		{
			name:        "Rejected token",
			status:      http.StatusUnauthorized,
			body:        `{"message":"Bad credentials"}`,
			expectedErr: "GitHub rejected the token",
		},
		{
			name:        "GraphQL error",
			status:      http.StatusOK,
			body:        `{"errors":[{"message":"Something went wrong"}]}`,
			expectedErr: "Something went wrong",
		},
		{
			name:   "GraphQL error alongside a SAML-hidden resource",
			status: http.StatusOK,
			body: `{"data":{"user":{"login":"ghost"}},"errors":[` + samlError +
				`,{"message":"Something went wrong"}]}`,
			expectedErr: "Something went wrong",
		},
		{
			name:        "SAML error without any data",
			status:      http.StatusOK,
			body:        `{"errors":[` + samlError + `]}`,
			expectedErr: "SAML enforcement",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.Endpoint = server.URL

			_, err := client.Fetch(context.Background(), "ghost", time.Now())

			require.Error(t, err, "the failure should be surfaced")
			assert.Contains(t, err.Error(), tc.expectedErr, "error message mismatch")
			if tc.expectedIs != nil {
				assert.ErrorIs(t, err, tc.expectedIs, "the web app maps this error to a 404")
			}
		})
	}
}

// samlError is what GitHub sends for a resource in an organization that
// enforces SAML single sign-on, when the token has not been authorized for it.
const samlError = `{"type":"FORBIDDEN","path":["user","pullRequests","nodes",0],` +
	`"message":"Resource protected by organization SAML enforcement. You must grant your OAuth token access to this organization."}`

func TestFetchToleratesSAMLHiddenResources(t *testing.T) {
	since := time.Date(2025, 3, 23, 0, 0, 0, 0, time.UTC)
	// Every section gets the same body, so each of the four requests reports
	// one hidden resource and nulls out one PR and one comment.
	client, _ := stubAPI(t, `{"data":{"user":{"login":"octocat","createdAt":"2020-01-01T00:00:00Z",
		"followers":{"totalCount":1},"repositories":{"totalCount":1},
		"contributionsCollection":{"contributionCalendar":{"weeks":[]},"pullRequestReviewContributions":{"totalCount":0}},
		"pullRequests":{"totalCount":2,"nodes":[null,{"state":"MERGED","merged":true,"createdAt":"2025-06-01T00:00:00Z"}],
			"pageInfo":{"hasNextPage":false}},
		"issueComments":{"totalCount":2,"nodes":[{"createdAt":"2025-06-01T00:00:00Z","reactions":{"nodes":[]}},null],
			"pageInfo":{"hasNextPage":false}}},
		"issueSearch":{"issueCount":1,"nodes":[null]},
		"rateLimit":{"remaining":4900,"resetAt":"2026-01-01T00:00:00Z","used":100}},
		"errors":[`+samlError+`]}`)

	c, err := client.Fetch(context.Background(), "octocat", since)

	require.NoError(t, err, "a SAML-hidden resource should not fail the whole fetch")
	assert.Len(t, c.PullRequests, 1, "the hidden PR should be skipped, the visible one kept")
	assert.Len(t, c.IssueComments, 1, "the hidden comment should be skipped, not kept as a zero-dated one")
	assert.Empty(t, c.Issues, "a hidden search result should be skipped")
	assert.Equal(t, 4, c.HiddenBySAML, "each request's SAML error should be counted")
}

func TestReachedBeforeSkipsHiddenNodes(t *testing.T) {
	since := time.Date(2025, 3, 23, 0, 0, 0, 0, time.UTC)
	nodes := []*prNode{{CreatedAt: since.AddDate(0, 1, 0)}, nil}

	assert.False(t, reachedBefore(nodes, since), "a trailing hidden PR has no date and must not stop the pagination")
}

func TestFallbackWindows(t *testing.T) {
	testCases := []struct {
		name     string
		months   int
		expected []int
	}{
		{name: "Default window falls back through every step", months: 12, expected: []int{12, 6, 3, 1}},
		{name: "A window on a step skips that step", months: 6, expected: []int{6, 3, 1}},
		{name: "A window between steps keeps the smaller ones", months: 4, expected: []int{4, 3, 1}},
		{name: "A short window falls back to one month", months: 2, expected: []int{2, 1}},
		{name: "One month has nothing to fall back to", months: 1, expected: []int{1}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, fallbackWindows(tc.months), "fallback sequence mismatch")
		})
	}
}
