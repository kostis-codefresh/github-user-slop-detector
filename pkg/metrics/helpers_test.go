package metrics

import (
	"time"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
)

// Fixtures for the metric tests. Every metric is a pure function of a
// *analysis.Contributor, so the fake is the struct itself — no metric test
// touches the network.

var (
	testSince = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	testUntil = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

// contributor builds a plausible baseline contributor and applies the given
// overrides, so each test only states what it actually cares about.
func contributor(overrides ...func(*analysis.Contributor)) *analysis.Contributor {
	c := &analysis.Contributor{
		Login:        "octocat",
		CreatedAt:    testUntil.AddDate(-3, 0, 0),
		ActiveMonths: map[string]bool{},
		Since:        testSince,
		Until:        testUntil,
	}
	for _, override := range overrides {
		override(c)
	}
	return c
}

// mergedPR returns a PR merged inside the window by the named account.
func mergedPR(repo string, stars int, mergedBy string) analysis.PullRequest {
	owner, name := splitRepo(repo)
	return analysis.PullRequest{
		State:     "MERGED",
		Merged:    true,
		CreatedAt: testSince.AddDate(0, 1, 0),
		MergedAt:  testSince.AddDate(0, 2, 0),
		Additions: 50,
		Deletions: 10,
		MergedBy:  mergedBy,
		RepoOwner: owner,
		RepoName:  name,
		Stars:     stars,
	}
}

// closedPR returns a PR closed inside the window without being merged.
func closedPR(repo string) analysis.PullRequest {
	owner, name := splitRepo(repo)
	return analysis.PullRequest{
		State: "CLOSED", CreatedAt: testSince.AddDate(0, 1, 0),
		Additions: 20, Deletions: 5, RepoOwner: owner, RepoName: name,
	}
}

// openPR returns a PR still awaiting a verdict.
func openPR(repo string) analysis.PullRequest {
	owner, name := splitRepo(repo)
	return analysis.PullRequest{
		State: "OPEN", CreatedAt: testSince.AddDate(0, 1, 0),
		Additions: 3, Deletions: 1, RepoOwner: owner, RepoName: name,
	}
}

func splitRepo(repo string) (owner, name string) {
	for i := range len(repo) {
		if repo[i] == '/' {
			return repo[:i], repo[i+1:]
		}
	}
	return repo, ""
}

// testConfig returns the default config with the given overrides applied.
func testConfig(overrides ...func(*Config)) Config {
	cfg := DefaultConfig()
	for _, override := range overrides {
		override(&cfg)
	}
	return cfg
}
