package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxPages caps each paginated connection at 5 pages of 100 nodes. The original
// action uses the same cap; without it a prolific account would cost hundreds of
// API points for data that barely moves the metrics.
const maxPages = 5

const defaultEndpoint = "https://api.github.com/graphql"

// Client fetches contributor data from the GitHub GraphQL API.
type Client struct {
	Token    string
	Endpoint string
	HTTP     *http.Client
}

// NewClient returns a client authenticating with the given token.
func NewClient(token string) *Client {
	return &Client{
		Token:    token,
		Endpoint: defaultEndpoint,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
	}
}

// Fetch retrieves the contributor's public activity since the given date and
// assembles it into a Contributor. It asks for one section of the document per
// request: see the note on contributorQuery for why the combined document does
// not fit inside GitHub's 10-second query limit.
func (cl *Client) Fetch(ctx context.Context, username string, since time.Time) (*Contributor, error) {
	vars := map[string]any{
		"username":         username,
		"since":            since.Format(time.RFC3339),
		"prCursor":         nil,
		"commentCursor":    nil,
		"issueSearchQuery": fmt.Sprintf("author:%s is:issue created:>=%s", username, since.Format("2006-01-02")),
	}

	profile, err := cl.section(ctx, vars, "profile", withProfile)
	if err != nil {
		return nil, err
	}
	if profile.User == nil {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	c := newContributor(profile, since)

	prFirst, err := cl.section(ctx, vars, "PRs page 1", withPRs)
	if err != nil {
		return nil, err
	}
	c.RateLimit = toRateLimit(prFirst)
	prPage := userOf(prFirst).PullRequests
	c.PullRequests = convertPRs(prPage.Nodes)
	// The connection is ordered newest first, so once a page reaches a PR from
	// before the window every later page would be dropped by the filter below.
	// Stopping there saves the requests without changing the data, and is what
	// makes a shorter window cheaper on the PR side at all.
	for page := 1; page < maxPages && prPage.PageInfo.HasNextPage && !reachedBefore(prPage.Nodes, since); page++ {
		vars["prCursor"] = prPage.PageInfo.EndCursor
		next, err := cl.section(ctx, vars, fmt.Sprintf("PRs page %d", page+1), withPRs)
		if err != nil {
			return nil, err
		}
		c.RateLimit = toRateLimit(next)
		prPage = userOf(next).PullRequests
		c.PullRequests = append(c.PullRequests, convertPRs(prPage.Nodes)...)
	}

	commentFirst, err := cl.section(ctx, vars, "comments page 1", withComments)
	if err != nil {
		return nil, err
	}
	c.RateLimit = toRateLimit(commentFirst)
	commentPage := userOf(commentFirst).IssueComments
	c.IssueComments = convertComments(commentPage.Nodes)
	for page := 1; page < maxPages && commentPage.PageInfo.HasNextPage; page++ {
		vars["commentCursor"] = commentPage.PageInfo.EndCursor
		next, err := cl.section(ctx, vars, fmt.Sprintf("comments page %d", page+1), withComments)
		if err != nil {
			return nil, err
		}
		c.RateLimit = toRateLimit(next)
		commentPage = userOf(next).IssueComments
		c.IssueComments = append(c.IssueComments, convertComments(commentPage.Nodes)...)
	}

	search, err := cl.section(ctx, vars, "issue search", withSearch)
	if err != nil {
		return nil, err
	}
	c.RateLimit = toRateLimit(search)
	c.Issues = convertIssues(search.IssueSearch.Nodes)

	// Only PRs are date-filtered; see the note on Contributor.IssueComments.
	kept := c.PullRequests[:0]
	for _, pr := range c.PullRequests {
		if !pr.CreatedAt.Before(since) {
			kept = append(kept, pr)
		}
	}
	c.PullRequests = kept

	return c, nil
}

// reachedBefore reports whether the page's oldest PR predates since.
func reachedBefore(nodes []prNode, since time.Time) bool {
	return len(nodes) > 0 && nodes[len(nodes)-1].CreatedAt.Before(since)
}

// The @include flags in contributorQuery, one per section of the document.
const (
	withProfile  = "withProfile"
	withPRs      = "withPRs"
	withComments = "withComments"
	withSearch   = "withSearch"
)

// section runs the document with only the named sections switched on, so each
// request pays for the one part of it the caller actually needs.
func (cl *Client) section(ctx context.Context, vars map[string]any, label string, include ...string) (*queryData, error) {
	for _, flag := range []string{withProfile, withPRs, withComments, withSearch} {
		vars[flag] = false
	}
	for _, flag := range include {
		vars[flag] = true
	}
	return cl.query(ctx, vars, label)
}

// userOf returns the user section, or an empty one when GitHub omitted it. Only
// the profile request treats a missing user as fatal; a later section losing it
// leaves the sections already collected intact, and an empty connection ends
// the pagination loop on its own.
func userOf(d *queryData) *userNode {
	if d.User == nil {
		return &userNode{}
	}
	return d.User
}

// newContributor builds the profile half of the model: the scalars, the
// contribution calendar and the review count. The connections are appended by
// Fetch as their own requests come back.
func newContributor(d *queryData, since time.Time) *Contributor {
	u := d.User
	c := &Contributor{
		Login:        u.Login,
		CreatedAt:    u.CreatedAt,
		Bio:          strings.TrimSpace(deref(u.Bio)),
		Company:      strings.TrimSpace(deref(u.Company)),
		Location:     strings.TrimSpace(deref(u.Location)),
		WebsiteURL:   strings.TrimSpace(deref(u.WebsiteURL)),
		Followers:    u.Followers.TotalCount,
		PublicRepos:  u.Repositories.TotalCount,
		ActiveMonths: map[string]bool{},
		ReviewsGiven: u.ContributionsCollection.PullRequestReviewContributions.TotalCount,
		RateLimit:    toRateLimit(d),
		Since:        since,
		Until:        time.Now().UTC(),
	}

	for _, week := range u.ContributionsCollection.ContributionCalendar.Weeks {
		for _, day := range week.ContributionDays {
			if day.ContributionCount > 0 && len(day.Date) >= 7 {
				c.ActiveMonths[day.Date[:7]] = true
			}
		}
	}

	return c
}

// toRateLimit lifts the budget out of a response. Every section carries it, so
// the caller overwrites with each one and ends up with the freshest reading.
func toRateLimit(d *queryData) RateLimit {
	return RateLimit{Remaining: d.RateLimit.Remaining, Used: d.RateLimit.Used, ResetAt: d.RateLimit.ResetAt}
}

func convertIssues(nodes []issueNode) []Issue {
	var issues []Issue
	for _, n := range nodes {
		// Non-Issue nodes come back as empty objects; skip them.
		if n.Typename != "Issue" {
			continue
		}
		issues = append(issues, Issue{
			CreatedAt: n.CreatedAt,
			Comments:  n.Comments.TotalCount,
			Reactions: contents(n.Reactions.Nodes),
		})
	}
	return issues
}

func convertPRs(nodes []prNode) []PullRequest {
	prs := make([]PullRequest, 0, len(nodes))
	for _, n := range nodes {
		pr := PullRequest{
			State:     n.State,
			Merged:    n.Merged,
			CreatedAt: n.CreatedAt,
		}
		if n.MergedAt != nil {
			pr.MergedAt = *n.MergedAt
		}
		if n.MergedBy != nil {
			pr.MergedBy = n.MergedBy.Login
		}
		if n.Repository != nil {
			pr.RepoOwner = n.Repository.Owner.Login
			pr.RepoName = n.Repository.Name
			pr.Stars = n.Repository.StargazerCount
		}
		prs = append(prs, pr)
	}
	return prs
}

func convertComments(nodes []commentNode) []IssueComment {
	comments := make([]IssueComment, 0, len(nodes))
	for _, n := range nodes {
		comments = append(comments, IssueComment{CreatedAt: n.CreatedAt, Reactions: contents(n.Reactions.Nodes)})
	}
	return comments
}

func contents(nodes []reactionNode) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Content)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// query runs the document once, retrying transient failures. label names which
// request this is ("first page", "comments page 3") so a retry warning shows
// whether the transient failures cluster on the paginated requests, which re-run
// the whole document for one connection, or on the first one.
func (cl *Client) query(ctx context.Context, vars map[string]any, label string) (*queryData, error) {
	const attempts = 3
	var lastErr error
	for attempt := range attempts {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * time.Second
			log.Printf("Fetcher -> retrying %s after %s: %v", label, backoff, lastErr)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		data, err := cl.do(ctx, vars)
		if err == nil {
			return data, nil
		}
		if !isTransient(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func (cl *Client) do(ctx context.Context, vars map[string]any) (*queryData, error) {
	body, err := json.Marshal(map[string]any{"query": contributorQuery, "variables": vars})
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cl.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cl.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cl.HTTP.Do(req)
	if err != nil {
		return nil, transientError{fmt.Errorf("call GitHub: %w", err)}
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, transientError{fmt.Errorf("read response: %w", err)}
	}

	remaining, resetAt := rateLimitFromHeaders(resp.Header)
	detail := strings.TrimSpace(string(payload))

	// GitHub reports an exhausted budget as 429, or as 403 with the remaining
	// count zeroed (the primary limit) or an "exceeded a secondary rate limit"
	// body. A 403 that is really a bad token has neither, so it falls through.
	if resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && (remaining == 0 || mentionsRateLimit(detail))) {
		return nil, &rateLimitError{Remaining: remaining, ResetAt: resetAt, Detail: detail}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("GitHub rejected the token (HTTP %d): %s", resp.StatusCode, detail)
	}
	// GitHub gives a GraphQL query 10 seconds and answers one it could not finish
	// with a 502 or a 504. Retrying re-runs the same too-expensive document, and
	// the docs warn that a timeout "deducts additional points from your primary
	// rate limit for the next hour" — so this stops rather than paying that twice
	// more for the same likely outcome. Other 5xx are genuine blips, still retried.
	if resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout {
		return nil, fmt.Errorf("%w: %s", ErrQueryTimeout, serverErrorMessage(resp, detail,
			fmt.Sprintf("HTTP %d, not retrying the same query because each timeout costs extra rate-limit points",
				resp.StatusCode)))
	}
	if resp.StatusCode >= 500 {
		return nil, transientError{errors.New(serverErrorMessage(resp, detail,
			fmt.Sprintf("GitHub returned HTTP %d", resp.StatusCode)))}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned HTTP %d: %s", resp.StatusCode, detail)
	}

	var out graphQLResponse
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(out.Errors) > 0 {
		messages := make([]string, 0, len(out.Errors))
		for _, e := range out.Errors {
			messages = append(messages, e.Message)
		}
		joined := strings.Join(messages, "; ")
		if mentionsRateLimit(joined) {
			return nil, &rateLimitError{Remaining: remaining, ResetAt: resetAt, Detail: joined}
		}
		return nil, fmt.Errorf("GraphQL: %s", joined)
	}
	if out.Data == nil {
		return nil, fmt.Errorf("GraphQL returned no data")
	}
	return out.Data, nil
}

// ErrQueryTimeout marks a query GitHub could not finish inside its 10s limit.
// Re-sending the same document would time out again, so the caller is expected
// to retry with a shorter window instead.
var ErrQueryTimeout = errors.New("GitHub could not answer the query within its 10s limit")

// rateLimitError ends the run on the first response that reports it. Unlike a
// 5xx, an exhausted budget does not clear inside a few seconds of backoff, so
// retrying would only spend more of a budget that is already gone.
type rateLimitError struct {
	Remaining int //-1 when GitHub did not say
	ResetAt   time.Time
	Detail    string
}

func (e *rateLimitError) Error() string {
	msg := "GitHub API rate limit exceeded (not retrying)"
	if e.Remaining >= 0 {
		msg += fmt.Sprintf(", %d points remaining", e.Remaining)
	}
	if until := time.Until(e.ResetAt); until > 0 {
		msg += fmt.Sprintf(", resets at %s (in %s)", e.ResetAt.UTC().Format(time.RFC3339), until.Round(time.Second))
	}
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return msg
}

func mentionsRateLimit(s string) bool { return strings.Contains(strings.ToLower(s), "rate limit") }

// serverErrorMessage appends the request ID and the body to a 5xx summary. The
// ID is the only handle GitHub support has on a recurring failure, and the body
// carries the reason; every other error path reports the body, so a 5xx that
// dropped it just hid the cause.
func serverErrorMessage(resp *http.Response, detail, summary string) string {
	if id := resp.Header.Get("X-GitHub-Request-Id"); id != "" {
		summary += " (request " + id + ")"
	}
	if detail != "" {
		summary += ": " + oneLine(detail, 300)
	}
	return summary
}

// oneLine flattens and shortens a server-supplied body so it fits in a progress
// line: a 5xx from the edge rather than from the API can be a whole HTML page.
func oneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > limit {
		s = s[:limit] + "..."
	}
	return s
}

// rateLimitFromHeaders reads the budget GitHub attaches to every response.
// X-RateLimit-Reset is an absolute Unix time; secondary rate limits omit it and
// send Retry-After as a delta in seconds instead. remaining is -1 when absent.
func rateLimitFromHeaders(h http.Header) (remaining int, resetAt time.Time) {
	remaining = -1
	if v, err := strconv.Atoi(h.Get("X-RateLimit-Remaining")); err == nil {
		remaining = v
	}
	if v, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		return remaining, time.Unix(v, 0).UTC()
	}
	if v, err := strconv.Atoi(h.Get("Retry-After")); err == nil {
		return remaining, time.Now().UTC().Add(time.Duration(v) * time.Second)
	}
	return remaining, time.Time{}
}

// transientError marks a failure worth retrying.
type transientError struct{ error }

func isTransient(err error) bool {
	var t transientError
	return errors.As(err, &t)
}
