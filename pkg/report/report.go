// Package report renders an evaluated contributor as a self-contained HTML
// document: the stylesheet is inlined, so the report is a single file that can
// be opened or attached anywhere.
package report

import (
	"embed"
	"fmt"
	"html/template"
	"io"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/metrics"
)

// The template is embedded so the binary needs no files at runtime.
//
//go:embed templates/report.html.tpl
var templateFS embed.FS

var reportTemplate = template.Must(template.ParseFS(templateFS, "templates/report.html.tpl"))

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
	StatusClass string //CSS class that tints the row and picks its verdict mark
}

// detail is one rendered <details> block.
type detail struct {
	Icon    string
	Summary string
	Blocks  []metrics.Block
	Failed  bool
}

// view is the flattened shape the template consumes.
type view struct {
	StatusText      string
	StatusClass     string
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

// Write renders the report as HTML.
func Write(w io.Writer, r metrics.Report, mode DetailMode) error {
	v := view{
		StatusText:     "Needs Review",
		StatusClass:    "needs-review",
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
		v.StatusText, v.StatusClass = "Passed", "passed"
	}

	for _, e := range r.Evaluations {
		v.Rows = append(v.Rows, row{
			Name:        e.Metadata.Name,
			URL:         e.Metadata.DocumentationURL(),
			Description: e.Metadata.Description,
			Value:       cell(e.Result.Value),
			Threshold:   cell(e.Result.Threshold),
			StatusClass: statusClass(e.Result),
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
		v.Details = append(v.Details, detail{
			Icon:    block.Icon,
			Summary: block.Summary,
			Blocks:  block.Blocks,
			Failed:  block.Failed,
		})
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

// statusClass is the verdict as a CSS class. It both tints the row and tells
// the template which mark to put in the status column; an inactive metric was
// never evaluated, so it gets neither a tick nor a cross.
func statusClass(r metrics.Result) string {
	switch {
	case !r.Active:
		return "inactive"
	case r.Passed:
		return "passed"
	default:
		return "failed"
	}
}
