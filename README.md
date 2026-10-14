# Contributor Report 🔍

A Go web app that analyses a GitHub user's public activity and serves an HTML contributor report, to help
maintainers spot low-effort or AI-generated pull requests ("AI slop") before spending review time on them. The same
analysis is also available as a CLI that writes the report to a file.

You give it a username, it gives you thirteen objective metrics with their values, their thresholds, and the evidence
behind each one. It does not produce a score or a verdict on a person — it produces data you can act on.

## Credit

The metrics, thresholds and report format are taken from
[jdiegosierra/contributor-report](https://github.com/jdiegosierra/contributor-report), a TypeScript GitHub Action that
posts this report as a PR comment. That project is the specification for **what** each metric means; see its
[metric documentation](https://github.com/jdiegosierra/contributor-report/tree/main/docs/metrics) for the reasoning
behind each one, and [PR #5](https://github.com/jdiegosierra/contributor-report/pull/5#issuecomment-3835434699) for
the original in action.

This is an independent Go implementation, not a port — the point is to run the same analysis **outside CI**, ad hoc,
against any username.

## Example report

The report is a single self-contained HTML file — the stylesheet is inlined, so there is nothing to serve and nothing
to copy alongside it. Open it in a browser, or attach it to a ticket as-is. It shows:

```
✅ @jdiegosierra
Contributor Report — Passed (12/13 metrics passed)

Metrics
  METRIC                    DESCRIPTION                         VALUE       THRESHOLD    STATUS
  PR Merge Rate             PRs merged vs closed                84%         >= 30%       ✅
  Repo Quality              Repos with ≥100 stars               1           >= 2         ❌
  Positive Reactions        Positive reactions received         43          >= 5         ✅
  Negative Reactions        Negative reactions received         0           <= 5         ✅
  Account Age               GitHub account age                  3432 days   >= 30 days   ✅
  Activity Consistency      Regular activity over time          100%        >= 50%       ✅
  Issue Engagement          Issues with community engagement    5           >= 1         ✅
  Code Reviews              Code reviews given to others        4           >= 1         ✅
  Merger Diversity          Unique maintainers who merged PRs   2           >= 2         ✅
  Repo History Merge Rate   Merge rate in this repo             89%         >= 0%        ✅
  Repo History Min PRs      Previous PRs in this repo           28          >= 0         ✅
  Profile Completeness      Profile richness (bio, followers)   90          >= 20        ✅
  Suspicious Patterns       Spam-like activity detection        0           N/A          ✅

Metric Details
  ▸ 📊 84% merge rate (26/32 PRs)
      Merged: 26
      Closed without merge: 5
      Open: 1
  ▸ ...

Recommendations
  Consider contributing to established open source projects with significant community adoption.
```

Failed metrics are tinted red in the table, passed ones green, and metrics that did not apply are dimmed. Every
metric name links to the upstream page explaining how its number is derived, and each one gets a collapsible
`<details>` block showing the evidence behind it — the table tells you *what*, the details tell you *why*.

## 🚀 How to Run This Tool

### Prerequisites

- For the CLI, a GitHub token in `GH_TOKEN`. The GitHub GraphQL API this tool uses does not accept anonymous
  requests, so the token is required rather than just a way to raise rate limits.
- For the web app, a [GitHub OAuth app](https://github.com/settings/developers) whose callback URL is
  `<APP_BASE_URL>/callback`. Users log in with it and their own token runs their analyses.
- Go 1.23+ to build it, or Docker.

### Building

```bash
make build        # dist/contributor-report-web and dist/contributor-report
make build-web    # the web app only
make build-cli    # the CLI only
```

### Web app

| Variable | Purpose |
|---|---|
| `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` | The OAuth app. Without both, nobody can log in and the app is read-only |
| `APP_BASE_URL` | Where the app is served, default `http://localhost:8080`. GitHub redirects back to `<APP_BASE_URL>/callback` |
| `SESSION_KEY` | Secret for the session cookie. Without it a random one is used, so logins do not survive a restart |
| `PORT` | Listen port, default `8080` |

```bash
# Serve on :8080
GITHUB_CLIENT_ID=... GITHUB_CLIENT_SECRET=... SESSION_KEY=... ./dist/contributor-report-web

# Run it straight from source
GITHUB_CLIENT_ID=... GITHUB_CLIENT_SECRET=... go run ./cmd/contributor-report-web

# Or the published container image
docker run -e GITHUB_CLIENT_ID -e GITHUB_CLIENT_SECRET -e SESSION_KEY -e APP_BASE_URL \
    -p 8080:8080 ghcr.io/kostis-codefresh/github-user-slop-detector-web
```

Anyone can view the home page and the cached reports. To analyze a username, log in with GitHub from the banner;
only accounts whose email is an `@octopus.com` address are allowed. Logout revokes the app's access on GitHub.
Enter a username on the home page, or open `/report/<username>` directly. The last 20 reports are kept in memory
and listed on the home page. `/health` is a liveness check that also reports which OAuth settings are missing and
the `APP_BASE_URL` in use. Without the OAuth settings the app still starts, read-only, with a red banner.

### CLI

```bash
# Report on a user
GH_TOKEN=$(gh auth token) ./dist/contributor-report octocat

# Include the two repo-history metrics, which need a repository to look at
GH_TOKEN=$(gh auth token) ./dist/contributor-report --repo owner/name octocat

# Only show the evidence for metrics that failed
GH_TOKEN=$(gh auth token) ./dist/contributor-report --details failed octocat

# Run it straight from source
GH_TOKEN=$(gh auth token) go run ./cmd/contributor-report octocat
```

The report is written to `<username>.html` in the working directory — `octocat.html` for the runs above — so reports
for different users never overwrite each other. Stdout carries a numbered progress line per step, so you can watch
what the run is doing:

```
step  1/16  Analysis Window ........... done (12 months, from 2025-09-11)
step  2/16  Fetch GitHub Activity ..... done (0 PRs, 7 comments, 0 issues, 0 reviews)
step  3/16  PR Merge Rate ............. done (0%, failed)
...
step 12/16  Repo History Merge Rate ... skipped (not applicable)
step 16/16  Render Report ............. done (octocat.html)
```

### Exit codes

| Code | Meaning |
| ---- | ------- |
| `0`  | Every required metric passed |
| `1`  | A required metric failed — needs review |
| `2`  | The run itself failed: bad flag, missing token, unknown user, API error |

### Configuration

Every threshold is a flag, and they default to meaningful values rather than the permissive all-zero defaults the
GitHub Action ships with — a bare run should tell you something. Run `./dist/contributor-report -h` for the full list.

```bash
./dist/contributor-report \
  --threshold-pr-merge-rate 0.5 \
  --threshold-account-age 90 \
  --minimum-stars 500 \
  --required prMergeRate,accountAge,repoQuality \
  octocat
```

Two flags are worth calling out:

- `--repo owner/name` enables **Repo History Merge Rate** and **Repo History Min PRs**. Without it those two rows
  render as `—` and are left out of the pass count, because there is no repository for them to be about.
- `--required` names the metrics that decide the overall verdict (default `prMergeRate,accountAge`). Everything else
  is still measured and reported — that is why a report can read "Passed (12/13 metrics passed)". Pass `--required=""`
  to make nothing gate the result.

## What it does

For each username (from the web form or the CLI argument), the tool will:

1. **📅 Work out the analysis window**: twelve months back by default, configurable with `--analysis-window`
2. **📡 Fetch the contributor's activity**: one GraphQL query for the profile, pull requests, issue comments, opened
   issues and contribution calendar, paginated up to 500 PRs and 500 comments
3. **✅ Evaluate every metric**: thirteen independent checks over the fetched data, plus four spam heuristics
4. **📊 Render the report**: a self-contained HTML document with the metric table, the detail blocks and any
   recommendations — served by the web app, or written to `<username>.html` by the CLI
5. **🚪 Exit accordingly** (CLI only): non-zero when a required metric did not pass

## Project layout

```
cmd/contributor-report-web/   the web app: handlers, GitHub login and the in-memory report cache
ui/                           the web app's embedded home and login pages
cmd/contributor-report/       the CLI: flag parsing and the pipeline
pkg/analysis/                 the Contributor data model and the GitHub GraphQL fetcher
pkg/metrics/                  the Metric interface, the registry, one file per metric, the spam patterns
pkg/report/                   the embedded HTML template and the renderer
```

Adding a metric touches three places: a new `pkg/metrics/m_<name>.go` implementing `Metric`, an entry in the
`NewRegistry()` slice (its position is its position in the report table), and a `m_<name>_test.go`.

Metrics are pure functions of an already-fetched `*analysis.Contributor`, so their tests build that struct directly
and never touch the network.

## Development

```bash
make build
make test
make lint

# Refresh the report renderer's golden files after a deliberate output change
go test ./pkg/report -update
```
