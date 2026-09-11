// Repo History: the contributor's track record in one specific repository.
// Both metrics describe the same repository, so they share a <details> block,
// and both go inactive when the CLI is run without --repo.
package metrics

import (
	"fmt"

	"github.com/kkp/contributor-report/pkg/analysis"
)

// repoHistoryDetail is the <details> block shared by both repo-history metrics.
type repoHistoryDetail struct{}

func (repoHistoryDetail) DetailGroup() string { return "repoHistory" }

func (repoHistoryDetail) Detail(c *analysis.Contributor, cfg Config) Detail {
	history := collectRepoHistory(c, cfg.Repo)
	if history.firstTime() {
		return Detail{
			Summary: "📁 First-time contributor",
			Body:    bullets("Repository: "+history.repo, "This is a first contribution to this repository."),
		}
	}
	return Detail{
		Summary: fmt.Sprintf("📁 %d previous PR%s in this repo", history.total, plural(history.total)),
		Body: bullets(
			"- Repository: "+history.repo,
			fmt.Sprintf("- Merged: %d", history.merged),
			fmt.Sprintf("- Closed without merge: %d", history.closedUnmerged),
			"- Repo merge rate: "+pct(history.mergeRate()),
		),
	}
}

// RepoHistoryMergeRate measures how much of the contributor's work in this
// repository has landed.
type RepoHistoryMergeRate struct{ repoHistoryDetail }

func (m *RepoHistoryMergeRate) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "repoHistoryMergeRate",
		Name:        "Repo History Merge Rate",
		DocSlug:     "repo-history",
		Description: "Merge rate in this repo",
	}
}

func (m *RepoHistoryMergeRate) Evaluate(c *analysis.Contributor, cfg Config) Result {
	if cfg.Repo == "" {
		return Result{}
	}
	history := collectRepoHistory(c, cfg.Repo)
	threshold := cfg.Thresholds.RepoHistoryMergeRate

	// No prior PRs, or none resolved yet, means there is no rate to judge.
	if history.resolved() == 0 {
		return Result{
			Active:     true,
			Passed:     threshold == 0,
			Value:      "0%",
			Threshold:  atLeastPct(threshold),
			DataPoints: history.total,
		}
	}

	result := Result{
		Active:     true,
		Passed:     history.mergeRate() >= threshold,
		Value:      pct(history.mergeRate()),
		Threshold:  atLeastPct(threshold),
		DataPoints: history.total,
		Recommendation: "Review previous rejected PRs in this repository to understand " +
			"maintainer expectations.",
	}
	if history.firstTime() {
		result.Recommendation = "Welcome! This is a first contribution to this repository."
	}
	return result
}

// RepoHistoryMinPRs counts the contributor's prior PRs to this repository.
type RepoHistoryMinPRs struct{ repoHistoryDetail }

func (m *RepoHistoryMinPRs) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "repoHistoryMinPRs",
		Name:        "Repo History Min PRs",
		DocSlug:     "repo-history",
		Description: "Previous PRs in this repo",
	}
}

func (m *RepoHistoryMinPRs) Evaluate(c *analysis.Contributor, cfg Config) Result {
	if cfg.Repo == "" {
		return Result{}
	}
	history := collectRepoHistory(c, cfg.Repo)
	return Result{
		Active:         true,
		Passed:         history.total >= cfg.Thresholds.RepoHistoryMinPRs,
		Value:          count(history.total),
		Threshold:      atLeast(cfg.Thresholds.RepoHistoryMinPRs),
		DataPoints:     history.total,
		Recommendation: "Build trust by making consistent, quality contributions to this repository over time.",
	}
}
