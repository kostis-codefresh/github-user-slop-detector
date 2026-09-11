// Positive and Negative Reactions: how the community responds to the
// contributor's comments and issues. Both metrics read the same tally, so they
// share one <details> block via the embedded reactionsDetail.
package metrics

import (
	"fmt"

	"github.com/kkp/contributor-report/pkg/analysis"
)

// reactionsDetail is the <details> block shared by both reaction metrics.
type reactionsDetail struct{}

func (reactionsDetail) DetailGroup() string { return "reactions" }

func (reactionsDetail) Detail(c *analysis.Contributor, _ Config) Detail {
	tally := collectReactions(c)
	if tally.comments == 0 {
		return Detail{Summary: "👍 No comments analyzed", Body: "No comments found in the analysis window."}
	}
	return Detail{
		Summary: fmt.Sprintf("👍 %d positive / %d negative", tally.positive, tally.negative),
		Body: bullets(
			fmt.Sprintf("- Comments analyzed: %d", tally.comments),
			fmt.Sprintf("- Positive reactions: %d", tally.positive),
			fmt.Sprintf("- Negative reactions: %d", tally.negative),
			fmt.Sprintf("- Neutral reactions: %d", tally.neutral),
			"- Positive ratio: "+pct(tally.ratio()),
		),
	}
}

// PositiveReactions counts thumbs-up, heart, rocket and hooray reactions.
type PositiveReactions struct{ reactionsDetail }

func (m *PositiveReactions) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "positiveReactions",
		Name:        "Positive Reactions",
		DocSlug:     "positive-reactions",
		Description: "Positive reactions received",
	}
}

func (m *PositiveReactions) Evaluate(c *analysis.Contributor, cfg Config) Result {
	tally := collectReactions(c)
	return Result{
		Active:         true,
		Passed:         tally.positive >= cfg.Thresholds.PositiveReactions,
		Value:          count(tally.positive),
		Threshold:      atLeast(cfg.Thresholds.PositiveReactions),
		DataPoints:     tally.comments,
		Recommendation: "Engage more with the community through helpful comments and discussions.",
	}
}

// NegativeReactions counts thumbs-down and confused reactions. Unlike every
// other metric its threshold is a ceiling.
type NegativeReactions struct{ reactionsDetail }

func (m *NegativeReactions) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "negativeReactions",
		Name:        "Negative Reactions",
		DocSlug:     "negative-reactions",
		Description: "Negative reactions received",
	}
}

func (m *NegativeReactions) Evaluate(c *analysis.Contributor, cfg Config) Result {
	tally := collectReactions(c)
	return Result{
		Active:         true,
		Passed:         tally.negative <= cfg.Thresholds.NegativeReactions,
		Value:          count(tally.negative),
		Threshold:      fmt.Sprintf("<= %d", cfg.Thresholds.NegativeReactions),
		DataPoints:     tally.comments,
		Recommendation: "Focus on constructive communication to improve community reception.",
	}
}
