package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/kostis-codefresh/github-user-slop-detector/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The whole output of this tool is one HTML document, so the renderer is
// pinned with golden files. Run `go test ./pkg/report -update` to refresh them
// after a deliberate change, then read the diff before committing it.
var update = flag.Bool("update", false, "rewrite the golden files")

// sampleReport is a fully populated report, shaped after a real contributor so
// the golden file exercises every section at once.
func sampleReport() metrics.Report {
	return metrics.Report{
		Username:    "octocat",
		Passed:      true,
		PassedCount: 12,
		TotalCount:  13,
		WindowStart: time.Date(2025, 3, 23, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC),
		Evaluations: []metrics.Evaluation{
			{
				Metadata: metrics.Metadata{Name: "PR Merge Rate", DocSlug: "pr-merge-rate", Description: "PRs merged vs closed"},
				Result:   metrics.Result{Active: true, Passed: true, Value: "86%", Threshold: ">= 30%"},
			},
			{
				Metadata: metrics.Metadata{Name: "Repo Quality", DocSlug: "repo-quality", Description: "Repos with ≥100 stars"},
				Result:   metrics.Result{Active: true, Passed: false, Value: "1", Threshold: ">= 2"},
			},
			{
				Metadata: metrics.Metadata{Name: "Repo History Min PRs", DocSlug: "repo-history", Description: "Previous PRs in this repo"},
				Result:   metrics.Result{},
			},
			{
				Metadata: metrics.Metadata{Name: "Suspicious Patterns", DocSlug: "suspicious-patterns", Description: "Spam-like activity detection"},
				Result:   metrics.Result{Active: true, Passed: true, Value: "0", Threshold: "N/A"},
			},
		},
		Details: []metrics.DetailSection{
			{
				Group:   "prMergeRate",
				Icon:    metrics.IconPRs,
				Summary: "86% merge rate (25/32 PRs)",
				Blocks: []metrics.Block{{
					Kind: metrics.KindList,
					Lines: []metrics.Line{
						{Text: "Merged: 25"},
						{Text: "Closed without merge: 4"},
						{Text: "Open: 3"},
					},
				}},
			},
			{
				Group:   "repoQuality",
				Icon:    metrics.IconRepos,
				Summary: "1 quality repo found",
				Blocks: []metrics.Block{{
					Kind: metrics.KindTable,
					Head: []string{"Repository", "Stars", "PRs"},
					Rows: [][]string{{"davila7/claude-code-templates", "23.4k", "1"}},
				}},
				Failed: true,
			},
		},
		Recommendations: []string{
			"Consider contributing to established open source projects with significant community adoption.",
		},
	}
}

func TestWrite(t *testing.T) {
	testCases := []struct {
		name    string
		mode    DetailMode
		report  func() metrics.Report
		homeURL string
		golden  string
	}{
		{name: "Happy Path scenario", mode: DetailsAll, report: sampleReport, golden: "report-all.golden.html"},
		{name: "Only failed details", mode: DetailsFailed, report: sampleReport, golden: "report-failed.golden.html"},
		{name: "No details", mode: DetailsNone, report: sampleReport, golden: "report-none.golden.html"},
		{
			name: "Problematic scenario",
			mode: DetailsAll,
			report: func() metrics.Report {
				r := sampleReport()
				r.Passed, r.PassedCount = false, 9
				r.IsNewAccount, r.NewAccountDays = true, 30
				return r
			},
			golden: "report-needs-review.golden.html",
		},
		{
			name: "Limited data note replaces nothing when the account is new",
			mode: DetailsNone,
			report: func() metrics.Report {
				r := sampleReport()
				r.HasLimitedData = true
				r.Recommendations = nil
				return r
			},
			golden: "report-limited-data.golden.html",
		},
		{
			name: "Notes the activity hidden by SAML enforcement",
			mode: DetailsNone,
			report: func() metrics.Report {
				r := sampleReport()
				r.HiddenBySAML = 3
				return r
			},
			golden: "report-saml-hidden.golden.html",
		},
		{
			name:    "Links back to the web app's home page",
			mode:    DetailsNone,
			report:  sampleReport,
			homeURL: "/",
			golden:  "report-home-link.golden.html",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer

			err := Write(&buf, tc.report(), tc.mode, tc.homeURL)
			require.NoError(t, err, "rendering the report should not fail")

			assertGolden(t, tc.golden, buf.String())
		})
	}
}

func TestWriteRendersInactiveMetricsAsDashes(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, Write(&buf, sampleReport(), DetailsNone, ""), "rendering should not fail")

	assert.Contains(t, buf.String(), ">Repo History Min PRs</a>", "the inactive metric still gets a row")
	assert.Contains(t, buf.String(), `<tr class="status-inactive">`, "an inactive metric is tinted as inactive, not as a pass or a failure")
	assert.Contains(t, buf.String(), `<td class="metric-status">—</td>`, "an inactive metric shows a dash, not a tick or a cross")
}

func TestWriteHidesTheNewAccountNoteWhenNotNew(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, Write(&buf, sampleReport(), DetailsNone, ""), "rendering should not fail")

	assert.NotContains(t, buf.String(), "new GitHub account", "an established account gets no new-account note")
}

func TestParseDetailMode(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expected  DetailMode
		expectErr bool
	}{
		{name: "Happy Path scenario", input: "all", expected: DetailsAll},
		{name: "Failed only", input: "failed", expected: DetailsFailed},
		{name: "None", input: "none", expected: DetailsNone},
		{name: "Problematic scenario", input: "verbose", expectErr: true},
		{name: "Empty is rejected", input: "", expectErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDetailMode(tc.input)

			if tc.expectErr {
				assert.Error(t, err, "invalid mode %q should be rejected", tc.input)
				return
			}
			require.NoError(t, err, "valid mode %q should be accepted", tc.input)
			assert.Equal(t, tc.expected, got, "parsed mode mismatch")
		})
	}
}

// assertGolden compares output against testdata, rewriting it under -update.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)

	if *update {
		require.NoError(t, os.MkdirAll("testdata", 0o755), "could not create testdata")
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644), "could not write %s", name)
		return
	}

	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file %s; run: go test ./pkg/report -update", name)
	assert.Equal(t, strings.ReplaceAll(string(want), "\r\n", "\n"), got, "rendered report differs from %s", name)
}

// The detail icons are chosen in pkg/metrics by name and drawn by the
// stylesheet, so a new icon whose CSS rule is missing would silently render no
// glyph at all. Every name must have a rule.
func TestEveryIconNameHasAStylesheetRule(t *testing.T) {
	tpl, err := templateFS.ReadFile("templates/report.html.tpl")
	require.NoError(t, err, "the template should be embedded")

	icons := []string{
		metrics.IconPRs, metrics.IconRepos, metrics.IconReactions, metrics.IconAccount,
		metrics.IconIssues, metrics.IconReviews, metrics.IconMergers, metrics.IconHistory,
		metrics.IconProfile, metrics.IconAlert, metrics.IconClean, metrics.IconDisabled,
	}
	for _, icon := range icons {
		assert.Contains(t, string(tpl), "."+"icon-"+icon+"::before { content:",
			"icon %q has no glyph in the stylesheet", icon)
	}
}

// Each registered metric's detail must name an icon, or its <details> block
// renders with a bare class and no glyph.
func TestEveryMetricDetailNamesAnIcon(t *testing.T) {
	c := &analysis.Contributor{Login: "octocat", CreatedAt: time.Now().AddDate(-3, 0, 0)}
	cfg := metrics.DefaultConfig()
	cfg.Repo = "org/project"

	for _, m := range metrics.NewRegistry() {
		assert.NotEmpty(t, m.Detail(c, cfg).Icon,
			"metric %q returns a detail with no icon", m.Metadata(cfg).Key)
	}
}
