// Package analysis holds the contributor data model and the GitHub fetcher that
// populates it. Everything downstream — metrics, report — reads a single
// *Contributor value and never talks to the network itself.
package analysis

import (
	"sort"
	"strings"
	"time"
)

// Contributor is the single struct threaded through the whole pipeline.
type Contributor struct {
	Login       string
	CreatedAt   time.Time
	Bio         string
	Company     string
	Location    string
	WebsiteURL  string
	Followers   int
	PublicRepos int

	// PullRequests are already filtered to those created within the window.
	PullRequests []PullRequest
	// IssueComments are deliberately NOT date-filtered, matching the original
	// action: the GraphQL connection has no date argument and the upstream code
	// never filters them afterwards.
	IssueComments []IssueComment
	// Issues come from the search connection, capped at 50 by the query.
	Issues []Issue

	// ActiveMonths holds the "YYYY-MM" keys that had at least one contribution
	// in the contribution calendar.
	ActiveMonths map[string]bool
	ReviewsGiven int

	// RateLimit is the GraphQL budget as of the most recent response GitHub sent
	// while assembling this struct.
	RateLimit RateLimit

	Since time.Time //start of the analysis window
	Until time.Time //when the report was generated
}

// RateLimit is what the API's rateLimit field reported: how much of the hourly
// GraphQL budget is left and when it refills.
type RateLimit struct {
	Remaining int
	Used      int
	ResetAt   time.Time
}

// PullRequest is one PR authored by the contributor.
type PullRequest struct {
	State     string //MERGED, CLOSED or OPEN
	Merged    bool
	CreatedAt time.Time
	MergedAt  time.Time
	Additions int
	Deletions int
	MergedBy  string //empty when the merger is unknown or the account was deleted
	RepoOwner string
	RepoName  string
	Stars     int
}

// Repo returns the owner/name identifier of the repository the PR targets.
func (pr PullRequest) Repo() string { return pr.RepoOwner + "/" + pr.RepoName }

// Size returns the number of lines the PR touched.
func (pr PullRequest) Size() int { return pr.Additions + pr.Deletions }

// IssueComment is a comment the contributor left, with the reactions it drew.
type IssueComment struct {
	CreatedAt time.Time
	Reactions []string
}

// Issue is an issue the contributor opened, with its engagement counts.
type Issue struct {
	CreatedAt time.Time
	Comments  int
	Reactions []string
}

// AgeInDays is the whole number of days since the account was created.
func (c *Contributor) AgeInDays() int {
	return int(c.Until.Sub(c.CreatedAt).Hours() / 24)
}

// ActiveMonthCount is the number of distinct months with any contribution.
func (c *Contributor) ActiveMonthCount() int { return len(c.ActiveMonths) }

// MergedPRs returns the PRs that were merged inside the analysis window,
// in the order GitHub returned them.
func (c *Contributor) MergedPRs() []PullRequest {
	var merged []PullRequest
	for _, pr := range c.PullRequests {
		if pr.Merged && !pr.MergedAt.IsZero() && !pr.MergedAt.Before(c.Since) {
			merged = append(merged, pr)
		}
	}
	return merged
}

// RepoStat aggregates the contributor's merged PRs for one repository.
type RepoStat struct {
	Owner    string
	Name     string
	Stars    int
	MergedPR int
}

// Repo returns the owner/name identifier.
func (r RepoStat) Repo() string { return r.Owner + "/" + r.Name }

// ContributedRepos aggregates merged PRs by repository. The result keeps
// first-seen order so it is stable regardless of map iteration.
func (c *Contributor) ContributedRepos() []RepoStat {
	index := make(map[string]int)
	var stats []RepoStat
	for _, pr := range c.MergedPRs() {
		key := pr.Repo()
		if at, seen := index[key]; seen {
			stats[at].MergedPR++
			continue
		}
		index[key] = len(stats)
		stats = append(stats, RepoStat{Owner: pr.RepoOwner, Name: pr.RepoName, Stars: pr.Stars, MergedPR: 1})
	}
	return stats
}

// TopReposByStars returns the contributed repositories sorted by star count,
// highest first, breaking ties on the repository name so output is stable.
func (c *Contributor) TopReposByStars() []RepoStat {
	stats := c.ContributedRepos()
	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].Stars != stats[j].Stars {
			return stats[i].Stars > stats[j].Stars
		}
		return stats[i].Repo() < stats[j].Repo()
	})
	return stats
}

// PRsInRepo returns the window's PRs targeting the given owner/name, matched
// case-insensitively because GitHub treats repository names that way.
func (c *Contributor) PRsInRepo(repo string) []PullRequest {
	want := strings.ToLower(repo)
	var found []PullRequest
	for _, pr := range c.PullRequests {
		if strings.ToLower(pr.Repo()) == want {
			found = append(found, pr)
		}
	}
	return found
}

// Window returns the start of the analysis window: months before now, with the
// day clamped to the target month's last day, at midnight UTC. This mirrors the
// original action's month arithmetic rather than subtracting 365 days.
func Window(now time.Time, months int) time.Time {
	now = now.UTC()
	// Day 0 of the month after the target normalises to the target's last day.
	lastDay := time.Date(now.Year(), now.Month()-time.Month(months)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := min(now.Day(), lastDay)
	return time.Date(now.Year(), now.Month()-time.Month(months), day, 0, 0, 0, 0, time.UTC)
}
