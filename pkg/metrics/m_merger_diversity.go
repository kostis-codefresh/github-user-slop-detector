// Merger Diversity: how many different people have vouched for the
// contributor's work by merging it.
package metrics

import (
	"fmt"
	"strings"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
)

// MergerDiversity counts the unique accounts that merged the contributor's PRs.
type MergerDiversity struct{}

func (m *MergerDiversity) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "mergerDiversity",
		Name:        "Merger Diversity",
		DocSlug:     "merger-diversity",
		Description: "Unique maintainers who merged PRs",
	}
}

func (m *MergerDiversity) Evaluate(c *analysis.Contributor, cfg Config) Result {
	stats := collectMergers(c)
	threshold := cfg.Thresholds.MergerDiversity

	if stats.totalMerged == 0 {
		return Result{Active: true, Passed: threshold == 0, Value: "0", Threshold: atLeast(threshold)}
	}

	// Merging only your own work on your own repositories is not diversity at
	// any count, so it fails outright rather than scoring its one merger.
	if stats.onlySelfMergesOnOwnRepos() && threshold > 0 {
		return Result{
			Active:     true,
			Passed:     false,
			Value:      "0",
			Threshold:  atLeast(threshold),
			DataPoints: stats.totalMerged,
			Recommendation: "Build trust by contributing to external projects where other " +
				"maintainers can review and merge your work.",
		}
	}

	return Result{
		Active:     true,
		Passed:     stats.uniqueMergers >= threshold,
		Value:      count(stats.uniqueMergers),
		Threshold:  atLeast(threshold),
		DataPoints: stats.totalMerged,
		Recommendation: "Increase trust signals by contributing to more diverse projects and " +
			"getting PRs merged by different maintainers.",
	}
}

func (m *MergerDiversity) DetailGroup() string { return "mergerDiversity" }

func (m *MergerDiversity) Detail(c *analysis.Contributor, _ Config) Detail {
	stats := collectMergers(c)
	if stats.totalMerged == 0 {
		return Detail{Summary: "👥 No merged PRs", Body: "No merged PRs found in the analysis window."}
	}

	var lines []string
	if len(stats.mergers) > 0 {
		shown := stats.mergers
		suffix := ""
		if len(shown) > 5 {
			shown, suffix = shown[:5], "..."
		}
		lines = append(lines, "**Mergers:** @"+strings.Join(shown, ", @")+suffix, "")
	}
	lines = append(lines,
		fmt.Sprintf("- Self-merges: %d (own repos: %d, external: %d)",
			stats.selfMerges, stats.selfOnOwnRepos, stats.selfOnOtherRepos),
		fmt.Sprintf("- Merged by others: %d", stats.mergedByOthers),
		"- Self-merge rate: "+pct(stats.selfMergeRate()),
	)
	if len(stats.externalRepos) > 0 {
		lines = append(lines, "- External repos with merge rights: "+strings.Join(stats.externalRepos, ", "))
	}
	if stats.onlySelfMergesOnOwnRepos() {
		lines = append(lines, "", "⚠️ All merges are self-merges on own repositories")
	}

	return Detail{
		Summary: fmt.Sprintf("👥 %d unique maintainer%s", stats.uniqueMergers, plural(stats.uniqueMergers)),
		Body:    bullets(lines...),
	}
}
