package report

import (
	"html"
	"html/template"
	"regexp"
	"strings"
)

// The metrics build their detail bodies as small markdown strings (see
// bullets() in pkg/metrics/common.go), so the renderer has to translate them.
// This handles exactly the five constructs the metrics emit — bullet lists,
// tables, bold, whole-line italic and plain paragraphs — and nothing else; it
// is not a general markdown parser.

// boldPattern matches the **bold** runs used for the merger list and the spam
// detection headings. Asterisks cannot appear in the data those lines embed
// (logins, repository names, severities), so matching pairs anywhere is safe.
var boldPattern = regexp.MustCompile(`\*\*(.+?)\*\*`)

// wholeLineItalic matches a line that is entirely wrapped in underscores, which
// is the only italic in the codebase (the "...and N more repositories" note in
// m_repo_quality.go). Matching underscore pairs anywhere would corrupt the spam
// evidence keys (threshold_accountAge) and repository names that legally
// contain an underscore.
var wholeLineItalic = regexp.MustCompile(`^_(.+)_$`)

// tableSeparator matches the |---|---| line under a table's header row.
var tableSeparator = regexp.MustCompile(`^\|[-|: ]+\|$`)

// bodyHTML converts a metric's detail body to HTML. The result is marked safe,
// so every piece of text must pass through inlineHTML to be escaped first: the
// bodies embed repository names, logins and spam evidence.
func bodyHTML(body string) template.HTML {
	var out strings.Builder
	lines := strings.Split(body, "\n")

	for at := 0; at < len(lines); {
		switch {
		case strings.TrimSpace(lines[at]) == "":
			at++
		case strings.HasPrefix(lines[at], "|"):
			at = writeTable(&out, lines, at)
		case strings.HasPrefix(lines[at], "- "):
			at = writeList(&out, lines, at)
		default:
			at = writeParagraph(&out, lines, at)
		}
	}
	return template.HTML(out.String())
}

// writeTable consumes the run of pipe-prefixed lines starting at from. The
// first is the header and the second the |---| separator; a paragraph that
// merely contains a pipe ("Avg stars: 2279 | Highest: 24127") never gets here
// because it does not start with one.
func writeTable(out *strings.Builder, lines []string, from int) int {
	at := from
	out.WriteString("<table>\n<thead>\n")
	writeRow(out, lines[at], "th")
	out.WriteString("</thead>\n")
	at++

	if at < len(lines) && tableSeparator.MatchString(strings.TrimSpace(lines[at])) {
		at++
	}

	out.WriteString("<tbody>\n")
	for ; at < len(lines) && strings.HasPrefix(lines[at], "|"); at++ {
		writeRow(out, lines[at], "td")
	}
	out.WriteString("</tbody>\n</table>\n")
	return at
}

// writeRow renders one "| a | b |" line as a table row of the given cell tag.
func writeRow(out *strings.Builder, line, cellTag string) {
	openTag, closeTag := "<"+cellTag+">", "</"+cellTag+">"
	out.WriteString("<tr>")
	for _, cell := range strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|") {
		out.WriteString(openTag)
		out.WriteString(inlineHTML(cell))
		out.WriteString(closeTag)
	}
	out.WriteString("</tr>\n")
}

// writeList consumes the run of "- " lines starting at from.
func writeList(out *strings.Builder, lines []string, from int) int {
	at := from
	out.WriteString("<ul>\n")
	for ; at < len(lines) && strings.HasPrefix(lines[at], "- "); at++ {
		out.WriteString("<li>")
		out.WriteString(inlineHTML(strings.TrimPrefix(lines[at], "- ")))
		out.WriteString("</li>\n")
	}
	out.WriteString("</ul>\n")
	return at
}

// writeParagraph consumes the run of plain lines starting at from. Consecutive
// lines are one paragraph with hard breaks, which is how the spam detail reads:
// a bold heading line followed by its description.
func writeParagraph(out *strings.Builder, lines []string, from int) int {
	at := from
	var rendered []string
	for ; at < len(lines); at++ {
		line := lines[at]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "- ") {
			break
		}
		rendered = append(rendered, inlineHTML(line))
	}
	out.WriteString("<p>")
	out.WriteString(strings.Join(rendered, "<br>\n"))
	out.WriteString("</p>\n")
	return at
}

// inlineHTML escapes one line and then applies the inline markup. Escaping
// comes first so that neither the data nor the markup it produces can inject
// tags; the markdown markers themselves survive escaping untouched.
func inlineHTML(line string) string {
	escaped := html.EscapeString(strings.TrimSpace(line))
	escaped = wholeLineItalic.ReplaceAllString(escaped, "<em>$1</em>")
	return boldPattern.ReplaceAllString(escaped, "<strong>$1</strong>")
}
