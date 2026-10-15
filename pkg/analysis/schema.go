package analysis

import "time"

// The types below mirror the GraphQL response shape one-to-one. They exist only
// to be decoded and immediately converted into the Contributor model.
//
// The node lists hold pointers because GitHub nulls out a node the token may not
// see (an organization's SAML enforcement) and still answers with the rest. A
// value slice would decode that null as a zero-dated PR or comment.

type graphQLResponse struct {
	Data   *queryData     `json:"data"`
	Errors []graphQLError `json:"errors"`
}

type graphQLError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type queryData struct {
	User        *userNode `json:"user"`
	IssueSearch struct {
		IssueCount int          `json:"issueCount"`
		Nodes      []*issueNode `json:"nodes"`
	} `json:"issueSearch"`
	RateLimit struct {
		Remaining int       `json:"remaining"`
		ResetAt   time.Time `json:"resetAt"`
		Used      int       `json:"used"`
	} `json:"rateLimit"`

	// hiddenBySAML counts the resources GitHub withheld from this response
	// because of an organization's SAML enforcement. Set by do, not decoded.
	hiddenBySAML int
}

type userNode struct {
	Login        string    `json:"login"`
	CreatedAt    time.Time `json:"createdAt"`
	Bio          *string   `json:"bio"`
	Company      *string   `json:"company"`
	Location     *string   `json:"location"`
	WebsiteURL   *string   `json:"websiteUrl"`
	Followers    countNode `json:"followers"`
	Repositories countNode `json:"repositories"`

	PullRequests struct {
		TotalCount int       `json:"totalCount"`
		Nodes      []*prNode `json:"nodes"`
		PageInfo   pageInfo  `json:"pageInfo"`
	} `json:"pullRequests"`

	ContributionsCollection struct {
		ContributionCalendar struct {
			Weeks []struct {
				ContributionDays []struct {
					ContributionCount int    `json:"contributionCount"`
					Date              string `json:"date"`
				} `json:"contributionDays"`
			} `json:"weeks"`
		} `json:"contributionCalendar"`
		PullRequestReviewContributions countNode `json:"pullRequestReviewContributions"`
	} `json:"contributionsCollection"`

	IssueComments struct {
		TotalCount int            `json:"totalCount"`
		Nodes      []*commentNode `json:"nodes"`
		PageInfo   pageInfo       `json:"pageInfo"`
	} `json:"issueComments"`
}

type countNode struct {
	TotalCount int `json:"totalCount"`
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type prNode struct {
	State     string     `json:"state"`
	Merged    bool       `json:"merged"`
	CreatedAt time.Time  `json:"createdAt"`
	MergedAt  *time.Time `json:"mergedAt"`
	MergedBy  *struct {
		Login string `json:"login"`
	} `json:"mergedBy"`
	Repository *struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name           string `json:"name"`
		StargazerCount int    `json:"stargazerCount"`
	} `json:"repository"`
}

type reactionNode struct {
	Content string `json:"content"`
}

type reactionsConnection struct {
	Nodes []reactionNode `json:"nodes"`
}

type commentNode struct {
	CreatedAt time.Time           `json:"createdAt"`
	Reactions reactionsConnection `json:"reactions"`
}

type issueNode struct {
	Typename  string              `json:"__typename"`
	CreatedAt time.Time           `json:"createdAt"`
	Comments  countNode           `json:"comments"`
	Reactions reactionsConnection `json:"reactions"`
}
