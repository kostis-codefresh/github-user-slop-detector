package metrics

import (
	"fmt"
	"testing"

	"github.com/kkp/contributor-report/pkg/analysis"
	"github.com/stretchr/testify/assert"
)

// manyPRs spreads n merged PRs over repoCount repositories with the given
// star count, each merged by the named account.
func manyPRs(n, repoCount, stars int, mergedBy string) func(*analysis.Contributor) {
	return func(c *analysis.Contributor) {
		for i := range n {
			owner := "octocat"
			if mergedBy != "octocat" {
				owner = "org"
			}
			repo := fmt.Sprintf("%s/repo-%d", owner, i%repoCount)
			c.PullRequests = append(c.PullRequests, mergedPR(repo, stars, mergedBy))
		}
	}
}

func TestSpamPattern(t *testing.T) {
	testCases := []struct {
		name      string
		ageDays   int
		prCount   int
		repoCount int
		expected  bool
	}{
		{name: "Problematic scenario", ageDays: 10, prCount: 30, repoCount: 15, expected: true},
		{name: "Happy Path scenario", ageDays: 900, prCount: 30, repoCount: 15, expected: false},
		{name: "Established account with a burst is not spam", ageDays: 400, prCount: 40, repoCount: 20, expected: false},
		{name: "New account with few PRs is not spam", ageDays: 10, prCount: 5, repoCount: 3, expected: false},
		{name: "New account focused on one repo is not spam", ageDays: 10, prCount: 30, repoCount: 2, expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(agedDays(tc.ageDays), manyPRs(tc.prCount, tc.repoCount, 500, "maint"))

			detection := (&spamPattern{}).Detect(c, testConfig())

			assert.Equal(t, tc.expected, detection != nil, "SPAM_PATTERN detection mismatch")
			if tc.expected {
				assert.Equal(t, SeverityCritical, detection.Severity, "SPAM_PATTERN must be critical")
			}
		})
	}
}

func TestHighPRRate(t *testing.T) {
	testCases := []struct {
		name     string
		ageDays  int
		prCount  int
		expected bool
	}{
		{name: "Problematic scenario", ageDays: 10, prCount: 30, expected: true},
		{name: "Happy Path scenario", ageDays: 365, prCount: 30, expected: false},
		{
			// A same-day account is treated as having produced everything in
			// one day rather than dividing by zero.
			name: "Brand new account does not divide by zero", ageDays: 0, prCount: 3, expected: true,
		},
		{name: "Brand new account with one PR is fine", ageDays: 0, prCount: 1, expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(agedDays(tc.ageDays), manyPRs(tc.prCount, 3, 500, "maint"))

			detection := (&highPRRate{}).Detect(c, testConfig())

			assert.Equal(t, tc.expected, detection != nil, "HIGH_PR_RATE detection mismatch")
			if tc.expected {
				assert.Equal(t, SeverityWarning, detection.Severity, "HIGH_PR_RATE is only a warning")
			}
		})
	}
}

func TestSelfMergeAbuse(t *testing.T) {
	t.Run("Problematic scenario", func(t *testing.T) {
		// Twenty self-merges into personal repos nobody stars.
		c := contributor(manyPRs(20, 10, 1, "octocat"))

		detection := (&selfMergeAbuse{}).Detect(c, testConfig())

		assert.NotNil(t, detection, "self-merging into dead repos should be detected")
		assert.Equal(t, SeverityCritical, detection.Severity, "SELF_MERGE_ABUSE must be critical")
	})

	t.Run("Happy Path scenario", func(t *testing.T) {
		c := contributor(manyPRs(20, 10, 500, "maint"))

		detection := (&selfMergeAbuse{}).Detect(c, testConfig())

		assert.Nil(t, detection, "PRs merged by others should never trigger self-merge abuse")
	})

	t.Run("Self-merging into popular repos is not abuse", func(t *testing.T) {
		// A maintainer of a well-starred project merges their own work; that
		// is ordinary, not manufactured history.
		c := contributor(manyPRs(20, 10, 5000, "octocat"))

		detection := (&selfMergeAbuse{}).Detect(c, testConfig())

		assert.Nil(t, detection, "high-star repos should not count as low quality")
	})

	t.Run("Inactive scenario", func(t *testing.T) {
		detection := (&selfMergeAbuse{}).Detect(contributor(), testConfig())

		assert.Nil(t, detection, "no merged PRs means nothing to judge")
	})
}

func TestRepoSpam(t *testing.T) {
	testCases := []struct {
		name      string
		repoCount int
		stars     int
		expected  bool
	}{
		{name: "Problematic scenario", repoCount: 15, stars: 1, expected: true},
		{name: "Happy Path scenario", repoCount: 15, stars: 500, expected: false},
		{name: "Few repositories are never repo spam", repoCount: 4, stars: 1, expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(manyPRs(tc.repoCount*2, tc.repoCount, tc.stars, "maint"))

			detection := (&repoSpam{}).Detect(c, testConfig())

			assert.Equal(t, tc.expected, detection != nil, "REPO_SPAM detection mismatch")
			if tc.expected {
				assert.Equal(t, SeverityWarning, detection.Severity, "REPO_SPAM is only a warning")
			}
		})
	}
}

func TestSuspiciousPatternsMetric(t *testing.T) {
	t.Run("Happy Path scenario", func(t *testing.T) {
		c := contributor(agedDays(900), manyPRs(10, 3, 500, "maint"))

		result := (&SuspiciousPatterns{}).Evaluate(c, testConfig())

		assert.True(t, result.Active, "spam detection is on by default")
		assert.True(t, result.Passed, "an ordinary contributor should pass")
		assert.Equal(t, "0", result.Value, "no patterns means zero")
		assert.Equal(t, "N/A", result.Threshold, "the metric has no configurable threshold")
	})

	t.Run("Problematic scenario fails on a critical finding", func(t *testing.T) {
		c := contributor(agedDays(10), manyPRs(30, 15, 1, "octocat"))

		result := (&SuspiciousPatterns{}).Evaluate(c, testConfig())

		assert.False(t, result.Passed, "a critical pattern must fail the metric")
		assert.Contains(t, result.Recommendation, "CRITICAL:", "the recommendation should lead with the severity")
	})

	t.Run("Warnings are reported but still pass", func(t *testing.T) {
		// A high submission rate alone is worth flagging, not failing.
		c := contributor(agedDays(10), manyPRs(30, 2, 500, "maint"))

		result := (&SuspiciousPatterns{}).Evaluate(c, testConfig())

		assert.True(t, result.Passed, "warnings alone should not fail the metric")
		assert.Equal(t, "1", result.Value, "the warning count should be reported")
	})

	t.Run("Inactive scenario", func(t *testing.T) {
		cfg := testConfig(func(cfg *Config) { cfg.SpamDetection = false })

		result := (&SuspiciousPatterns{}).Evaluate(contributor(), cfg)

		assert.False(t, result.Active, "disabling spam detection makes the metric inactive")
	})
}

func TestSuspiciousPatternsDetail(t *testing.T) {
	t.Run("Problematic scenario", func(t *testing.T) {
		c := contributor(agedDays(10), manyPRs(30, 15, 1, "octocat"))

		detail := (&SuspiciousPatterns{}).Detail(c, testConfig())

		assert.Contains(t, detail.Summary, "🚨", "the summary should flag the findings")
		assert.Contains(t, detail.Summary, "critical", "the summary should count criticals")
		assert.Contains(t, detail.Body, "**🔴 CRITICAL: SPAM_PATTERN**", "the critical finding should be headed")
		assert.Contains(t, detail.Body, "- accountAgeDays: 10", "evidence should be listed")
	})

	t.Run("Happy Path scenario", func(t *testing.T) {
		c := contributor(agedDays(900), manyPRs(5, 2, 500, "maint"))

		detail := (&SuspiciousPatterns{}).Detail(c, testConfig())

		assert.Equal(t, "✅ No suspicious patterns detected", detail.Summary, "clean summary mismatch")
		assert.Equal(t, "Activity appears normal.", detail.Body, "clean body mismatch")
	})
}
