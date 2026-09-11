package metrics

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kkp/contributor-report/pkg/analysis"
)

// Reaction contents GitHub reports, split into the buckets the report cares
// about. LAUGH and EYES are deliberately neither positive nor negative.
var (
	positiveReactions = map[string]bool{"THUMBS_UP": true, "HEART": true, "ROCKET": true, "HOORAY": true}
	negativeReactions = map[string]bool{"THUMBS_DOWN": true, "CONFUSED": true}
)

// pct formats a 0-1 ratio as a whole-percent cell, e.g. 0.857 -> "86%".
func pct(v float64) string { return strconv.FormatFloat(v*100, 'f', 0, 64) + "%" }

// pct1 formats a 0-1 ratio with one decimal, used inside prose.
func pct1(v float64) string { return strconv.FormatFloat(v*100, 'f', 1, 64) + "%" }

// atLeast renders a minimum-threshold cell.
func atLeast(v any) string { return fmt.Sprintf(">= %v", v) }

// atLeastPct renders a minimum-threshold cell for a ratio.
func atLeastPct(v float64) string { return ">= " + pct(v) }

// count renders an integer table cell.
func count(v int) string { return strconv.Itoa(v) }

// plural returns "s" unless n is 1, for building readable detail strings.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// compactStars renders a star count the way the repo table does: 23418 -> "23.4k".
func compactStars(n int) string {
	if n >= 1000 {
		return strconv.FormatFloat(float64(n)/1000, 'f', 1, 64) + "k"
	}
	return strconv.Itoa(n)
}

// joinWithCommas renders a list as prose: "a, b and c".
func joinWithCommas(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}

// bullets joins detail body lines.
func bullets(lines ...string) string { return strings.Join(lines, "\n") }

// checkmark renders a boolean as the tick/cross used in the profile table.
func checkmark(ok bool) string {
	if ok {
		return "✅"
	}
	return "❌"
}

// prStats are the PR counts several metrics and patterns share.
type prStats struct {
	total          int
	merged         int
	closedUnmerged int
	open           int
	veryShort      int
	totalLines     int
}

func (s prStats) resolved() int { return s.merged + s.closedUnmerged }

// mergeRate is merged over resolved; open PRs are excluded from the
// denominator because they have not had a verdict yet.
func (s prStats) mergeRate() float64 {
	if s.resolved() == 0 {
		return 0
	}
	return float64(s.merged) / float64(s.resolved())
}

func (s prStats) averageSize() float64 {
	if s.total == 0 {
		return 0
	}
	return float64(s.totalLines) / float64(s.total)
}

func collectPRStats(prs []analysis.PullRequest) prStats {
	stats := prStats{total: len(prs)}
	for _, pr := range prs {
		switch {
		case pr.Merged:
			stats.merged++
		case pr.State == "CLOSED":
			stats.closedUnmerged++
		case pr.State == "OPEN":
			stats.open++
		}
		stats.totalLines += pr.Size()
		if pr.Size() < 10 {
			stats.veryShort++
		}
	}
	return stats
}

// reactionTally counts the reactions drawn by the contributor's comments and issues.
type reactionTally struct {
	positive int
	negative int
	neutral  int
	comments int //comments plus issues analysed
}

func (t reactionTally) total() int { return t.positive + t.negative + t.neutral }

// ratio is the share of reactions that were positive. With no reactions at all
// there is nothing to judge, so it reports the neutral midpoint.
func (t reactionTally) ratio() float64 {
	if t.total() == 0 {
		return 0.5
	}
	return float64(t.positive) / float64(t.total())
}

func collectReactions(c *analysis.Contributor) reactionTally {
	tally := reactionTally{comments: len(c.IssueComments) + len(c.Issues)}
	add := func(contents []string) {
		for _, content := range contents {
			switch {
			case positiveReactions[content]:
				tally.positive++
			case negativeReactions[content]:
				tally.negative++
			default:
				tally.neutral++
			}
		}
	}
	for _, comment := range c.IssueComments {
		add(comment.Reactions)
	}
	for _, issue := range c.Issues {
		add(issue.Reactions)
	}
	return tally
}

// mergerStats describes who merged the contributor's PRs.
type mergerStats struct {
	totalMerged      int
	uniqueMergers    int
	mergers          []string //display order, original casing
	selfMerges       int
	selfOnOwnRepos   int
	selfOnOtherRepos int
	mergedByOthers   int
	externalRepos    []string //repos where the contributor can merge their own work
}

func (m mergerStats) selfMergeRate() float64 {
	if m.totalMerged == 0 {
		return 0
	}
	return float64(m.selfMerges) / float64(m.totalMerged)
}

// onlySelfMergesOnOwnRepos is the red flag the metric calls out: every merge was
// the contributor merging their own work on their own repository, so no one
// else has ever vouched for it.
func (m mergerStats) onlySelfMergesOnOwnRepos() bool {
	return m.totalMerged > 0 && m.selfOnOwnRepos == m.totalMerged && m.mergedByOthers == 0
}

func collectMergers(c *analysis.Contributor) mergerStats {
	stats := mergerStats{}
	seenMerger := map[string]bool{}
	seenRepo := map[string]bool{}
	username := strings.ToLower(c.Login)

	for _, pr := range c.MergedPRs() {
		stats.totalMerged++
		if pr.MergedBy == "" {
			// A deleted account still merged the PR; it counts towards the
			// total but cannot count towards diversity.
			continue
		}
		merger := strings.ToLower(pr.MergedBy)
		if !seenMerger[merger] {
			seenMerger[merger] = true
			stats.mergers = append(stats.mergers, pr.MergedBy)
		}
		if merger != username {
			stats.mergedByOthers++
			continue
		}
		stats.selfMerges++
		if strings.ToLower(pr.RepoOwner) == username {
			stats.selfOnOwnRepos++
			continue
		}
		stats.selfOnOtherRepos++
		if !seenRepo[pr.Repo()] {
			seenRepo[pr.Repo()] = true
			stats.externalRepos = append(stats.externalRepos, pr.Repo())
		}
	}
	stats.uniqueMergers = len(stats.mergers)
	return stats
}

// repoHistory describes the contributor's track record in one repository.
type repoHistory struct {
	repo           string
	total          int
	merged         int
	closedUnmerged int
}

func (r repoHistory) resolved() int { return r.merged + r.closedUnmerged }

func (r repoHistory) mergeRate() float64 {
	if r.resolved() == 0 {
		return 0
	}
	return float64(r.merged) / float64(r.resolved())
}

// firstTime reports whether the contributor has never landed anything here.
func (r repoHistory) firstTime() bool { return r.merged == 0 }

func collectRepoHistory(c *analysis.Contributor, repo string) repoHistory {
	history := repoHistory{repo: repo}
	for _, pr := range c.PRsInRepo(repo) {
		history.total++
		switch {
		case pr.Merged:
			history.merged++
		case pr.State == "CLOSED":
			history.closedUnmerged++
		}
	}
	return history
}
