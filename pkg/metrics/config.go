// Package metrics evaluates a contributor against the report's metrics. Every
// metric is a pure function of an already-fetched *analysis.Contributor, so
// nothing here touches the network.
package metrics

import "strings"

// Thresholds holds the bar each metric must clear. Defaults are the meaningful
// values from the upstream README's example report rather than the action's
// all-zero defaults, which would make a bare CLI run pass everything.
type Thresholds struct {
	PRMergeRate          float64
	RepoQuality          int
	PositiveReactions    int
	NegativeReactions    int //a maximum, not a minimum
	AccountAge           int //days
	ActivityConsistency  float64
	IssueEngagement      int
	CodeReviews          int
	MergerDiversity      int
	RepoHistoryMergeRate float64
	RepoHistoryMinPRs    int
	ProfileCompleteness  int
}

// DefaultThresholds returns the out-of-the-box bar.
func DefaultThresholds() Thresholds {
	return Thresholds{
		PRMergeRate:          0.30,
		RepoQuality:          2,
		PositiveReactions:    5,
		NegativeReactions:    5,
		AccountAge:           30,
		ActivityConsistency:  0.50,
		IssueEngagement:      1,
		CodeReviews:          1,
		MergerDiversity:      2,
		RepoHistoryMergeRate: 0,
		RepoHistoryMinPRs:    0,
		ProfileCompleteness:  20,
	}
}

// Config is everything a metric needs beyond the contributor data itself.
type Config struct {
	// Repo is the owner/name the repo-history metrics apply to. Empty means the
	// CLI was run without --repo, and those metrics report themselves inactive.
	Repo           string
	AnalysisWindow int //months
	MinimumStars   int
	SpamDetection  bool
	NewAccountDays int
	Required       []string
	Thresholds     Thresholds
}

// DefaultConfig returns the configuration a bare CLI invocation uses.
func DefaultConfig() Config {
	return Config{
		AnalysisWindow: 12,
		MinimumStars:   100,
		SpamDetection:  true,
		NewAccountDays: 30,
		Required:       []string{"prMergeRate", "accountAge"},
		Thresholds:     DefaultThresholds(),
	}
}

// isRequired reports whether the named metric gates the overall verdict.
func (cfg Config) isRequired(key string) bool {
	for _, name := range cfg.Required {
		if strings.EqualFold(strings.TrimSpace(name), key) {
			return true
		}
	}
	return false
}
