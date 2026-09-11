package metrics

// Detail is the body of a metric's <details> block: a summary line and the
// content under it. The content is structured rather than pre-rendered markup
// so that pkg/report decides the markup and html/template does the escaping.
type Detail struct {
	Icon    string
	Summary string
	Blocks  []Block
}

// The icons a detail can be shown with. These are names, not glyphs: the
// report's stylesheet keys its .icon-<name> rules on them, so the choice of
// picture is the template's and never Go's.
const (
	IconPRs       = "prs"       //PR Merge Rate
	IconRepos     = "repos"     //Repo Quality
	IconReactions = "reactions" //both reaction metrics
	IconAccount   = "account"   //account age and activity consistency
	IconIssues    = "issues"    //Issue Engagement
	IconReviews   = "reviews"   //Code Reviews
	IconMergers   = "mergers"   //Merger Diversity
	IconHistory   = "history"   //both repo-history metrics
	IconProfile   = "profile"   //Profile Completeness
	IconAlert     = "alert"     //spam findings present
	IconClean     = "clean"     //spam detection found nothing
	IconDisabled  = "disabled"  //spam detection switched off
)

// BlockKind selects how pkg/report renders a block.
type BlockKind string

const (
	KindText  BlockKind = "text"  //a paragraph, one line break between lines
	KindList  BlockKind = "list"  //a bullet list
	KindNote  BlockKind = "note"  //an italic aside
	KindTable BlockKind = "table" //a header row plus body rows
)

// Block is one piece of a detail body. A table carries Head and Rows; every
// other kind carries Lines.
type Block struct {
	Kind  BlockKind
	Lines []Line
	Head  []string
	Rows  [][]string
}

// Line is one paragraph line or list item. Label, when set, is emphasised
// ahead of Text: "Mergers: @a, @b", "CRITICAL: SPAM_PATTERN".
type Line struct {
	Label string
	Text  string
}
