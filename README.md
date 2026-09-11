# Contributor Report 🔍

A single-binary Go CLI that analyses a GitHub user's public activity and writes an HTML contributor report, to
help maintainers spot low-effort or AI-generated pull requests ("AI slop") before spending review time on them.

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
to copy alongside it. Open `report.html` in a browser, or attach it to a ticket as-is. Its content reads like this:

```
✅ Contributor Report
@jdiegosierra — Passed (12/13 metrics passed)

| Metric | Description | Value | Threshold | Status |
|--------|-------------|-------|-----------|--------|
| [PR Merge Rate](...) | PRs merged vs closed | 84% | >= 30% | ✅ |
| [Repo Quality](...) | Repos with ≥100 stars | 1 | >= 2 | ❌ |
| [Positive Reactions](...) | Positive reactions received | 43 | >= 5 | ✅ |
| [Negative Reactions](...) | Negative reactions received | 0 | <= 5 | ✅ |
| [Account Age](...) | GitHub account age | 3432 days | >= 30 days | ✅ |
| [Activity Consistency](...) | Regular activity over time | 100% | >= 50% | ✅ |
| [Issue Engagement](...) | Issues with community engagement | 5 | >= 1 | ✅ |
| [Code Reviews](...) | Code reviews given to others | 4 | >= 1 | ✅ |
| [Merger Diversity](...) | Unique maintainers who merged PRs | 2 | >= 2 | ✅ |
| [Repo History Merge Rate](...) | Merge rate in this repo | 89% | >= 0% | ✅ |
| [Repo History Min PRs](...) | Previous PRs in this repo | 28 | >= 0 | ✅ |
| [Profile Completeness](...) | Profile richness (bio, followers) | 90 | >= 20 | ✅ |
| [Suspicious Patterns](...) | Spam-like activity detection | 0 | N/A | ✅ |

Metric Details

  ▸ 📊 84% merge rate (26/32 PRs)
      - Merged: 26
      - Closed without merge: 5
      - Open: 1
      - Avg PR size: 2005.3 lines
      - Very short PRs (<10 lines): 11

  ▸ ...

Recommendations

- Consider contributing to established open source projects with significant community adoption.
```

Failed metrics are tinted red in the table, passed ones green, and metrics that did not apply are dimmed. Each metric
also gets a collapsible `<details>` block explaining how its number was reached — the table tells you *what*, the
details tell you *why*.

## 🚀 How to Run This Tool

### Prerequisites

- A GitHub token in `GH_TOKEN`. The GitHub GraphQL API this tool uses does not accept anonymous requests, so the
  token is required rather than just a way to raise rate limits.
- Go 1.23+ to build it.

### Building

```bash
go build -o contributor-report .
```

### Running

```bash
# Report on a user
GH_TOKEN=$(gh auth token) ./contributor-report octocat

# Include the two repo-history metrics, which need a repository to look at
GH_TOKEN=$(gh auth token) ./contributor-report --repo owner/name octocat

# Only show the evidence for metrics that failed
GH_TOKEN=$(gh auth token) ./contributor-report --details failed octocat

# Run it straight from source
GH_TOKEN=$(gh auth token) go run . octocat
```

The report is written to `report.html` in the working directory. Stdout carries a numbered progress line per step, so
you can watch what the run is doing:

```
step  1/16  Analysis Window ........... done (12 months, from 2025-09-11)
step  2/16  Fetch GitHub Activity ..... done (0 PRs, 7 comments, 0 issues, 0 reviews)
step  3/16  PR Merge Rate ............. done (0%, failed)
...
step 12/16  Repo History Merge Rate ... skipped (not applicable)
step 16/16  Render Report ............. done (report.html)
```

### Exit codes

| Code | Meaning |
| ---- | ------- |
| `0`  | Every required metric passed |
| `1`  | A required metric failed — needs review |
| `2`  | The run itself failed: bad flag, missing token, unknown user, API error |

### Configuration

Every threshold is a flag, and they default to meaningful values rather than the permissive all-zero defaults the
GitHub Action ships with — a bare run should tell you something. Run `./contributor-report -h` for the full list.

```bash
./contributor-report \
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

When you run it, the tool will:

1. **📅 Work out the analysis window**: twelve months back by default, configurable with `--analysis-window`
2. **📡 Fetch the contributor's activity**: one GraphQL query for the profile, pull requests, issue comments, opened
   issues and contribution calendar, paginated up to 500 PRs and 500 comments
3. **✅ Evaluate every metric**: thirteen independent checks over the fetched data, plus four spam heuristics
4. **📊 Render the report**: a self-contained HTML document written to `report.html`, with the metric table, the
   detail blocks and any recommendations
5. **🚪 Exit accordingly**: non-zero when a required metric did not pass

## Project layout

```
main.go            flag parsing and the pipeline
pkg/analysis/      the Contributor data model and the GitHub GraphQL fetcher
pkg/metrics/       the Metric interface, the registry, one file per metric, the spam patterns
pkg/report/        the embedded HTML template, the renderer and the markdown-to-HTML converter
```

Adding a metric touches three places: a new `pkg/metrics/m_<name>.go` implementing `Metric`, an entry in the
`NewRegistry()` slice (its position is its position in the report table), and a `m_<name>_test.go`.

Metrics are pure functions of an already-fetched `*analysis.Contributor`, so their tests build that struct directly
and never touch the network.

## Development

```bash
go build ./...
go vet ./...
go test ./...

# Refresh the report renderer's golden files after a deliberate output change
go test ./pkg/report -update
```
