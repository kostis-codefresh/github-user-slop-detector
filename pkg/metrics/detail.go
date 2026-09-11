package metrics

// Detail is the body of a metric's <details> block: a summary line and the
// content under it. The content is structured rather than pre-rendered markup
// so that pkg/report decides the markup and html/template does the escaping.
type Detail struct {
	Summary string
	Blocks  []Block
}

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
// ahead of Text: "Mergers: @a, @b", "🔴 CRITICAL: SPAM_PATTERN".
type Line struct {
	Label string
	Text  string
}
