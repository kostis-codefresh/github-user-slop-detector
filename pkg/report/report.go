// Package report renders an evaluated contributor as a markdown document.
package report

import (
	"embed"
	"fmt"
	"io"
	"text/template"

	"github.com/kkp/contributor-report/pkg/metrics"
)

// The template is embedded so the binary needs no files at runtime. It is a
// text/template rather than html/template: the output is markdown, and HTML
// escaping would mangle the table pipes and the <details> blocks.
//
//go:embed templates/report.md.tpl
var templateFS embed.FS

var reportTemplate = template.Must(template.ParseFS(templateFS, "templates/report.md.tpl"))

// DetailMode controls how much of the Metric Details section is rendered.
type DetailMode string

const (
	DetailsAll    DetailMode = "all"
	DetailsFailed DetailMode = "failed"
	DetailsNone   DetailMode = "none"
)

// ParseDetailMode validates the --details flag.
func ParseDetailMode(s string) (DetailMode, error) {
	switch DetailMode(s) {
	case DetailsAll, DetailsFailed, DetailsNone:
		return DetailMode(s), nil
	default:
		return "", fmt.Errorf("invalid --details %q: want all, failed or none", s)
	}
}

// row is one line of the metric table.
type row struct {
	Name        string
	URL         string
	Description string
	Value       string
	Threshold   string
	Status      string
}

// detail is one rendered <details> block.
type detail struct {
	Summary string
	Body    string
}

// view is the flattened shape the template consumes.
type view struct {
	StatusEmoji     string
	StatusText      string
	Username        string
	PassedCount     int
	TotalCount      int
	NewAccountNote  bool
	NewAccountDays  int
	LimitedDataNote bool
	Rows            []row
	Details         []detail
	Recommendations []string
	WindowStart     string
	WindowEnd       string
}

// Write renders the report as markdown.
func Write(w io.Writer, r metrics.Report, mode DetailMode) error {
	v := view{
		StatusEmoji:    "⚠️",
		StatusText:     "Needs Review",
		Username:       r.Username,
		PassedCount:    r.PassedCount,
		TotalCount:     r.TotalCount,
		NewAccountNote: r.IsNewAccount,
		NewAccountDays: r.NewAccountDays,
		// The new-account note already explains thin data, so only show the
		// generic warning when that one is not being shown.
		LimitedDataNote: r.HasLimitedData && !r.IsNewAccount,
		Recommendations: r.Recommendations,
		WindowStart:     r.WindowStart.Format("2006-01-02"),
		WindowEnd:       r.WindowEnd.Format("2006-01-02"),
	}
	if r.Passed {
		v.StatusEmoji, v.StatusText = "✅", "Passed"
	}

	for _, e := range r.Evaluations {
		v.Rows = append(v.Rows, row{
			Name:        e.Metadata.Name,
			URL:         e.Metadata.DocumentationURL(),
			Description: e.Metadata.Description,
			Value:       cell(e.Result.Value),
			Threshold:   cell(e.Result.Threshold),
			Status:      status(e.Result),
		})
	}

	for _, block := range r.Details {
		if mode == DetailsNone {
			break
		}
		// Blocks are shared by several metrics, so a group survives the
		// failed-only filter if any of its metrics failed.
		if mode == DetailsFailed && !block.Failed {
			continue
		}
		v.Details = append(v.Details, detail{Summary: block.Summary, Body: block.Body})
	}

	return reportTemplate.Execute(w, v)
}

// cell renders an inactive metric's empty value as a dash.
func cell(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// status is the table's verdict column. Inactive metrics get a dash rather
// than a tick or a cross: they were never evaluated.
func status(r metrics.Result) string {
	switch {
	case !r.Active:
		return "—"
	case r.Passed:
		return "✅"
	default:
		return "❌"
	}
}
