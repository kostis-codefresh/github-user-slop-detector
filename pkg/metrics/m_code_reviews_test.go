package metrics

import (
	"testing"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/stretchr/testify/assert"
)

func TestCodeReviews(t *testing.T) {
	testCases := []struct {
		name           string
		reviews        int
		threshold      int
		expectedValue  string
		expectedPassed bool
	}{
		{name: "Happy Path scenario", reviews: 4, threshold: 1, expectedValue: "4", expectedPassed: true},
		{name: "Problematic scenario", reviews: 0, threshold: 1, expectedValue: "0", expectedPassed: false},
		{name: "Zero threshold accepts no reviews", reviews: 0, threshold: 0, expectedValue: "0", expectedPassed: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(func(c *analysis.Contributor) { c.ReviewsGiven = tc.reviews })
			cfg := testConfig(func(cfg *Config) { cfg.Thresholds.CodeReviews = tc.threshold })

			result := (&CodeReviews{}).Evaluate(c, cfg)

			assert.Equal(t, tc.expectedValue, result.Value, "codeReviews value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "codeReviews passed status mismatch")
		})
	}
}

func TestCodeReviewsDetail(t *testing.T) {
	t.Run("Happy Path scenario", func(t *testing.T) {
		c := contributor(func(c *analysis.Contributor) { c.ReviewsGiven = 4 })

		detail := (&CodeReviews{}).Detail(c, testConfig())

		assert.Equal(t, "👀 4 reviews given", detail.Summary, "summary mismatch")
		assert.Contains(t, detail.Body, "- Review comments: 2", "review comment estimate mismatch")
	})

	t.Run("Empty data scenario", func(t *testing.T) {
		detail := (&CodeReviews{}).Detail(contributor(), testConfig())

		assert.Equal(t, "👀 No code reviews given", detail.Summary, "empty summary mismatch")
	})
}
