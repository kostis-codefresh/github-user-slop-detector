package metrics

import (
	"fmt"
	"testing"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/stretchr/testify/assert"
)

// withReactions puts the given reaction contents on a single comment.
func withReactions(contents ...string) func(*analysis.Contributor) {
	return func(c *analysis.Contributor) {
		c.IssueComments = append(c.IssueComments, analysis.IssueComment{Reactions: contents})
	}
}

func TestPositiveReactions(t *testing.T) {
	testCases := []struct {
		name           string
		reactions      []string
		threshold      int
		expectedValue  string
		expectedPassed bool
	}{
		{
			name:           "Happy Path scenario",
			reactions:      []string{"THUMBS_UP", "HEART", "ROCKET", "HOORAY", "THUMBS_UP"},
			threshold:      5,
			expectedValue:  "5",
			expectedPassed: true,
		},
		{
			name:           "Problematic scenario",
			reactions:      []string{"THUMBS_UP"},
			threshold:      5,
			expectedValue:  "1",
			expectedPassed: false,
		},
		{
			// LAUGH and EYES are neither praise nor criticism.
			name:           "Neutral reactions are not positive",
			reactions:      []string{"LAUGH", "EYES"},
			threshold:      1,
			expectedValue:  "0",
			expectedPassed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(withReactions(tc.reactions...))
			cfg := testConfig(func(cfg *Config) { cfg.Thresholds.PositiveReactions = tc.threshold })

			result := (&PositiveReactions{}).Evaluate(c, cfg)

			assert.True(t, result.Active, "positiveReactions should always be active")
			assert.Equal(t, tc.expectedValue, result.Value, "positiveReactions value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "positiveReactions passed status mismatch")
		})
	}
}

func TestNegativeReactions(t *testing.T) {
	testCases := []struct {
		name           string
		reactions      []string
		threshold      int
		expectedValue  string
		expectedPassed bool
	}{
		{
			name:           "Happy Path scenario",
			reactions:      []string{"THUMBS_UP"},
			threshold:      5,
			expectedValue:  "0",
			expectedPassed: true,
		},
		{
			// The threshold is a ceiling here, unlike every other metric.
			name:           "Problematic scenario",
			reactions:      []string{"THUMBS_DOWN", "CONFUSED", "THUMBS_DOWN"},
			threshold:      2,
			expectedValue:  "3",
			expectedPassed: false,
		},
		{
			name:           "Exactly at the ceiling still passes",
			reactions:      []string{"THUMBS_DOWN", "CONFUSED"},
			threshold:      2,
			expectedValue:  "2",
			expectedPassed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(withReactions(tc.reactions...))
			cfg := testConfig(func(cfg *Config) { cfg.Thresholds.NegativeReactions = tc.threshold })

			result := (&NegativeReactions{}).Evaluate(c, cfg)

			assert.Equal(t, tc.expectedValue, result.Value, "negativeReactions value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "negativeReactions passed status mismatch")
			assert.Equal(t, fmt.Sprintf("<= %d", tc.threshold), result.Threshold,
				"negativeReactions renders a ceiling, not a floor")
		})
	}
}

func TestReactionsCountIssuesAsWellAsComments(t *testing.T) {
	c := contributor(func(c *analysis.Contributor) {
		c.IssueComments = []analysis.IssueComment{{Reactions: []string{"THUMBS_UP"}}}
		c.Issues = []analysis.Issue{{Reactions: []string{"HEART", "THUMBS_DOWN"}}}
	})

	result := (&PositiveReactions{}).Evaluate(c, testConfig())

	assert.Equal(t, "2", result.Value, "reactions on issues should count too")
	assert.Equal(t, 2, result.DataPoints, "one comment plus one issue were analysed")
}

func TestReactionsDetail(t *testing.T) {
	t.Run("Happy Path scenario", func(t *testing.T) {
		c := contributor(withReactions("THUMBS_UP", "THUMBS_UP", "THUMBS_DOWN", "LAUGH"))

		detail := (&PositiveReactions{}).Detail(c, testConfig())

		assert.Equal(t, "👍 2 positive / 1 negative", detail.Summary, "summary mismatch")
		assert.Contains(t, detail.Body, "- Comments analyzed: 1", "comment count missing")
		assert.Contains(t, detail.Body, "- Neutral reactions: 1", "neutral count missing")
		assert.Contains(t, detail.Body, "- Positive ratio: 50%", "ratio mismatch")
	})

	t.Run("Empty data scenario", func(t *testing.T) {
		detail := (&NegativeReactions{}).Detail(contributor(), testConfig())

		assert.Equal(t, "👍 No comments analyzed", detail.Summary, "empty summary mismatch")
	})
}

func TestReactionMetricsShareOneDetailBlock(t *testing.T) {
	assert.Equal(t, (&PositiveReactions{}).DetailGroup(), (&NegativeReactions{}).DetailGroup(),
		"both reaction metrics must render into the same <details> block")
}
