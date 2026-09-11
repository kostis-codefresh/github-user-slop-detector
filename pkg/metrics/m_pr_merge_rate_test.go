package metrics

import (
	"testing"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/stretchr/testify/assert"
)

func TestPRMergeRate(t *testing.T) {
	testCases := []struct {
		name           string
		prs            []analysis.PullRequest
		threshold      float64
		expectedValue  string
		expectedPassed bool
	}{
		{
			name:           "Happy Path scenario",
			prs:            []analysis.PullRequest{mergedPR("a/b", 10, "maint"), mergedPR("a/b", 10, "maint"), closedPR("a/b")},
			threshold:      0.3,
			expectedValue:  "67%",
			expectedPassed: true,
		},
		{
			name:           "Problematic scenario",
			prs:            []analysis.PullRequest{mergedPR("a/b", 10, "maint"), closedPR("a/b"), closedPR("a/b"), closedPR("a/b")},
			threshold:      0.3,
			expectedValue:  "25%",
			expectedPassed: false,
		},
		{
			// Open PRs have not been judged yet, so they must not drag the
			// rate down the way a rejection would.
			name:           "Open PRs stay out of the denominator",
			prs:            []analysis.PullRequest{mergedPR("a/b", 10, "maint"), openPR("a/b"), openPR("a/b")},
			threshold:      0.9,
			expectedValue:  "100%",
			expectedPassed: true,
		},
		{
			name:           "No PRs passes a zero threshold",
			prs:            nil,
			threshold:      0,
			expectedValue:  "0%",
			expectedPassed: true,
		},
		{
			name:           "No PRs cannot clear a real threshold",
			prs:            nil,
			threshold:      0.3,
			expectedValue:  "0%",
			expectedPassed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(func(c *analysis.Contributor) { c.PullRequests = tc.prs })
			cfg := testConfig(func(cfg *Config) { cfg.Thresholds.PRMergeRate = tc.threshold })

			result := (&PRMergeRate{}).Evaluate(c, cfg)

			assert.True(t, result.Active, "prMergeRate should always be active")
			assert.Equal(t, tc.expectedValue, result.Value, "prMergeRate value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "prMergeRate passed status mismatch")
		})
	}
}

func TestPRMergeRateDetail(t *testing.T) {
	t.Run("Happy Path scenario", func(t *testing.T) {
		c := contributor(func(c *analysis.Contributor) {
			c.PullRequests = []analysis.PullRequest{mergedPR("a/b", 10, "maint"), closedPR("a/b"), openPR("a/b")}
		})

		detail := (&PRMergeRate{}).Detail(c, testConfig())

		assert.Equal(t, "📊 50% merge rate (1/3 PRs)", detail.Summary, "summary mismatch")
		assert.Contains(t, detail.Body, "- Merged: 1", "merged count missing")
		assert.Contains(t, detail.Body, "- Closed without merge: 1", "closed count missing")
		assert.Contains(t, detail.Body, "- Open: 1", "open count missing")
		assert.Contains(t, detail.Body, "- Avg PR size: 29.7 lines", "average size should be rounded")
		assert.Contains(t, detail.Body, "- Very short PRs (<10 lines): 1", "short PR count missing")
	})

	t.Run("Empty data scenario", func(t *testing.T) {
		detail := (&PRMergeRate{}).Detail(contributor(), testConfig())

		assert.Equal(t, "📊 No pull requests found", detail.Summary, "empty summary mismatch")
		assert.NotContains(t, detail.Body, "Very short", "empty body should not list short PRs")
	})
}
