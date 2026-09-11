// Suspicious pattern detection. Each pattern is a small independent rule over
// the same contributor data, registered by hand so the list is greppable and
// its order is the order findings are reported in.
package metrics

import (
	"fmt"
	"strconv"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
)

// Severity separates findings that should fail a contributor outright from
// those that are only worth a maintainer's attention.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityWarning  Severity = "WARNING"
)

// Thresholds for spam detection. Unlike the metric thresholds these are not
// configurable: they describe behaviour that is suspicious in any project.
const (
	spamNewAccountDays  = 30
	spamHighPRCount     = 25
	spamHighRepoCount   = 10
	spamHighPRRate      = 2.0 //PRs per day averaged over the account's life
	spamSelfMergeRate   = 0.5
	spamLowQualityStars = 10
	spamRepoCount       = 10
	spamAvgStars        = 10
)

// Evidence is one supporting figure behind a detection.
type Evidence struct {
	Key   string
	Value string
}

// Detection is a triggered pattern.
type Detection struct {
	Type        string
	Severity    Severity
	Description string
	Evidence    []Evidence
}

// Pattern is one spam heuristic. Detect returns nil when it does not fire.
type Pattern interface {
	Detect(c *analysis.Contributor, cfg Config) *Detection
}

// newPatterns lists every heuristic, in reporting order.
func newPatterns() []Pattern {
	return []Pattern{
		&spamPattern{},
		&highPRRate{},
		&selfMergeAbuse{},
		&repoSpam{},
	}
}

// prRate is PRs per day over the account's lifetime. A brand-new account is
// treated as having its whole output in a single day rather than dividing by
// zero, which is the harshest reading and the right one for spam detection.
func prRate(c *analysis.Contributor) float64 {
	total := len(c.PullRequests)
	if c.AgeInDays() <= 0 {
		return float64(total)
	}
	return float64(total) / float64(c.AgeInDays())
}

// spamPattern fires on a brand-new account already spraying PRs at many repos.
type spamPattern struct{}

func (p *spamPattern) Detect(c *analysis.Contributor, _ Config) *Detection {
	age, total, repos := c.AgeInDays(), len(c.PullRequests), len(c.ContributedRepos())
	if age >= spamNewAccountDays || total <= spamHighPRCount || repos <= spamHighRepoCount {
		return nil
	}
	return &Detection{
		Type:     "SPAM_PATTERN",
		Severity: SeverityCritical,
		Description: fmt.Sprintf("New account (%d days) with unusually high activity: "+
			"%d PRs across %d different repositories.", age, total, repos),
		Evidence: []Evidence{
			{"accountAgeDays", strconv.Itoa(age)},
			{"totalPRs", strconv.Itoa(total)},
			{"uniqueRepoCount", strconv.Itoa(repos)},
			{"threshold_accountAge", strconv.Itoa(spamNewAccountDays)},
			{"threshold_prCount", strconv.Itoa(spamHighPRCount)},
			{"threshold_repoCount", strconv.Itoa(spamHighRepoCount)},
		},
	}
}

// highPRRate fires when the sustained submission rate is faster than a person
// plausibly writes pull requests.
type highPRRate struct{}

func (p *highPRRate) Detect(c *analysis.Contributor, _ Config) *Detection {
	rate := prRate(c)
	if rate <= spamHighPRRate {
		return nil
	}
	return &Detection{
		Type:     "HIGH_PR_RATE",
		Severity: SeverityWarning,
		Description: fmt.Sprintf("High PR submission rate: %s PRs/day over account lifetime. "+
			"This may indicate automated or low-quality submissions.",
			strconv.FormatFloat(rate, 'f', 2, 64)),
		Evidence: []Evidence{
			{"prRate", strconv.FormatFloat(rate, 'f', 2, 64)},
			{"totalPRs", strconv.Itoa(len(c.PullRequests))},
			{"accountAgeDays", strconv.Itoa(c.AgeInDays())},
			{"threshold", strconv.FormatFloat(spamHighPRRate, 'f', 1, 64)},
		},
	}
}

// selfMergeAbuse fires when most merges are the contributor merging their own
// work into repositories nobody else watches — the classic way to manufacture
// a contribution history.
type selfMergeAbuse struct{}

func (p *selfMergeAbuse) Detect(c *analysis.Contributor, _ Config) *Detection {
	mergers := collectMergers(c)
	if mergers.totalMerged == 0 || mergers.selfMergeRate() <= spamSelfMergeRate {
		return nil
	}

	lowQualityRepos, lowQualityPRs := 0, 0
	for _, repo := range c.ContributedRepos() {
		if repo.Stars < spamLowQualityStars {
			lowQualityRepos++
			lowQualityPRs += repo.MergedPR
		}
	}
	if float64(lowQualityPRs)/float64(mergers.totalMerged) <= 0.5 {
		return nil
	}

	return &Detection{
		Type:     "SELF_MERGE_ABUSE",
		Severity: SeverityCritical,
		Description: fmt.Sprintf("High rate of self-merges on low-quality repositories. "+
			"%s self-merge rate with %d/%d PRs to repos with <%d stars.",
			pct(mergers.selfMergeRate()), lowQualityPRs, mergers.totalMerged, spamLowQualityStars),
		Evidence: []Evidence{
			{"selfMergeRate", pct1(mergers.selfMergeRate())},
			{"lowQualityPRs", strconv.Itoa(lowQualityPRs)},
			{"totalMergedPRs", strconv.Itoa(mergers.totalMerged)},
			{"lowQualityRepoCount", strconv.Itoa(lowQualityRepos)},
			{"threshold_selfMergeRate", pct(spamSelfMergeRate)},
			{"threshold_stars", strconv.Itoa(spamLowQualityStars)},
		},
	}
}

// repoSpam fires when contributions are scattered across many repositories
// that nobody stars.
type repoSpam struct{}

func (p *repoSpam) Detect(c *analysis.Contributor, _ Config) *Detection {
	repos := c.ContributedRepos()
	if len(repos) <= spamRepoCount {
		return nil
	}
	totalStars := 0
	for _, repo := range repos {
		totalStars += repo.Stars
	}
	average := float64(totalStars) / float64(len(repos))
	if average >= spamAvgStars {
		return nil
	}
	return &Detection{
		Type:     "REPO_SPAM",
		Severity: SeverityWarning,
		Description: fmt.Sprintf("Contributions spread across %d repositories with an average of "+
			"only %s stars. May indicate targeting of low-quality repos.",
			len(repos), strconv.FormatFloat(average, 'f', 0, 64)),
		Evidence: []Evidence{
			{"uniqueRepoCount", strconv.Itoa(len(repos))},
			{"averageStars", strconv.FormatFloat(average, 'f', 1, 64)},
			{"threshold_repoCount", strconv.Itoa(spamRepoCount)},
			{"threshold_avgStars", strconv.Itoa(spamAvgStars)},
		},
	}
}

// detectAll runs every pattern and returns those that fired.
func detectAll(c *analysis.Contributor, cfg Config) []Detection {
	var found []Detection
	for _, pattern := range newPatterns() {
		if detection := pattern.Detect(c, cfg); detection != nil {
			found = append(found, *detection)
		}
	}
	return found
}
