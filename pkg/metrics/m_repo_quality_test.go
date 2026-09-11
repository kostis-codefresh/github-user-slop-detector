package metrics

import (
	"testing"

	"github.com/kkp/contributor-report/pkg/analysis"
	"github.com/stretchr/testify/assert"
)

func TestRepoQuality(t *testing.T) {
	testCases := []struct {
		name           string
		prs            []analysis.PullRequest
		threshold      int
		expectedValue  string
		expectedPassed bool
	}{
		{
			name: "Happy Path scenario",
			prs: []analysis.PullRequest{
				mergedPR("big/one", 5000, "maint"),
				mergedPR("big/two", 200, "maint"),
				mergedPR("tiny/three", 1, "maint"),
			},
			threshold:      2,
			expectedValue:  "2",
			expectedPassed: true,
		},
		{
			name:           "Problematic scenario",
			prs:            []analysis.PullRequest{mergedPR("tiny/one", 3, "maint"), mergedPR("tiny/two", 9, "maint")},
			threshold:      2,
			expectedValue:  "0",
			expectedPassed: false,
		},
		{
			// Only merged work counts: an unmerged PR to a popular project
			// says nothing about meeting its standards.
			name:           "Closed PRs do not count as contributions",
			prs:            []analysis.PullRequest{closedPR("big/one")},
			threshold:      1,
			expectedValue:  "0",
			expectedPassed: false,
		},
		{
			name:           "No merged PRs passes a zero threshold",
			prs:            nil,
			threshold:      0,
			expectedValue:  "0",
			expectedPassed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(func(c *analysis.Contributor) { c.PullRequests = tc.prs })
			cfg := testConfig(func(cfg *Config) { cfg.Thresholds.RepoQuality = tc.threshold })

			result := (&RepoQuality{}).Evaluate(c, cfg)

			assert.True(t, result.Active, "repoQuality should always be active")
			assert.Equal(t, tc.expectedValue, result.Value, "repoQuality value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "repoQuality passed status mismatch")
		})
	}
}

func TestRepoQualityMetadataNamesTheStarBar(t *testing.T) {
	meta := (&RepoQuality{}).Metadata(testConfig(func(cfg *Config) { cfg.MinimumStars = 250 }))

	assert.Equal(t, "Repos with ≥250 stars", meta.Description, "description should quote the configured bar")
}

func TestRepoQualityDetail(t *testing.T) {
	t.Run("Happy Path scenario", func(t *testing.T) {
		c := contributor(func(c *analysis.Contributor) {
			c.PullRequests = []analysis.PullRequest{
				mergedPR("small/repo", 12, "maint"),
				mergedPR("huge/repo", 23418, "maint"),
				mergedPR("huge/repo", 23418, "maint"),
			}
		})

		detail := (&RepoQuality{}).Detail(c, testConfig())

		assert.Equal(t, "📊 1 quality repo found", detail.Summary, "summary mismatch")
		// Highest-starred first, with thousands compacted.
		assert.Contains(t, detail.Body, "| huge/repo | 23.4k | 2 |", "top repo row mismatch")
		assert.Contains(t, detail.Body, "| small/repo | 12 | 1 |", "second repo row mismatch")
		assert.Contains(t, detail.Body, "Avg stars: 11715 | Highest: 23418", "summary line mismatch")
	})

	t.Run("Truncates beyond five repositories", func(t *testing.T) {
		c := contributor(func(c *analysis.Contributor) {
			for i, repo := range []string{"a/1", "a/2", "a/3", "a/4", "a/5", "a/6", "a/7"} {
				c.PullRequests = append(c.PullRequests, mergedPR(repo, 100*(i+1), "maint"))
			}
		})

		detail := (&RepoQuality{}).Detail(c, testConfig())

		assert.Contains(t, detail.Body, "_...and 2 more repositories_", "truncation note missing")
	})

	t.Run("Empty data scenario", func(t *testing.T) {
		detail := (&RepoQuality{}).Detail(contributor(), testConfig())

		assert.Equal(t, "📊 No repository contributions found", detail.Summary, "empty summary mismatch")
	})
}
