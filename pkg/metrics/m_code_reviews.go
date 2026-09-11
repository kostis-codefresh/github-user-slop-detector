// Code Reviews: whether the contributor reviews other people's work.
package metrics

import (
	"fmt"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
)

// CodeReviews counts reviews the contributor gave to others' pull requests.
type CodeReviews struct{}

func (m *CodeReviews) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "codeReviews",
		Name:        "Code Reviews",
		DocSlug:     "code-reviews",
		Description: "Code reviews given to others",
	}
}

func (m *CodeReviews) Evaluate(c *analysis.Contributor, cfg Config) Result {
	return Result{
		Active:         true,
		Passed:         c.ReviewsGiven >= cfg.Thresholds.CodeReviews,
		Value:          count(c.ReviewsGiven),
		Threshold:      atLeast(cfg.Thresholds.CodeReviews),
		DataPoints:     c.ReviewsGiven,
		Recommendation: "Participate in code reviews to demonstrate engagement with the community.",
	}
}

func (m *CodeReviews) DetailGroup() string { return "codeReviews" }

func (m *CodeReviews) Detail(c *analysis.Contributor, _ Config) Detail {
	if c.ReviewsGiven == 0 {
		return Detail{
			Icon:    IconReviews,
			Summary: "No code reviews given",
			Blocks:  []Block{text("No code reviews found in the analysis window.")},
		}
	}
	return Detail{
		Icon:    IconReviews,
		Summary: fmt.Sprintf("%d review%s given", c.ReviewsGiven, plural(c.ReviewsGiven)),
		Blocks: []Block{list(
			fmt.Sprintf("Reviews: %d", c.ReviewsGiven),
			// GitHub's contribution collection exposes no comment count, so
			// this is the same estimate the original action reports.
			fmt.Sprintf("Review comments: %d", c.ReviewsGiven/2),
		)},
	}
}
