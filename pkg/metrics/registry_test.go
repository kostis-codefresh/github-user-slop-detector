package metrics

import (
	"strings"
	"testing"

	"github.com/kkp/contributor-report/pkg/analysis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistryIsWellFormed guards the hand-maintained registry slice: a metric
// added without a key, or with one that collides, would otherwise silently
// break --required matching and the report table.
func TestRegistryIsWellFormed(t *testing.T) {
	cfg := DefaultConfig()
	seen := make(map[string]bool)

	for _, metric := range NewRegistry() {
		meta := metric.Metadata(cfg)

		require.NotEmpty(t, meta.Key, "a metric is missing its Key")
		require.NotEmpty(t, meta.Name, "metric %s has no display name", meta.Key)
		require.NotEmpty(t, meta.DocSlug, "metric %s has no DocSlug", meta.Key)
		require.NotEmpty(t, meta.Description, "metric %s has no description", meta.Key)
		assert.NotEmpty(t, metric.DetailGroup(), "metric %s has no detail group", meta.Key)

		assert.False(t, seen[meta.Key], "metric key %q is registered twice", meta.Key)
		seen[meta.Key] = true

		assert.True(t, strings.HasPrefix(meta.DocumentationURL(), docsBaseURL),
			"metric %s builds an unexpected documentation URL", meta.Key)
		assert.True(t, strings.HasSuffix(meta.DocumentationURL(), ".md"),
			"metric %s should link to a markdown page", meta.Key)
	}
}

// TestDefaultRequiredMetricsExist stops the default verdict from silently
// depending on a metric name that no longer exists.
func TestDefaultRequiredMetricsExist(t *testing.T) {
	cfg := DefaultConfig()
	registered := make(map[string]bool)
	for _, metric := range NewRegistry() {
		registered[metric.Metadata(cfg).Key] = true
	}

	for _, name := range cfg.Required {
		assert.True(t, registered[name], "default --required names %q, which is not a registered metric", name)
	}
}

func TestEvaluateCountsOnlyActiveMetrics(t *testing.T) {
	c := contributor(agedDays(900), activeIn("2025-01"))

	t.Run("Without a repo the repo-history metrics drop out", func(t *testing.T) {
		report := Evaluate(c, testConfig())

		assert.Equal(t, 11, report.TotalCount, "two repo-history metrics should be inactive")
		assert.Len(t, report.Evaluations, 13, "every metric still gets a table row")
	})

	t.Run("With a repo every metric counts", func(t *testing.T) {
		report := Evaluate(c, testConfig(func(cfg *Config) { cfg.Repo = "org/project" }))

		assert.Equal(t, 13, report.TotalCount, "all thirteen metrics should be active")
	})
}

func TestEvaluateVerdictFollowsRequiredMetricsOnly(t *testing.T) {
	// Fails repo quality but has a healthy merge rate and an old account.
	c := contributor(agedDays(900), func(c *analysis.Contributor) {
		c.PullRequests = []analysis.PullRequest{mergedPR("tiny/repo", 1, "maint")}
	})

	t.Run("A failure outside the required set does not fail the run", func(t *testing.T) {
		report := Evaluate(c, testConfig())

		assert.True(t, report.Passed, "only prMergeRate and accountAge gate the default verdict")
		assert.Less(t, report.PassedCount, report.TotalCount, "other metrics still report their failures")
	})

	t.Run("A failure inside the required set fails the run", func(t *testing.T) {
		report := Evaluate(c, testConfig(func(cfg *Config) { cfg.Required = []string{"repoQuality"} }))

		assert.False(t, report.Passed, "the required metric failed")
	})

	t.Run("An empty required set never fails the run", func(t *testing.T) {
		report := Evaluate(c, testConfig(func(cfg *Config) { cfg.Required = nil }))

		assert.True(t, report.Passed, "with nothing required there is nothing to fail")
	})
}

func TestEvaluateSharesDetailBlocksBetweenMetrics(t *testing.T) {
	report := Evaluate(contributor(agedDays(900)), testConfig(func(cfg *Config) { cfg.Repo = "org/project" }))

	// Thirteen metrics collapse into ten blocks: reactions, account and
	// repo history each cover two metrics.
	assert.Len(t, report.Details, 10, "detail blocks should be deduplicated by group")

	groups := make(map[string]bool)
	for _, block := range report.Details {
		assert.False(t, groups[block.Group], "detail group %q rendered twice", block.Group)
		groups[block.Group] = true
	}
}

func TestEvaluateMarksSharedBlockFailedWhenEitherMetricFails(t *testing.T) {
	// Plenty of negative reactions, no positive ones: the negative metric
	// fails while the positive one does too, but the point is that the shared
	// block inherits the failure rather than the first metric's verdict.
	c := contributor(agedDays(900), withReactions("THUMBS_DOWN", "THUMBS_DOWN", "THUMBS_DOWN",
		"THUMBS_DOWN", "THUMBS_DOWN", "THUMBS_DOWN"))

	report := Evaluate(c, testConfig())

	var reactions DetailBlock
	for _, block := range report.Details {
		if block.Group == "reactions" {
			reactions = block
		}
	}
	assert.True(t, reactions.Failed, "the shared block should be marked failed")
}

func TestRecommendationsCollapseOnCriticalSpam(t *testing.T) {
	c := contributor(agedDays(10), manyPRs(30, 15, 1, "octocat"))

	report := Evaluate(c, testConfig())

	require.Len(t, report.Recommendations, 1, "a critical spam finding should replace all other advice")
	assert.Contains(t, report.Recommendations[0], "CRITICAL:", "the single recommendation should be the spam warning")
}

func TestLimitedDataWarnsOnThinEvidence(t *testing.T) {
	t.Run("Problematic scenario", func(t *testing.T) {
		report := Evaluate(contributor(agedDays(900)), testConfig())

		assert.True(t, report.HasLimitedData, "an empty account has almost no evidence behind it")
	})

	t.Run("Happy Path scenario", func(t *testing.T) {
		c := contributor(agedDays(900), manyPRs(10, 3, 500, "maint"))

		report := Evaluate(c, testConfig())

		assert.False(t, report.HasLimitedData, "ten PRs is enough to judge")
	})
}

func TestNewAccountIsFlagged(t *testing.T) {
	report := Evaluate(contributor(agedDays(5)), testConfig())

	assert.True(t, report.IsNewAccount, "a five-day-old account is new")
	assert.Equal(t, 30, report.NewAccountDays, "the threshold should travel with the report")
}
