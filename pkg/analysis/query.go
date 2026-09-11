package analysis

// contributorQuery fetches everything the metrics need, but one section at a
// time. GitHub gives a query 10 seconds and answers one that runs over with a
// 502; measured against a prolific account the sections cost ~4.7s (PRs), ~6.7s
// (comments), ~1.5s (calendar) and ~2.1s (search) — 15s together, so asking for
// all of them at once times out. The @include flags let the caller pay for one
// section per request, which keeps the worst single request at ~6.7s.
//
// This is a transport change only: every field and page size is what the
// combined document asked for, so no metric sees different data. The upstream
// action sends all sections together and retries the resulting 502s instead.
//
// The PR and issue-comment connections are paginated by the caller, which is
// why both cursors are variables. login stays outside the profile fragment
// because a selection set cannot be empty and Fetch needs it to tell a missing
// user from an empty one.
const contributorQuery = `
query ContributorAnalysis(
  $username: String!
  $since: DateTime!
  $prCursor: String
  $commentCursor: String
  $issueSearchQuery: String!
  $withProfile: Boolean!
  $withPRs: Boolean!
  $withComments: Boolean!
  $withSearch: Boolean!
) {
  user(login: $username) {
    login

    ... on User @include(if: $withProfile) {
      createdAt
      bio
      company
      location
      websiteUrl
      followers { totalCount }
      repositories(privacy: PUBLIC) { totalCount }

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
    }

    pullRequests(
      first: 100
      states: [MERGED, CLOSED, OPEN]
      orderBy: {field: CREATED_AT, direction: DESC}
      after: $prCursor
    ) @include(if: $withPRs) {
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

    issueComments(
      first: 100
      orderBy: {field: UPDATED_AT, direction: DESC}
      after: $commentCursor
    ) @include(if: $withComments) {
      totalCount
      nodes {
        createdAt
        reactions(first: 10) { nodes { content } }
      }
      pageInfo { hasNextPage endCursor }
    }
  }

  issueSearch: search(query: $issueSearchQuery, type: ISSUE_ADVANCED, first: 50) @include(if: $withSearch) {
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
