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
// assembles it into a Contributor.
func (cl *Client) Fetch(ctx context.Context, username string, since time.Time) (*Contributor, error) {
	vars := map[string]any{
		"username":         username,
		"since":            since.Format(time.RFC3339),
		"prCursor":         nil,
		"commentCursor":    nil,
		"issueSearchQuery": fmt.Sprintf("author:%s is:issue created:>=%s", username, since.Format("2006-01-02")),
	}

	first, err := cl.query(ctx, vars)
	if err != nil {
		return nil, err
	}
	if first.User == nil {
		return nil, fmt.Errorf("user not found: %s", username)
	}

	c := newContributor(first, since)

	prPage := first.User.PullRequests
	for page := 1; page < maxPages && prPage.PageInfo.HasNextPage; page++ {
		vars["prCursor"] = prPage.PageInfo.EndCursor
		vars["commentCursor"] = nil
		next, err := cl.query(ctx, vars)
		if err != nil {
			return nil, err
		}
		if next.User == nil {
			break
		}
		prPage = next.User.PullRequests
		c.PullRequests = append(c.PullRequests, convertPRs(prPage.Nodes)...)
	}

	commentPage := first.User.IssueComments
	for page := 1; page < maxPages && commentPage.PageInfo.HasNextPage; page++ {
		vars["prCursor"] = nil
		vars["commentCursor"] = commentPage.PageInfo.EndCursor
		next, err := cl.query(ctx, vars)
		if err != nil {
			return nil, err
		}
		if next.User == nil {
			break
		}
		commentPage = next.User.IssueComments
		c.IssueComments = append(c.IssueComments, convertComments(commentPage.Nodes)...)
	}

	// Only PRs are date-filtered; see the note on Contributor.IssueComments.
	kept := c.PullRequests[:0]
	for _, pr := range c.PullRequests {
		if !pr.CreatedAt.Before(since) {
			kept = append(kept, pr)
		}
	}
	c.PullRequests = kept

	log.Printf("Fetcher -> %s: %d PRs, %d comments, %d issues, %d reviews",
		username, len(c.PullRequests), len(c.IssueComments), len(c.Issues), c.ReviewsGiven)
	return c, nil
}

// newContributor builds the model from the first response page.
func newContributor(d *queryData, since time.Time) *Contributor {
	u := d.User
	c := &Contributor{
		Login:         u.Login,
		CreatedAt:     u.CreatedAt,
		Bio:           strings.TrimSpace(deref(u.Bio)),
		Company:       strings.TrimSpace(deref(u.Company)),
		Location:      strings.TrimSpace(deref(u.Location)),
		WebsiteURL:    strings.TrimSpace(deref(u.WebsiteURL)),
		Followers:     u.Followers.TotalCount,
		PublicRepos:   u.Repositories.TotalCount,
		PullRequests:  convertPRs(u.PullRequests.Nodes),
		IssueComments: convertComments(u.IssueComments.Nodes),
		ActiveMonths:  map[string]bool{},
		ReviewsGiven:  u.ContributionsCollection.PullRequestReviewContributions.TotalCount,
		Since:         since,
		Until:         time.Now().UTC(),
	}

	for _, week := range u.ContributionsCollection.ContributionCalendar.Weeks {
		for _, day := range week.ContributionDays {
			if day.ContributionCount > 0 && len(day.Date) >= 7 {
				c.ActiveMonths[day.Date[:7]] = true
			}
		}
	}

	for _, n := range d.IssueSearch.Nodes {
		// Non-Issue nodes come back as empty objects; skip them.
		if n.Typename != "Issue" {
			continue
		}
		c.Issues = append(c.Issues, Issue{
			CreatedAt: n.CreatedAt,
			Comments:  n.Comments.TotalCount,
			Reactions: contents(n.Reactions.Nodes),
		})
	}

	return c
}

func convertPRs(nodes []prNode) []PullRequest {
	prs := make([]PullRequest, 0, len(nodes))
	for _, n := range nodes {
		pr := PullRequest{
			State:     n.State,
			Merged:    n.Merged,
			CreatedAt: n.CreatedAt,
			Additions: n.Additions,
			Deletions: n.Deletions,
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

// query runs the document once, retrying transient failures.
func (cl *Client) query(ctx context.Context, vars map[string]any) (*queryData, error) {
	const attempts = 3
	var lastErr error
	for attempt := range attempts {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * time.Second
			log.Printf("Fetcher -> retrying after %s: %v", backoff, lastErr)
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

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("GitHub rejected the token (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return nil, transientError{fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
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
		if strings.Contains(strings.ToLower(joined), "rate limit") {
			return nil, transientError{fmt.Errorf("GraphQL: %s", joined)}
		}
		return nil, fmt.Errorf("GraphQL: %s", joined)
	}
	if out.Data == nil {
		return nil, fmt.Errorf("GraphQL returned no data")
	}
	return out.Data, nil
}

// transientError marks a failure worth retrying.
type transientError struct{ error }

func isTransient(err error) bool {
	var t transientError
	return errors.As(err, &t)
}
