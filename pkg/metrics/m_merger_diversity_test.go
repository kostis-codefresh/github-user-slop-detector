package metrics

import (
	"testing"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/stretchr/testify/assert"
)

func TestMergerDiversity(t *testing.T) {
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
				mergedPR("org/one", 100, "maintainer-a"),
				mergedPR("org/two", 100, "maintainer-b"),
			},
			threshold:      2,
			expectedValue:  "2",
			expectedPassed: true,
		},
		{
			name:           "Problematic scenario",
			prs:            []analysis.PullRequest{mergedPR("org/one", 100, "maintainer-a")},
			threshold:      2,
			expectedValue:  "1",
			expectedPassed: false,
		},
		{
			// Merging your own work on your own repository is not external
			// validation at any count, so it fails outright.
			name: "Self-merges on own repos fail outright",
			prs: []analysis.PullRequest{
				mergedPR("octocat/one", 5, "octocat"),
				mergedPR("octocat/two", 5, "octocat"),
			},
			threshold:      1,
			expectedValue:  "0",
			expectedPassed: false,
		},
		{
			// Being able to merge into someone else's project is a trust
			// signal, not a red flag.
			name:           "Self-merges on external repos are not penalised",
			prs:            []analysis.PullRequest{mergedPR("someone-else/repo", 500, "octocat")},
			threshold:      1,
			expectedValue:  "1",
			expectedPassed: true,
		},
		{
			name:           "Merger login case does not matter",
			prs:            []analysis.PullRequest{mergedPR("org/one", 100, "OctoCat")},
			threshold:      1,
			expectedValue:  "1",
			expectedPassed: true,
		},
		{
			name:           "No merged PRs passes a zero threshold",
			prs:            []analysis.PullRequest{closedPR("org/one")},
			threshold:      0,
			expectedValue:  "0",
			expectedPassed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(func(c *analysis.Contributor) { c.PullRequests = tc.prs })
			cfg := testConfig(func(cfg *Config) { cfg.Thresholds.MergerDiversity = tc.threshold })

			result := (&MergerDiversity{}).Evaluate(c, cfg)

			assert.Equal(t, tc.expectedValue, result.Value, "mergerDiversity value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "mergerDiversity passed status mismatch")
		})
	}
}

func TestMergerDiversityCountsDeletedMergers(t *testing.T) {
	// A deleted account still merged the PR, so it belongs in the total even
	// though it cannot be credited as a distinct maintainer.
	c := contributor(func(c *analysis.Contributor) {
		c.PullRequests = []analysis.PullRequest{
			mergedPR("org/one", 100, "maintainer-a"),
			mergedPR("org/two", 100, ""),
		}
	})

	result := (&MergerDiversity{}).Evaluate(c, testConfig())

	assert.Equal(t, "1", result.Value, "an unknown merger is not a distinct maintainer")
	assert.Equal(t, 2, result.DataPoints, "both merges are still evidence")
}

func TestMergerDiversityDetail(t *testing.T) {
	t.Run("Happy Path scenario", func(t *testing.T) {
		c := contributor(func(c *analysis.Contributor) {
			c.PullRequests = []analysis.PullRequest{
				mergedPR("octocat/own", 5, "octocat"),
				mergedPR("org/other", 100, "maintainer-a"),
				mergedPR("trusted/repo", 300, "octocat"),
			}
		})

		detail := (&MergerDiversity{}).Detail(c, testConfig())

		assert.Equal(t, "👥 2 unique maintainers", detail.Summary, "summary mismatch")
		assert.Equal(t, []Block{
			para(Line{Label: "Mergers:", Text: "@octocat, @maintainer-a"}),
			list(
				"Self-merges: 2 (own repos: 1, external: 1)",
				"Merged by others: 1",
				"Self-merge rate: 67%",
				"External repos with merge rights: trusted/repo",
			),
		}, detail.Blocks, "merger detail body mismatch")
	})

	t.Run("Problematic scenario warns about self-merges", func(t *testing.T) {
		c := contributor(func(c *analysis.Contributor) {
			c.PullRequests = []analysis.PullRequest{mergedPR("octocat/own", 5, "octocat")}
		})

		detail := (&MergerDiversity{}).Detail(c, testConfig())

		assert.Equal(t, text("⚠️ All merges are self-merges on own repositories"),
			detail.Blocks[len(detail.Blocks)-1], "warning missing")
	})

	t.Run("Empty data scenario", func(t *testing.T) {
		detail := (&MergerDiversity{}).Detail(contributor(), testConfig())

		assert.Equal(t, "👥 No merged PRs", detail.Summary, "empty summary mismatch")
	})
}
