package analysis

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

	t.Run("Sends the expected search query", func(t *testing.T) {
		require.NotEmpty(t, *requests, "at least one request should have been made")
		assert.Equal(t, "author:octocat is:issue created:>=2025-03-23", (*requests)[0]["issueSearchQuery"],
			"the issue search query should scope to the author and window")
	})
}

func TestFetchPaginates(t *testing.T) {
	// A first page that claims more data, then a page that ends it.
	first := `{"data":{"user":{"login":"octocat","createdAt":"2020-01-01T00:00:00Z",
		"followers":{"totalCount":1},"repositories":{"totalCount":1},
		"pullRequests":{"totalCount":1,"nodes":[{"state":"OPEN","createdAt":"2025-06-01T00:00:00Z",
		"repository":{"owner":{"login":"org"},"name":"a","stargazerCount":1}}],
		"pageInfo":{"hasNextPage":true,"endCursor":"CURSOR-1"}},
		"contributionsCollection":{"contributionCalendar":{"weeks":[]},"pullRequestReviewContributions":{"totalCount":0}},
		"issueComments":{"totalCount":0,"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}},
		"issueSearch":{"issueCount":0,"nodes":[]},"rateLimit":{"remaining":100,"resetAt":"2026-01-01T00:00:00Z","used":1}}}`

	var pages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		body := first
		if pages > 1 {
			// Second page returns one more PR and stops.
			body = `{"data":{"user":{"login":"octocat","createdAt":"2020-01-01T00:00:00Z",
				"followers":{"totalCount":1},"repositories":{"totalCount":1},
				"pullRequests":{"totalCount":1,"nodes":[{"state":"OPEN","createdAt":"2025-07-01T00:00:00Z",
				"repository":{"owner":{"login":"org"},"name":"b","stargazerCount":1}}],
				"pageInfo":{"hasNextPage":false,"endCursor":""}},
				"contributionsCollection":{"contributionCalendar":{"weeks":[]},"pullRequestReviewContributions":{"totalCount":0}},
				"issueComments":{"totalCount":0,"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}},
				"issueSearch":{"issueCount":0,"nodes":[]},"rateLimit":{"remaining":100,"resetAt":"2026-01-01T00:00:00Z","used":1}}}`
		}
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.Endpoint = server.URL

	c, err := client.Fetch(context.Background(), "octocat", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err, "fetching should succeed")

	assert.Equal(t, 2, pages, "the second PR page should have been requested")
	assert.Len(t, c.PullRequests, 2, "PRs from both pages should be kept")
}

func TestFetchErrors(t *testing.T) {
	testCases := []struct {
		name        string
		status      int
		body        string
		expectedErr string
	}{
		{
			name:        "Unknown user",
			status:      http.StatusOK,
			body:        `{"data":{"user":null,"issueSearch":{"nodes":[]}}}`,
			expectedErr: "user not found: ghost",
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
		})
	}
}
