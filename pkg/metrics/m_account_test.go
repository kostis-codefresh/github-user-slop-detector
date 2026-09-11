package metrics

import (
	"testing"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/stretchr/testify/assert"
)

// activeIn marks the given "YYYY-MM" months as having contributions.
func activeIn(months ...string) func(*analysis.Contributor) {
	return func(c *analysis.Contributor) {
		for _, month := range months {
			c.ActiveMonths[month] = true
		}
	}
}

// agedDays sets the account creation date the given number of days back.
func agedDays(days int) func(*analysis.Contributor) {
	return func(c *analysis.Contributor) {
		c.CreatedAt = c.Until.AddDate(0, 0, -days)
	}
}

func TestAccountAge(t *testing.T) {
	testCases := []struct {
		name           string
		days           int
		threshold      int
		expectedValue  string
		expectedPassed bool
	}{
		{name: "Happy Path scenario", days: 3261, threshold: 30, expectedValue: "3261 days", expectedPassed: true},
		{name: "Problematic scenario", days: 5, threshold: 30, expectedValue: "5 days", expectedPassed: false},
		{name: "Exactly at the threshold passes", days: 30, threshold: 30, expectedValue: "30 days", expectedPassed: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(agedDays(tc.days))
			cfg := testConfig(func(cfg *Config) { cfg.Thresholds.AccountAge = tc.threshold })

			result := (&AccountAge{}).Evaluate(c, cfg)

			assert.Equal(t, tc.expectedValue, result.Value, "accountAge value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "accountAge passed status mismatch")
		})
	}
}

func TestActivityConsistency(t *testing.T) {
	testCases := []struct {
		name           string
		months         []string
		window         int
		threshold      float64
		expectedValue  string
		expectedPassed bool
	}{
		{
			name:           "Happy Path scenario",
			months:         []string{"2025-01", "2025-02", "2025-03", "2025-04", "2025-05", "2025-06"},
			window:         12,
			threshold:      0.5,
			expectedValue:  "50%",
			expectedPassed: true,
		},
		{
			name:           "Problematic scenario",
			months:         []string{"2025-01"},
			window:         12,
			threshold:      0.5,
			expectedValue:  "8%",
			expectedPassed: false,
		},
		{
			// The calendar is fetched from a date mid-month, so it can span
			// one more distinct month than the window nominally covers.
			name: "Can exceed one hundred percent",
			months: []string{
				"2025-01", "2025-02", "2025-03", "2025-04", "2025-05", "2025-06",
				"2025-07", "2025-08", "2025-09", "2025-10", "2025-11", "2025-12", "2026-01",
			},
			window:         12,
			threshold:      0.5,
			expectedValue:  "108%",
			expectedPassed: true,
		},
		{
			name:           "No activity scenario",
			months:         nil,
			window:         12,
			threshold:      0.5,
			expectedValue:  "0%",
			expectedPassed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(activeIn(tc.months...))
			cfg := testConfig(func(cfg *Config) {
				cfg.AnalysisWindow = tc.window
				cfg.Thresholds.ActivityConsistency = tc.threshold
			})

			result := (&ActivityConsistency{}).Evaluate(c, cfg)

			assert.Equal(t, tc.expectedValue, result.Value, "activityConsistency value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "activityConsistency passed status mismatch")
		})
	}
}

func TestActivityConsistencySparesYoungAccountsAdvice(t *testing.T) {
	t.Run("Young account gets no recommendation", func(t *testing.T) {
		// An account too young to have a track record cannot be faulted for
		// not having one.
		c := contributor(agedDays(20))

		result := (&ActivityConsistency{}).Evaluate(c, testConfig())

		assert.False(t, result.Passed, "a silent young account still fails the metric")
		assert.Empty(t, result.Recommendation, "a young account should not be told to be more consistent")
	})

	t.Run("Established account gets a recommendation", func(t *testing.T) {
		c := contributor(agedDays(400))

		result := (&ActivityConsistency{}).Evaluate(c, testConfig())

		assert.NotEmpty(t, result.Recommendation, "an established account should get advice")
	})
}

func TestAccountDetail(t *testing.T) {
	c := contributor(agedDays(3261), activeIn("2025-01", "2025-02"))

	detail := (&AccountAge{}).Detail(c, testConfig())

	assert.Equal(t, "📅 Account: 3261 days old", detail.Summary, "summary mismatch")
	assert.Contains(t, detail.Body, "- Active months: 2/12", "active month line mismatch")
	assert.Contains(t, detail.Body, "- Consistency score: 17%", "consistency line mismatch")
}

func TestAccountMetricsShareOneDetailBlock(t *testing.T) {
	assert.Equal(t, (&AccountAge{}).DetailGroup(), (&ActivityConsistency{}).DetailGroup(),
		"both account metrics must render into the same <details> block")
}
