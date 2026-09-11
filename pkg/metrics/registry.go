package metrics

import (
	"time"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/kostis-codefresh/github-user-slop-detector/pkg/progress"
)

// docsBaseURL is where the upstream metric documentation is published. Metric
// names in the report link there so a reader can see how a number was derived.
const docsBaseURL = "https://github.com/jdiegosierra/contributor-report/blob/main/docs/metrics/"

// minDataPointsForFullReport is the total evidence below which the report warns
// that the analysis was thin.
const minDataPointsForFullReport = 5

// Metadata is a metric's static declaration.
type Metadata struct {
	Key         string //stable identifier, matches --required names
	Name        string //display name in the table
	DocSlug     string //basename of the docs page, without .md
	Description string //the table's Description cell
}

// DocumentationURL returns the public page explaining the metric.
func (m Metadata) DocumentationURL() string { return docsBaseURL + m.DocSlug + ".md" }

// Result is one metric's verdict.
type Result struct {
	// Active reports whether the metric applies at all. An inactive metric —
	// repo history with no --repo, spam detection switched off — is neither a
	// pass nor a failure and is left out of the headline count.
	Active         bool
	Passed         bool
	Value          string //pre-formatted table cell, e.g. "86%", "3261 days"
	Threshold      string //pre-formatted table cell, e.g. ">= 30%", "N/A"
	Recommendation string //advice shown when the metric fails
	DataPoints     int    //evidence behind the verdict, summed for the limited-data note
}

// Detail is the body of a metric's <details> block.
type Detail struct {
	Summary string
	Body    string
}

// Metric is one check over a fetched contributor. Metrics are stateless structs
// and are registered by hand in NewRegistry.
type Metric interface {
	// Metadata takes the config because one description embeds a configured
	// value (the star bar for repo quality); the rest ignore it.
	Metadata(cfg Config) Metadata
	Evaluate(c *analysis.Contributor, cfg Config) Result
	// DetailGroup names the <details> block this metric contributes to. Metrics
	// that share a block — the two reaction metrics, the two repo-history
	// metrics — return the same group and embed a common Detail implementation.
	DetailGroup() string
	Detail(c *analysis.Contributor, cfg Config) Detail
}

// NewRegistry lists every metric. The order here is the order of the report
// table, so it is a literal slice rather than a map or an init() registry.
func NewRegistry() []Metric {
	return []Metric{
		&PRMergeRate{},
		&RepoQuality{},
		&PositiveReactions{},
		&NegativeReactions{},
		&AccountAge{},
		&ActivityConsistency{},
		&IssueEngagement{},
		&CodeReviews{},
		&MergerDiversity{},
		&RepoHistoryMergeRate{},
		&RepoHistoryMinPRs{},
		&ProfileCompleteness{},
		&SuspiciousPatterns{},
	}
}

// Evaluation pairs a metric's declaration with its verdict.
type Evaluation struct {
	Metadata Metadata
	Result   Result
}

// DetailBlock is a rendered <details> section.
type DetailBlock struct {
	Group   string
	Summary string
	Body    string
	Failed  bool //true when any metric in the group failed
}

// Report is everything the renderer needs.
type Report struct {
	Username        string
	Passed          bool
	PassedCount     int
	TotalCount      int
	IsNewAccount    bool
	NewAccountDays  int
	HasLimitedData  bool
	Evaluations     []Evaluation
	Details         []DetailBlock
	Recommendations []string
	WindowStart     time.Time
	WindowEnd       time.Time
}

// Evaluate runs every registered metric and assembles the report. prog reports
// one progress step per metric and may be nil, which is how the tests run.
func Evaluate(c *analysis.Contributor, cfg Config, prog *progress.Stepper) Report {
	registry := NewRegistry()

	report := Report{
		Username:       c.Login,
		IsNewAccount:   c.AgeInDays() < cfg.NewAccountDays,
		NewAccountDays: cfg.NewAccountDays,
		WindowStart:    c.Since,
		WindowEnd:      c.Until,
		Passed:         true,
	}

	groupIndex := make(map[string]int)
	dataPoints := 0

	for _, metric := range registry {
		meta := metric.Metadata(cfg)
		prog.Step(meta.Name)
		result := metric.Evaluate(c, cfg)
		report.Evaluations = append(report.Evaluations, Evaluation{Metadata: meta, Result: result})

		if !result.Active {
			prog.Skip("not applicable")
			continue
		}
		prog.Done(stepDetail(result))
		dataPoints += result.DataPoints
		report.TotalCount++
		if result.Passed {
			report.PassedCount++
		}
		// Only the required metrics gate the verdict, so a run can report
		// "12/13 metrics passed" and still pass overall.
		if !result.Passed && cfg.isRequired(meta.Key) {
			report.Passed = false
		}

		group := metric.DetailGroup()
		if at, seen := groupIndex[group]; seen {
			report.Details[at].Failed = report.Details[at].Failed || !result.Passed
			continue
		}
		detail := metric.Detail(c, cfg)
		groupIndex[group] = len(report.Details)
		report.Details = append(report.Details, DetailBlock{
			Group:   group,
			Summary: detail.Summary,
			Body:    detail.Body,
			Failed:  !result.Passed,
		})
	}

	// The original sums every metric's data points, which double-counts
	// heavily; kept as-is so the warning triggers on the same inputs.
	report.HasLimitedData = dataPoints < minDataPointsForFullReport
	report.Recommendations = recommendations(report.Evaluations)
	return report
}

// stepDetail summarises a verdict for the progress line. Value is already a
// formatted cell, and is empty for the metrics that only report a status.
func stepDetail(result Result) string {
	status := "passed"
	if !result.Passed {
		status = "failed"
	}
	if result.Value == "" {
		return status
	}
	return result.Value + ", " + status
}

// recommendations collects the advice from failed metrics. A critical spam
// finding replaces everything else — nothing else is worth saying at that point.
func recommendations(evaluations []Evaluation) []string {
	var advice []string
	for _, e := range evaluations {
		if !e.Result.Active || e.Result.Passed || e.Result.Recommendation == "" {
			continue
		}
		if e.Metadata.Key == "suspiciousPatterns" {
			return []string{e.Result.Recommendation}
		}
		advice = append(advice, e.Result.Recommendation)
	}
	return advice
}
