package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBodyHTML(t *testing.T) {
	testCases := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "Happy Path scenario",
			body:     "- Merged: 25\n- Closed without merge: 4",
			expected: "<ul>\n<li>Merged: 25</li>\n<li>Closed without merge: 4</li>\n</ul>\n",
		},
		{
			name:     "Renders a table with a header row",
			body:     "| Repository | Stars |\n|------------|-------|\n| argoproj/argo-cd | 24.1k |",
			expected: "<table>\n<thead>\n<tr><th>Repository</th><th>Stars</th></tr>\n</thead>\n<tbody>\n<tr><td>argoproj/argo-cd</td><td>24.1k</td></tr>\n</tbody>\n</table>\n",
		},
		{
			name:     "Renders a single line as a paragraph",
			body:     "Activity appears normal.",
			expected: "<p>Activity appears normal.</p>\n",
		},
		{
			name:     "Joins consecutive lines into one paragraph",
			body:     "Repository: argoproj/argo-cd\nThis is a first contribution to this repository.",
			expected: "<p>Repository: argoproj/argo-cd<br>\nThis is a first contribution to this repository.</p>\n",
		},
		{
			name:     "A blank line separates blocks",
			body:     "**Mergers:** @octocat\n\n- Self-merges: 3",
			expected: "<p><strong>Mergers:</strong> @octocat</p>\n<ul>\n<li>Self-merges: 3</li>\n</ul>\n",
		},
		{
			name:     "Italicises a line wrapped in underscores",
			body:     "_...and 8 more repositories_",
			expected: "<p><em>...and 8 more repositories</em></p>\n",
		},
		{
			// Spam evidence keys and repository names legally contain
			// underscores, so they must survive untouched.
			name:     "Leaves underscores inside a line alone",
			body:     "- threshold_accountAge: 30\n- threshold_prCount: 50",
			expected: "<ul>\n<li>threshold_accountAge: 30</li>\n<li>threshold_prCount: 50</li>\n</ul>\n",
		},
		{
			// m_repo_quality.go emits this as prose, not as a table row.
			name:     "A paragraph containing a pipe is not a table",
			body:     "Avg stars: 2279 | Highest: 24127",
			expected: "<p>Avg stars: 2279 | Highest: 24127</p>\n",
		},
		{
			name:     "Escapes HTML in the data",
			body:     "- Very short PRs (<10 lines): 30",
			expected: "<ul>\n<li>Very short PRs (&lt;10 lines): 30</li>\n</ul>\n",
		},
		{
			name:     "Escapes tags before applying inline markup",
			body:     "**<script>alert(1)</script>**",
			expected: "<p><strong>&lt;script&gt;alert(1)&lt;/script&gt;</strong></p>\n",
		},
		{
			name:     "Empty body renders nothing",
			body:     "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := bodyHTML(tc.body)

			assert.Equal(t, tc.expected, string(got), "converted body mismatch for %q", tc.body)
		})
	}
}

// The spam detail is the only body that mixes all three block kinds, so it is
// worth pinning end to end.
func TestBodyHTMLConvertsAMixedBody(t *testing.T) {
	body := "**🔴 critical: SPAM_PATTERN**\nNew account (14 days) with unusually high activity.\n\n- accountAgeDays: 14\n- totalPRs: 60"

	got := bodyHTML(body)

	assert.Equal(t,
		"<p><strong>🔴 critical: SPAM_PATTERN</strong><br>\nNew account (14 days) with unusually high activity.</p>\n"+
			"<ul>\n<li>accountAgeDays: 14</li>\n<li>totalPRs: 60</li>\n</ul>\n",
		string(got), "the spam detail body should render as a paragraph followed by a list")
}
