// Repo Quality: whether the contributor lands work in projects with an audience.
package metrics

import (
	"fmt"

	"github.com/kkp/contributor-report/pkg/analysis"
)

// RepoQuality counts repositories above the star bar where the contributor has
// merged at least one PR.
type RepoQuality struct{}

func (m *RepoQuality) Metadata(cfg Config) Metadata {
	return Metadata{
		Key:         "repoQuality",
		Name:        "Repo Quality",
		DocSlug:     "repo-quality",
		Description: fmt.Sprintf("Repos with ≥%d stars", cfg.MinimumStars),
	}
}

func (m *RepoQuality) Evaluate(c *analysis.Contributor, cfg Config) Result {
	repos := c.ContributedRepos()
	quality := 0
	for _, repo := range repos {
		if repo.Stars >= cfg.MinimumStars {
			quality++
		}
	}
	return Result{
		Active:     true,
		Passed:     quality >= cfg.Thresholds.RepoQuality,
		Value:      count(quality),
		Threshold:  atLeast(cfg.Thresholds.RepoQuality),
		DataPoints: len(repos),
		Recommendation: "Consider contributing to established open source projects " +
			"with significant community adoption.",
	}
}

func (m *RepoQuality) DetailGroup() string { return "repoQuality" }

func (m *RepoQuality) Detail(c *analysis.Contributor, cfg Config) Detail {
	repos := c.TopReposByStars()
	if len(repos) == 0 {
		return Detail{
			Summary: "📊 No repository contributions found",
			Body:    "No merged PRs found in the analysis window.",
		}
	}

	quality, totalStars, highest := 0, 0, 0
	for _, repo := range repos {
		if repo.Stars >= cfg.MinimumStars {
			quality++
		}
		totalStars += repo.Stars
		if repo.Stars > highest {
			highest = repo.Stars
		}
	}

	lines := []string{"| Repository | Stars | PRs |", "|------------|-------|-----|"}
	shown := repos
	if len(shown) > 5 {
		shown = shown[:5]
	}
	for _, repo := range shown {
		lines = append(lines, fmt.Sprintf("| %s | %s | %d |", repo.Repo(), compactStars(repo.Stars), repo.MergedPR))
	}
	if len(repos) > len(shown) {
		lines = append(lines, "", fmt.Sprintf("_...and %d more repositories_", len(repos)-len(shown)))
	}
	lines = append(lines, "", fmt.Sprintf("Avg stars: %d | Highest: %d", totalStars/len(repos), highest))

	return Detail{
		Summary: fmt.Sprintf("📊 %d quality repo%s found", quality, plural(quality)),
		Body:    bullets(lines...),
	}
}
