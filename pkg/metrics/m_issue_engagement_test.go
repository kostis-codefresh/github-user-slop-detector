package metrics

import (
	"testing"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/stretchr/testify/assert"
)

func TestIssueEngagement(t *testing.T) {
	testCases := []struct {
		name           string
		issues         []analysis.Issue
		threshold      int
		expectedValue  string
		expectedPassed bool
	}{
		{
			name:           "Happy Path scenario",
			issues:         []analysis.Issue{{Comments: 3}, {Comments: 1}, {}},
			threshold:      1,
			expectedValue:  "2",
			expectedPassed: true,
		},
		{
			name:           "Problematic scenario",
			issues:         []analysis.Issue{{}, {}, {}},
			threshold:      1,
			expectedValue:  "0",
			expectedPassed: false,
		},
		{
			// Comments and reactions land on the same issues, so the counts
			// overlap and the larger one is reported rather than their sum.
			name: "Overlapping engagement is not double counted",
			issues: []analysis.Issue{
				{Comments: 2, Reactions: []string{"THUMBS_UP"}},
				{Comments: 1, Reactions: []string{"HEART"}},
			},
			threshold:      2,
			expectedValue:  "2",
			expectedPassed: true,
		},
		{
			name:           "Reactions alone count as engagement",
			issues:         []analysis.Issue{{Reactions: []string{"ROCKET"}}},
			threshold:      1,
			expectedValue:  "1",
			expectedPassed: true,
		},
		{
			name:           "No issues scenario",
			issues:         nil,
			threshold:      1,
			expectedValue:  "0",
			expectedPassed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(func(c *analysis.Contributor) { c.Issues = tc.issues })
			cfg := testConfig(func(cfg *Config) { cfg.Thresholds.IssueEngagement = tc.threshold })

			result := (&IssueEngagement{}).Evaluate(c, cfg)

			assert.Equal(t, tc.expectedValue, result.Value, "issueEngagement value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "issueEngagement passed status mismatch")
		})
	}
}

func TestIssueEngagementDetail(t *testing.T) {
	t.Run("Happy Path scenario", func(t *testing.T) {
		c := contributor(func(c *analysis.Contributor) {
			c.Issues = []analysis.Issue{{Comments: 4, Reactions: []string{"HEART"}}, {Comments: 1}, {}}
		})

		detail := (&IssueEngagement{}).Detail(c, testConfig())

		assert.Equal(t, "💬 2 issues with engagement", detail.Summary, "summary mismatch")
		assert.Contains(t, detail.Body, "- Issues created: 3", "issue count missing")
		assert.Contains(t, detail.Body, "- With comments from others: 2", "commented count missing")
		assert.Contains(t, detail.Body, "- With reactions: 1", "reacted count missing")
		assert.Contains(t, detail.Body, "- Avg comments per issue: 1.7", "average mismatch")
	})

	t.Run("Empty data scenario", func(t *testing.T) {
		detail := (&IssueEngagement{}).Detail(contributor(), testConfig())

		assert.Equal(t, "💬 No issues created", detail.Summary, "empty summary mismatch")
		assert.Equal(t, "No issues found in the analysis window.", detail.Body, "empty body mismatch")
	})
}
