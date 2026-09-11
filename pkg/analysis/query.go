package analysis

// contributorQuery fetches everything the metrics need in one round trip.
// The PR and issue-comment connections are paginated separately by the caller,
// which is why both cursors are variables; the other sections are re-fetched
// and discarded on those extra pages.
const contributorQuery = `
query ContributorAnalysis($username: String!, $since: DateTime!, $prCursor: String, $commentCursor: String, $issueSearchQuery: String!) {
  user(login: $username) {
    login
    createdAt
    bio
    company
    location
    websiteUrl
    followers { totalCount }
    repositories(privacy: PUBLIC) { totalCount }

    pullRequests(
      first: 100
      states: [MERGED, CLOSED, OPEN]
      orderBy: {field: CREATED_AT, direction: DESC}
      after: $prCursor
    ) {
      totalCount
      nodes {
        state
        merged
        mergedAt
        createdAt
        additions
        deletions
        mergedBy { login }
        repository {
          owner { login }
          name
          stargazerCount
        }
      }
      pageInfo { hasNextPage endCursor }
    }

    contributionsCollection(from: $since) {
      contributionCalendar {
        weeks {
          contributionDays {
            contributionCount
            date
          }
        }
      }
      pullRequestReviewContributions { totalCount }
    }

    issueComments(
      first: 100
      orderBy: {field: UPDATED_AT, direction: DESC}
      after: $commentCursor
    ) {
      totalCount
      nodes {
        createdAt
        reactions(first: 10) { nodes { content } }
      }
      pageInfo { hasNextPage endCursor }
    }
  }

  issueSearch: search(query: $issueSearchQuery, type: ISSUE_ADVANCED, first: 50) {
    issueCount
    nodes {
      __typename
      ... on Issue {
        createdAt
        comments { totalCount }
        reactions(first: 20) { nodes { content } }
      }
    }
  }

  rateLimit { remaining resetAt used }
}
`
