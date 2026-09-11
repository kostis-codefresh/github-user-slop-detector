// contributor-report analyses a GitHub user's public activity and prints a
// markdown contributor report, to help maintainers spot low-effort or
// AI-generated pull requests.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kkp/contributor-report/pkg/analysis"
	"github.com/kkp/contributor-report/pkg/metrics"
	"github.com/kkp/contributor-report/pkg/report"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

// Exit codes. Needs-review is kept distinct from an operational failure so the
// tool can be scripted without conflating "bad contributor" with "broken run".
const (
	exitPassed      = 0
	exitNeedsReview = 1
	exitError       = 2
)

func main() {
	log.SetFlags(0)
	os.Exit(run())
}

func run() int {
	cfg := metrics.DefaultConfig()
	thresholds := &cfg.Thresholds

	showVersion := flag.Bool("version", false, "Print version information")
	flag.BoolVar(showVersion, "v", false, "Print version information (shorthand)")
	repo := flag.String("repo", "", "Repository as owner/name, enables the repo history metrics")
	details := flag.String("details", "all", "Metric details to show: all, failed or none")
	required := flag.String("required", strings.Join(cfg.Required, ","),
		"Comma-separated metrics that must pass for the overall verdict")
	flag.IntVar(&cfg.AnalysisWindow, "analysis-window", cfg.AnalysisWindow, "Months of history to analyse")
	flag.IntVar(&cfg.MinimumStars, "minimum-stars", cfg.MinimumStars, "Stars for a repo to count as quality")
	flag.BoolVar(&cfg.SpamDetection, "spam-detection", cfg.SpamDetection, "Enable suspicious pattern detection")
	flag.IntVar(&cfg.NewAccountDays, "new-account-days", cfg.NewAccountDays, "Days below which an account is new")

	flag.Float64Var(&thresholds.PRMergeRate, "threshold-pr-merge-rate", thresholds.PRMergeRate, "Minimum PR merge rate (0-1)")
	flag.IntVar(&thresholds.RepoQuality, "threshold-repo-quality", thresholds.RepoQuality, "Minimum quality repos")
	flag.IntVar(&thresholds.PositiveReactions, "threshold-positive-reactions", thresholds.PositiveReactions, "Minimum positive reactions")
	flag.IntVar(&thresholds.NegativeReactions, "threshold-negative-reactions", thresholds.NegativeReactions, "Maximum negative reactions")
	flag.IntVar(&thresholds.AccountAge, "threshold-account-age", thresholds.AccountAge, "Minimum account age in days")
	flag.Float64Var(&thresholds.ActivityConsistency, "threshold-activity-consistency", thresholds.ActivityConsistency, "Minimum activity consistency (0-1)")
	flag.IntVar(&thresholds.IssueEngagement, "threshold-issue-engagement", thresholds.IssueEngagement, "Minimum engaged issues")
	flag.IntVar(&thresholds.CodeReviews, "threshold-code-reviews", thresholds.CodeReviews, "Minimum code reviews given")
	flag.IntVar(&thresholds.MergerDiversity, "threshold-merger-diversity", thresholds.MergerDiversity, "Minimum unique mergers")
	flag.Float64Var(&thresholds.RepoHistoryMergeRate, "threshold-repo-history-merge-rate", thresholds.RepoHistoryMergeRate, "Minimum merge rate in this repo (0-1)")
	flag.IntVar(&thresholds.RepoHistoryMinPRs, "threshold-repo-history-min-prs", thresholds.RepoHistoryMinPRs, "Minimum previous PRs in this repo")
	flag.IntVar(&thresholds.ProfileCompleteness, "threshold-profile-completeness", thresholds.ProfileCompleteness, "Minimum profile score (0-100)")

	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return exitPassed
	}

	if flag.NArg() != 1 {
		flag.Usage()
		return exitError
	}
	username := flag.Arg(0)

	cfg.Repo = *repo
	cfg.Required = splitList(*required)
	if cfg.Repo != "" && strings.Count(cfg.Repo, "/") != 1 {
		log.Printf("invalid --repo %q: want owner/name", cfg.Repo)
		return exitError
	}

	mode, err := report.ParseDetailMode(*details)
	if err != nil {
		log.Print(err)
		return exitError
	}

	token := os.Getenv("GH_TOKEN")
	if token == "" {
		log.Print("GH_TOKEN is not set: the GitHub GraphQL API requires a token.\n" +
			"Try: GH_TOKEN=$(gh auth token) contributor-report " + username)
		return exitError
	}

	// Step 1 - work out the analysis window
	since := analysis.Window(time.Now(), cfg.AnalysisWindow)

	// Step 2 - fetch the contributor's public activity
	contributor, err := analysis.NewClient(token).Fetch(context.Background(), username, since)
	if err != nil {
		log.Print(err)
		return exitError
	}

	// Step 3 - evaluate every metric
	result := metrics.Evaluate(contributor, cfg)

	// Step 4 - render the report to stdout
	if err := report.Write(os.Stdout, result, mode); err != nil {
		log.Print(err)
		return exitError
	}

	// Step 5 - exit non-zero when the required metrics did not pass
	if !result.Passed {
		return exitNeedsReview
	}
	return exitPassed
}

// splitList parses a comma-separated flag value, dropping empty entries so
// --required="" means "no metric gates the verdict".
func splitList(s string) []string {
	var out []string
	for _, item := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(),
		"contributor-report analyses a GitHub user's public activity and prints a markdown report.\n\n"+
			"Usage:\n  contributor-report [flags] <username>\n\n"+
			"The GH_TOKEN environment variable must hold a GitHub token.\n\nFlags:\n")
	flag.PrintDefaults()
}
