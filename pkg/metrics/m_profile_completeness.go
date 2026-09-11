// Profile Completeness: how much of a real presence the account projects.
package metrics

import (
	"fmt"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
)

// Profile scoring weights, out of 100. Followers and repositories are scored in
// steps rather than per-unit so that a handful of each already counts for
// something and a large following cannot dominate the score.
const (
	followersBase = 20 //more than zero followers
	followersMid  = 10 //at least ten
	followersHigh = 10 //at least fifty
	reposBase     = 15 //more than zero public repos
	reposHigh     = 5  //at least five
	bioPoints     = 20
	companyPoints = 20
)

// ProfileCompleteness scores bio, company, followers and public repositories.
type ProfileCompleteness struct{}

func (m *ProfileCompleteness) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "profileCompleteness",
		Name:        "Profile Completeness",
		DocSlug:     "profile-completeness",
		Description: "Profile richness (bio, followers)",
	}
}

// followerPoints scores the follower count in steps, capped at 40.
func followerPoints(followers int) int {
	switch {
	case followers >= 50:
		return followersBase + followersMid + followersHigh
	case followers >= 10:
		return followersBase + followersMid
	case followers > 0:
		return followersBase
	default:
		return 0
	}
}

// repoPoints scores the public repository count in steps, capped at 20.
func repoPoints(repos int) int {
	switch {
	case repos >= 5:
		return reposBase + reposHigh
	case repos > 0:
		return reposBase
	default:
		return 0
	}
}

// profileScore is the 0-100 total. Location and website are shown on a profile
// but carry no points, matching the upstream scoring.
func profileScore(c *analysis.Contributor) int {
	score := followerPoints(c.Followers) + repoPoints(c.PublicRepos)
	if c.Bio != "" {
		score += bioPoints
	}
	if c.Company != "" {
		score += companyPoints
	}
	return score
}

func (m *ProfileCompleteness) Evaluate(c *analysis.Contributor, cfg Config) Result {
	result := Result{
		Active:     true,
		Passed:     profileScore(c) >= cfg.Thresholds.ProfileCompleteness,
		Value:      count(profileScore(c)),
		Threshold:  atLeast(cfg.Thresholds.ProfileCompleteness),
		DataPoints: 1,
	}
	if missing := missingProfileParts(c); len(missing) > 0 {
		result.Recommendation = "Complete your GitHub profile by adding: " +
			joinWithCommas(missing) + ". A complete profile builds trust with maintainers."
	}
	return result
}

// missingProfileParts lists the profile fields worth filling in, in the order
// they are worth the most effort.
func missingProfileParts(c *analysis.Contributor) []string {
	var missing []string
	if c.Bio == "" {
		missing = append(missing, "bio")
	}
	if c.Company == "" {
		missing = append(missing, "company/affiliation")
	}
	if c.Followers == 0 {
		missing = append(missing, "GitHub followers (engage with community)")
	}
	return missing
}

func (m *ProfileCompleteness) DetailGroup() string { return "profileCompleteness" }

func (m *ProfileCompleteness) Detail(c *analysis.Contributor, _ Config) Detail {
	bio, company := 0, 0
	if c.Bio != "" {
		bio = bioPoints
	}
	if c.Company != "" {
		company = companyPoints
	}
	return Detail{
		Icon:    IconProfile,
		Summary: fmt.Sprintf("Score: %d/100", profileScore(c)),
		Blocks: []Block{table(
			[]string{"Component", "Status", "Points"},
			[]string{"Followers", count(c.Followers), fmt.Sprintf("+%d/40", followerPoints(c.Followers))},
			[]string{"Public repos", count(c.PublicRepos), fmt.Sprintf("+%d/20", repoPoints(c.PublicRepos))},
			[]string{"Bio", yesNo(c.Bio != ""), fmt.Sprintf("+%d/20", bio)},
			[]string{"Company", yesNo(c.Company != ""), fmt.Sprintf("+%d/20", company)},
		)},
	}
}
