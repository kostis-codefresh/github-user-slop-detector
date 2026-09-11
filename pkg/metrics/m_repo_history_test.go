package metrics

import (
	"testing"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/stretchr/testify/assert"
)

// The two repo-history metrics are the only ones that can be inactive, so the
// no-repo case matters as much as the pass and fail cases.

func TestRepoHistoryMergeRate(t *testing.T) {
	testCases := []struct {
		name           string
		repo           string
		prs            []analysis.PullRequest
		threshold      float64
		expectedActive bool
		expectedValue  string
		expectedPassed bool
	}{
		{
			name:           "Happy Path scenario",
			repo:           "org/project",
			prs:            []analysis.PullRequest{mergedPR("org/project", 100, "maint"), closedPR("org/project")},
			threshold:      0.3,
			expectedActive: true,
			expectedValue:  "50%",
			expectedPassed: true,
		},
		{
			name:           "Problematic scenario",
			repo:           "org/project",
			prs:            []analysis.PullRequest{closedPR("org/project"), closedPR("org/project")},
			threshold:      0.3,
			expectedActive: true,
			expectedValue:  "0%",
			expectedPassed: false,
		},
		{
			// Without --repo there is no repository to have a history in, so
			// the metric reports itself inactive rather than failing.
			name:           "Inactive scenario",
			repo:           "",
			prs:            []analysis.PullRequest{mergedPR("org/project", 100, "maint")},
			threshold:      0.3,
			expectedActive: false,
		},
		{
			name:           "PRs to other repositories are ignored",
			repo:           "org/project",
			prs:            []analysis.PullRequest{mergedPR("other/repo", 100, "maint")},
			threshold:      0,
			expectedActive: true,
			expectedValue:  "0%",
			expectedPassed: true,
		},
		{
			name:           "Repository name case does not matter",
			repo:           "ORG/Project",
			prs:            []analysis.PullRequest{mergedPR("org/project", 100, "maint")},
			threshold:      0.5,
			expectedActive: true,
			expectedValue:  "100%",
			expectedPassed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(func(c *analysis.Contributor) { c.PullRequests = tc.prs })
			cfg := testConfig(func(cfg *Config) {
				cfg.Repo = tc.repo
				cfg.Thresholds.RepoHistoryMergeRate = tc.threshold
			})

			result := (&RepoHistoryMergeRate{}).Evaluate(c, cfg)

			assert.Equal(t, tc.expectedActive, result.Active, "repoHistoryMergeRate active status mismatch")
			if !tc.expectedActive {
				return
			}
			assert.Equal(t, tc.expectedValue, result.Value, "repoHistoryMergeRate value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "repoHistoryMergeRate passed status mismatch")
		})
	}
}

func TestRepoHistoryMinPRs(t *testing.T) {
	testCases := []struct {
		name           string
		repo           string
		prs            []analysis.PullRequest
		threshold      int
		expectedActive bool
		expectedValue  string
		expectedPassed bool
	}{
		{
			name:           "Happy Path scenario",
			repo:           "org/project",
			prs:            []analysis.PullRequest{mergedPR("org/project", 100, "maint"), openPR("org/project")},
			threshold:      2,
			expectedActive: true,
			expectedValue:  "2",
			expectedPassed: true,
		},
		{
			name:           "Problematic scenario",
			repo:           "org/project",
			prs:            nil,
			threshold:      1,
			expectedActive: true,
			expectedValue:  "0",
			expectedPassed: false,
		},
		{
			name:           "Inactive scenario",
			repo:           "",
			prs:            []analysis.PullRequest{mergedPR("org/project", 100, "maint")},
			threshold:      1,
			expectedActive: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(func(c *analysis.Contributor) { c.PullRequests = tc.prs })
			cfg := testConfig(func(cfg *Config) {
				cfg.Repo = tc.repo
				cfg.Thresholds.RepoHistoryMinPRs = tc.threshold
			})

			result := (&RepoHistoryMinPRs{}).Evaluate(c, cfg)

			assert.Equal(t, tc.expectedActive, result.Active, "repoHistoryMinPRs active status mismatch")
			if !tc.expectedActive {
				return
			}
			assert.Equal(t, tc.expectedValue, result.Value, "repoHistoryMinPRs value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "repoHistoryMinPRs passed status mismatch")
		})
	}
}

func TestRepoHistoryWelcomesFirstTimers(t *testing.T) {
	c := contributor(func(c *analysis.Contributor) {
		c.PullRequests = []analysis.PullRequest{closedPR("org/project")}
	})
	cfg := testConfig(func(cfg *Config) {
		cfg.Repo = "org/project"
		cfg.Thresholds.RepoHistoryMergeRate = 0.3
	})

	result := (&RepoHistoryMergeRate{}).Evaluate(c, cfg)

	assert.False(t, result.Passed, "nothing has landed here yet")
	assert.Contains(t, result.Recommendation, "Welcome!", "a first-timer should be welcomed, not scolded")
}

func TestRepoHistoryDetail(t *testing.T) {
	cfg := testConfig(func(cfg *Config) { cfg.Repo = "org/project" })

	t.Run("Happy Path scenario", func(t *testing.T) {
		c := contributor(func(c *analysis.Contributor) {
			c.PullRequests = []analysis.PullRequest{
				mergedPR("org/project", 100, "maint"),
				mergedPR("org/project", 100, "maint"),
				closedPR("org/project"),
			}
		})

		detail := (&RepoHistoryMergeRate{}).Detail(c, cfg)

		assert.Equal(t, "📁 3 previous PRs in this repo", detail.Summary, "summary mismatch")
		assert.Contains(t, detail.Body, "- Repository: org/project", "repository line missing")
		assert.Contains(t, detail.Body, "- Merged: 2", "merged count mismatch")
		assert.Contains(t, detail.Body, "- Repo merge rate: 67%", "merge rate mismatch")
	})

	t.Run("First-time contributor scenario", func(t *testing.T) {
		detail := (&RepoHistoryMinPRs{}).Detail(contributor(), cfg)

		assert.Equal(t, "📁 First-time contributor", detail.Summary, "first-timer summary mismatch")
	})
}

func TestRepoHistoryMetricsShareOneDetailBlock(t *testing.T) {
	assert.Equal(t, (&RepoHistoryMergeRate{}).DetailGroup(), (&RepoHistoryMinPRs{}).DetailGroup(),
		"both repo-history metrics must render into the same <details> block")
}
