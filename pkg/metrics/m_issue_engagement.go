// Issue Engagement: whether the issues the contributor opens draw a response.
package metrics

import (
	"fmt"
	"strconv"

	"github.com/kkp/contributor-report/pkg/analysis"
)

// IssueEngagement counts issues that drew comments or reactions.
type IssueEngagement struct{}

func (m *IssueEngagement) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "issueEngagement",
		Name:        "Issue Engagement",
		DocSlug:     "issue-engagement",
		Description: "Issues with community engagement",
	}
}

// issueStats summarises the contributor's opened issues.
type issueStats struct {
	created       int
	withComments  int
	withReactions int
	totalComments int
}

// engaged is the count the metric reports. Comments and reactions overlap on
// the same issue, so the larger of the two is used rather than their sum.
func (s issueStats) engaged() int {
	return max(s.withComments, s.withReactions)
}

func (s issueStats) averageComments() float64 {
	if s.created == 0 {
		return 0
	}
	return float64(s.totalComments) / float64(s.created)
}

func collectIssues(c *analysis.Contributor) issueStats {
	stats := issueStats{created: len(c.Issues)}
	for _, issue := range c.Issues {
		if issue.Comments > 0 {
			stats.withComments++
		}
		if len(issue.Reactions) > 0 {
			stats.withReactions++
		}
		stats.totalComments += issue.Comments
	}
	return stats
}

func (m *IssueEngagement) Evaluate(c *analysis.Contributor, cfg Config) Result {
	stats := collectIssues(c)
	return Result{
		Active:         true,
		Passed:         stats.engaged() >= cfg.Thresholds.IssueEngagement,
		Value:          count(stats.engaged()),
		Threshold:      atLeast(cfg.Thresholds.IssueEngagement),
		DataPoints:     stats.created,
		Recommendation: "Create issues to report bugs or suggest features, and engage with the community.",
	}
}

func (m *IssueEngagement) DetailGroup() string { return "issueEngagement" }

func (m *IssueEngagement) Detail(c *analysis.Contributor, _ Config) Detail {
	stats := collectIssues(c)
	if stats.created == 0 {
		return Detail{Summary: "💬 No issues created", Body: "No issues found in the analysis window."}
	}
	return Detail{
		Summary: fmt.Sprintf("💬 %d issue%s with engagement", stats.engaged(), plural(stats.engaged())),
		Body: bullets(
			fmt.Sprintf("- Issues created: %d", stats.created),
			fmt.Sprintf("- With comments from others: %d", stats.withComments),
			fmt.Sprintf("- With reactions: %d", stats.withReactions),
			"- Avg comments per issue: "+strconv.FormatFloat(stats.averageComments(), 'f', 1, 64),
		),
	}
}
