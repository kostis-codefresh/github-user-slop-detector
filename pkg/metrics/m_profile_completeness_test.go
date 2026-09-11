package metrics

import (
	"testing"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
	"github.com/stretchr/testify/assert"
)

func TestProfileCompleteness(t *testing.T) {
	testCases := []struct {
		name           string
		followers      int
		repos          int
		bio            string
		company        string
		threshold      int
		expectedValue  string
		expectedPassed bool
	}{
		{
			name:      "Happy Path scenario",
			followers: 60, repos: 20, bio: "I write software", company: "Acme",
			threshold: 20, expectedValue: "100", expectedPassed: true,
		},
		{
			name:      "Problematic scenario",
			followers: 0, repos: 0, threshold: 20,
			expectedValue: "0", expectedPassed: false,
		},
		{
			// Scoring is stepped, not per-follower: 27 followers clears the
			// ten mark but not the fifty mark.
			name:      "Followers score in steps",
			followers: 27, repos: 11, bio: "hi", company: "Acme",
			threshold: 20, expectedValue: "90", expectedPassed: true,
		},
		{
			name:      "A single follower is worth the base band",
			followers: 1, repos: 1,
			threshold: 20, expectedValue: "35", expectedPassed: true,
		},
		{
			// Whitespace is not a bio.
			name:      "Blank bio earns nothing",
			followers: 0, repos: 0, bio: "", company: "",
			threshold: 1, expectedValue: "0", expectedPassed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := contributor(func(c *analysis.Contributor) {
				c.Followers, c.PublicRepos, c.Bio, c.Company = tc.followers, tc.repos, tc.bio, tc.company
			})
			cfg := testConfig(func(cfg *Config) { cfg.Thresholds.ProfileCompleteness = tc.threshold })

			result := (&ProfileCompleteness{}).Evaluate(c, cfg)

			assert.Equal(t, tc.expectedValue, result.Value, "profileCompleteness value mismatch")
			assert.Equal(t, tc.expectedPassed, result.Passed, "profileCompleteness passed status mismatch")
		})
	}
}

func TestProfileCompletenessNamesWhatIsMissing(t *testing.T) {
	c := contributor(func(c *analysis.Contributor) { c.PublicRepos = 10 })

	result := (&ProfileCompleteness{}).Evaluate(c, testConfig())

	assert.Contains(t, result.Recommendation,
		"bio, company/affiliation and GitHub followers (engage with community)",
		"the recommendation should list every missing part as prose")
}

func TestProfileCompletenessDetail(t *testing.T) {
	c := contributor(func(c *analysis.Contributor) {
		c.Followers, c.PublicRepos, c.Bio = 26, 13, "I write software"
	})

	detail := (&ProfileCompleteness{}).Detail(c, testConfig())

	assert.Equal(t, "📝 Score: 70/100", detail.Summary, "summary mismatch")
	assert.Equal(t, []Block{table([]string{"Component", "Status", "Points"},
		[]string{"Followers", "26", "+30/40"},
		[]string{"Public repos", "13", "+20/20"},
		[]string{"Bio", "✅", "+20/20"},
		[]string{"Company", "❌", "+0/20"},
	)}, detail.Blocks, "profile score table mismatch")
}
