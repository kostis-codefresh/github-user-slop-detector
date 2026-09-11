// Account Age and Activity Consistency: how long the account has existed and
// how evenly its activity is spread. Both describe the account itself, so they
// share one <details> block.
package metrics

import (
	"fmt"

	"github.com/kkp/contributor-report/pkg/analysis"
)

// accountDetail is the <details> block shared by both account metrics.
type accountDetail struct{}

func (accountDetail) DetailGroup() string { return "account" }

func (accountDetail) Detail(c *analysis.Contributor, cfg Config) Detail {
	return Detail{
		Summary: fmt.Sprintf("📅 Account: %d days old", c.AgeInDays()),
		Body: bullets(
			"- Created: "+c.CreatedAt.UTC().Format("2006-01-02"),
			fmt.Sprintf("- Active months: %d/%d", c.ActiveMonthCount(), windowMonths(cfg)),
			"- Consistency score: "+pct(consistency(c, cfg)),
		),
	}
}

// windowMonths is the consistency denominator, capped at a year because the
// contribution calendar only ever covers twelve months.
func windowMonths(cfg Config) int {
	return min(cfg.AnalysisWindow, 12)
}

// consistency is the share of the window's months that saw any contribution.
// It can exceed 100%: the calendar is fetched from a date mid-month, so it can
// span one more distinct month than the window nominally covers.
func consistency(c *analysis.Contributor, cfg Config) float64 {
	months := windowMonths(cfg)
	if months == 0 {
		return 0
	}
	return float64(c.ActiveMonthCount()) / float64(months)
}

// AccountAge measures how long the GitHub account has existed.
type AccountAge struct{ accountDetail }

func (m *AccountAge) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "accountAge",
		Name:        "Account Age",
		DocSlug:     "account-age",
		Description: "GitHub account age",
	}
}

func (m *AccountAge) Evaluate(c *analysis.Contributor, cfg Config) Result {
	age := c.AgeInDays()
	return Result{
		Active:         true,
		Passed:         age >= cfg.Thresholds.AccountAge,
		Value:          fmt.Sprintf("%d days", age),
		Threshold:      fmt.Sprintf(">= %d days", cfg.Thresholds.AccountAge),
		DataPoints:     1,
		Recommendation: "Continue building your contribution history. New accounts naturally have limited data.",
	}
}

// ActivityConsistency measures whether contributions are spread over time
// rather than delivered in a single burst.
type ActivityConsistency struct{ accountDetail }

func (m *ActivityConsistency) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "activityConsistency",
		Name:        "Activity Consistency",
		DocSlug:     "activity-consistency",
		Description: "Regular activity over time",
	}
}

func (m *ActivityConsistency) Evaluate(c *analysis.Contributor, cfg Config) Result {
	score := consistency(c, cfg)
	result := Result{
		Active:     true,
		Passed:     score >= cfg.Thresholds.ActivityConsistency,
		Value:      pct(score),
		Threshold:  atLeastPct(cfg.Thresholds.ActivityConsistency),
		DataPoints: c.ActiveMonthCount(),
	}
	// An account too young to have a track record cannot be faulted for not
	// having one, so it gets no advice to act on.
	if c.AgeInDays() >= 90 {
		result.Recommendation = "Maintain consistent activity over time to build a stronger contribution profile."
	}
	return result
}
