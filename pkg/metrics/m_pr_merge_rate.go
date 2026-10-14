// PR Merge Rate: how much of the contributor's submitted work actually lands.
package metrics

import (
	"fmt"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
)

// PRMergeRate measures merged PRs against those closed without merging.
type PRMergeRate struct{}

func (m *PRMergeRate) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "prMergeRate",
		Name:        "PR Merge Rate",
		DocSlug:     "pr-merge-rate",
		Description: "PRs merged vs closed",
	}
}

func (m *PRMergeRate) Evaluate(c *analysis.Contributor, cfg Config) Result {
	stats := collectPRStats(c.PullRequests)
	threshold := cfg.Thresholds.PRMergeRate

	if stats.total == 0 {
		// No submissions is not evidence of bad submissions.
		return Result{Active: true, Passed: threshold == 0, Value: "0%", Threshold: atLeastPct(threshold)}
	}

	rate := stats.mergeRate()
	return Result{
		Active:     true,
		Passed:     rate >= threshold,
		Value:      pct(rate),
		Threshold:  atLeastPct(threshold),
		DataPoints: stats.total,
		Recommendation: "Improve PR quality to increase merge rate. " +
			"Focus on smaller, well-documented changes.",
	}
}

func (m *PRMergeRate) DetailGroup() string { return "prMergeRate" }

func (m *PRMergeRate) Detail(c *analysis.Contributor, _ Config) Detail {
	stats := collectPRStats(c.PullRequests)
	if stats.total == 0 {
		return Detail{
			Icon:    IconPRs,
			Summary: "No pull requests found",
			Blocks:  []Block{text("No PRs found in the analysis window.")},
		}
	}

	items := []string{
		fmt.Sprintf("Merged: %d", stats.merged),
		fmt.Sprintf("Closed without merge: %d", stats.closedUnmerged),
		fmt.Sprintf("Open: %d", stats.open),
	}
	return Detail{
		Icon:    IconPRs,
		Summary: fmt.Sprintf("%s merge rate (%d/%d PRs)", pct(stats.mergeRate()), stats.merged, stats.total),
		Blocks:  []Block{list(items...)},
	}
}
