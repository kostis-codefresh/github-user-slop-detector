package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkp/contributor-report/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The whole output of this tool is one markdown document, so the renderer is
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
		Details: []metrics.DetailBlock{
			{
				Group:   "prMergeRate",
				Summary: "📊 86% merge rate (25/32 PRs)",
				Body:    "- Merged: 25\n- Closed without merge: 4\n- Open: 3",
			},
			{
				Group:   "repoQuality",
				Summary: "📊 1 quality repo found",
				Body:    "| Repository | Stars | PRs |\n|------------|-------|-----|\n| davila7/claude-code-templates | 23.4k | 1 |",
				Failed:  true,
			},
		},
		Recommendations: []string{
			"Consider contributing to established open source projects with significant community adoption.",
		},
	}
}

func TestWrite(t *testing.T) {
	testCases := []struct {
		name   string
		mode   DetailMode
		report func() metrics.Report
		golden string
	}{
		{name: "Happy Path scenario", mode: DetailsAll, report: sampleReport, golden: "report-all.golden.md"},
		{name: "Only failed details", mode: DetailsFailed, report: sampleReport, golden: "report-failed.golden.md"},
		{name: "No details", mode: DetailsNone, report: sampleReport, golden: "report-none.golden.md"},
		{
			name: "Problematic scenario",
			mode: DetailsAll,
			report: func() metrics.Report {
				r := sampleReport()
				r.Passed, r.PassedCount = false, 9
				r.IsNewAccount, r.NewAccountDays = true, 30
				return r
			},
			golden: "report-needs-review.golden.md",
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
			golden: "report-limited-data.golden.md",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer

			err := Write(&buf, tc.report(), tc.mode)
			require.NoError(t, err, "rendering the report should not fail")

			assertGolden(t, tc.golden, buf.String())
		})
	}
}

func TestWriteRendersInactiveMetricsAsDashes(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, Write(&buf, sampleReport(), DetailsNone), "rendering should not fail")

	assert.Contains(t, buf.String(), "| [Repo History Min PRs](", "the inactive metric still gets a row")
	assert.Contains(t, buf.String(), "| — | — | — |", "an inactive metric shows dashes, not a tick or a cross")
}

func TestWriteHidesTheNewAccountNoteWhenNotNew(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, Write(&buf, sampleReport(), DetailsNone), "rendering should not fail")

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
			assert.NoError(t, err, "valid mode %q should be accepted", tc.input)
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
