{{define "blocks"}}{{range .}}{{if eq .Kind "table"}}<table>
<thead>
<tr>{{range .Head}}<th>{{.}}</th>{{end}}</tr>
</thead>
<tbody>
{{range .Rows}}<tr>{{range .}}<td>{{template "cell" .}}</td>{{end}}</tr>
{{end}}</tbody>
</table>
{{else if eq .Kind "list"}}<ul>
{{range .Lines}}<li>{{template "line" .}}</li>
{{end}}</ul>
{{else if eq .Kind "note"}}<p><em>{{range .Lines}}{{.Text}}{{end}}</em></p>
{{else}}<p>{{range $i, $l := .Lines}}{{if $i}}<br>
{{end}}{{template "line" $l}}{{end}}</p>
{{end}}{{end}}{{end -}}
{{define "line"}}{{if .Label}}<strong>{{.Label}}</strong>{{if .Text}} {{end}}{{end}}{{.Text}}{{end -}}
{{/* "yes" and "no" are the only cell values that are not shown verbatim: metrics
     emit the keyword and the tick is chosen here. See yesNo in pkg/metrics. */ -}}
{{define "cell"}}{{if eq . "yes"}}<span class="tick-yes">✅</span>{{else if eq . "no"}}<span class="tick-no">❌</span>{{else}}{{.}}{{end}}{{end -}}
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Contributor Report - @{{.Username}}</title>
<style>
/* The palette and font stack match the web app's pages (ui/index.html), so a
   report reads as part of the same site; the CLI's file gets the same look. */
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  line-height: 1.6;
  color: #1f2328;
  background-color: #f6f8fa;
  padding: 2rem;
}

a {
  color: #0969da;
  text-decoration: none;
}

a:hover {
  text-decoration: underline;
}

.masthead {
  max-width: 1200px;
  margin: 0 auto 1.5rem;
}

.home {
  display: inline-block;
  margin-bottom: 1rem;
  font-size: 0.875rem;
}

.title {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.title svg {
  width: 3rem;
  height: 3rem;
  flex-shrink: 0;
}

h1 {
  font-size: 2rem;
  font-weight: 600;
  line-height: 1.25;
  overflow-wrap: anywhere;
}

.report-info {
  color: #59636e;
  font-size: 0.95rem;
}

h2 {
  font-size: 1.25rem;
  font-weight: 600;
  margin: 2rem 0 1rem 0;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid #d8dee4;
}

.report {
  max-width: 1200px;
  margin: 0 auto;
  background: #fff;
  padding: 2rem;
  border: 1px solid #d0d7de;
  border-radius: 8px;
}

/* Headline verdict, tinted the same way as the table rows it summarises */
.verdict {
  font-size: 1.25rem;
  font-weight: 600;
  padding: 1rem 1.25rem;
  border-radius: 6px;
  border-left: 4px solid #d0d7de;
}

.verdict-passed {
  background-color: #dafbe1;
  border-left-color: #1a7f37;
  color: #116329;
}

.verdict-needs-review {
  background-color: #fff8c5;
  border-left-color: #bf8700;
  color: #7d4e00;
}

.note {
  margin-top: 1rem;
  padding: 0.75rem 1rem;
  background-color: #f6f8fa;
  border-left: 4px solid #d0d7de;
  border-radius: 4px;
  color: #59636e;
  font-size: 0.95rem;
}

/* Metric table. The wrapper scrolls the five columns inside the card at phone
   width, instead of widening the whole page. */
.table-scroll {
  overflow-x: auto;
  border: 1px solid #d0d7de;
  border-radius: 6px;
}

.metrics-table {
  width: 100%;
  border-collapse: collapse;
}

.metrics-table thead {
  background: #f6f8fa;
  color: #59636e;
}

.metrics-table th {
  padding: 0.75rem 1rem;
  text-align: left;
  font-weight: 600;
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  border-bottom: 1px solid #d0d7de;
}

.metrics-table td {
  padding: 0.75rem 1rem;
  border-bottom: 1px solid #d8dee4;
  vertical-align: top;
}

.metrics-table tbody tr:hover {
  background-color: #f6f8fa;
}

.metrics-table tbody tr:last-child td {
  border-bottom: none;
}

.metric-name a {
  font-weight: 600;
}

.metric-description {
  font-size: 0.95rem;
}

.metric-value,
.metric-threshold {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.9rem;
  white-space: nowrap;
}

.metric-value {
  font-weight: 700;
}

.metric-threshold {
  color: #59636e;
}

.metric-status {
  text-align: center;
}

/* One table, with the verdict carried by a left border per row */
.metrics-table .status-passed {
  border-left: 4px solid #1a7f37;
}

.metrics-table .status-failed {
  border-left: 4px solid #cf222e;
}

.metrics-table .status-failed .metric-value {
  color: #a40e26;
}

/* An inactive metric was never evaluated, so it is dimmed rather than judged */
.metrics-table .status-inactive {
  border-left: 4px solid #d0d7de;
  opacity: 0.7;
}

.metrics-table .status-inactive .metric-description {
  color: #59636e;
}

/* Collapsible metric details */
details {
  margin-bottom: 1rem;
  background: #fff;
  border: 1px solid #d0d7de;
  border-left: 4px solid #d0d7de;
  border-radius: 6px;
}

details.failed {
  border-left-color: #cf222e;
}

summary {
  padding: 0.75rem 1rem;
  font-weight: 600;
  cursor: pointer;
  background-color: #f6f8fa;
  border-radius: 4px;
}

summary:hover {
  background-color: #eaeef2;
}

/* The detail icons are decoration, so the glyphs live here rather than in the
   summary text. pkg/metrics picks the name; this picks the picture. */
/* A summary with no icon class generates no ::before box, so this is inert there */
summary::before {
  margin-right: 0.4rem;
}

.icon-prs::before { content: "📊"; }
.icon-repos::before { content: "📊"; }
.icon-reactions::before { content: "👍"; }
.icon-account::before { content: "📅"; }
.icon-issues::before { content: "💬"; }
.icon-reviews::before { content: "👀"; }
.icon-mergers::before { content: "👥"; }
.icon-history::before { content: "📁"; }
.icon-profile::before { content: "📝"; }
.icon-alert::before { content: "🚨"; }
.icon-clean::before { content: "✅"; }
.icon-disabled::before { content: "➖"; }

details[open] summary {
  border-bottom: 1px solid #d0d7de;
  border-radius: 4px 4px 0 0;
}

.detail-body {
  padding: 1rem;
  font-size: 0.95rem;
  overflow-x: auto;
}

.detail-body p {
  margin-bottom: 0.75rem;
}

.detail-body ul {
  margin: 0 0 0.75rem 1.25rem;
}

.detail-body > :last-child {
  margin-bottom: 0;
}

.detail-body table {
  width: 100%;
  border-collapse: collapse;
  margin-bottom: 0.75rem;
}

.detail-body th,
.detail-body td {
  padding: 0.5rem 0.6rem;
  text-align: left;
  border-bottom: 1px solid #d8dee4;
}

.detail-body th {
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: #59636e;
  background-color: #f6f8fa;
}

.detail-body em {
  color: #59636e;
}

/* The tick and cross the "cell" template puts in a boolean table cell */
.tick-yes {
  color: #1a7f37;
}

.tick-no {
  color: #cf222e;
}

.recommendations {
  margin-left: 1.25rem;
}

.recommendations li {
  margin-bottom: 0.5rem;
}

footer {
  margin-top: 2rem;
  padding-top: 1rem;
  border-top: 1px solid #d8dee4;
  color: #59636e;
  font-size: 0.85rem;
  text-align: center;
}

@media (max-width: 768px) {
  body {
    padding: 1rem;
  }

  h1 {
    font-size: 1.5rem;
  }

  .title svg {
    width: 2.5rem;
    height: 2.5rem;
  }

  .report {
    padding: 1rem;
  }

  .metrics-table th,
  .metrics-table td {
    padding: 0.5rem;
  }
}
</style>
</head>
<body>
<header class="masthead">
{{if .HomeURL}}<a class="home" href="{{.HomeURL}}">&larr; All reports</a>
{{end}}<div class="title">{{.Logo}}<h1>@{{.Username}}</h1></div>
<p class="report-info">Contributor Report &middot; Generated {{.GeneratedAt}}</p>
</header>

<section class="report">
<p class="verdict verdict-{{.StatusClass}}">{{if eq .StatusClass "passed"}}✅{{else}}⚠️{{end}} {{.StatusText}} ({{.PassedCount}}/{{.TotalCount}} metrics passed)</p>
{{if .NewAccountNote}}<p class="note"><strong>Note:</strong> This is a new GitHub account (&lt; {{.NewAccountDays}} days old). Limited history is available for evaluation.</p>
{{end}}{{if .LimitedDataNote}}<p class="note"><strong>Note:</strong> Limited contribution data available. Results may be affected.</p>
{{end}}
<h2>Metrics</h2>
<div class="table-scroll">
<table class="metrics-table">
<thead>
<tr><th>Metric</th><th>Description</th><th>Value</th><th>Threshold</th><th>Status</th></tr>
</thead>
<tbody>
{{range .Rows}}<tr class="status-{{.StatusClass}}">
<td class="metric-name"><a href="{{.URL}}" target="_blank" rel="noopener">{{.Name}}</a></td>
<td class="metric-description">{{.Description}}</td>
<td class="metric-value">{{.Value}}</td>
<td class="metric-threshold">{{.Threshold}}</td>
<td class="metric-status">{{if eq .StatusClass "passed"}}✅{{else if eq .StatusClass "failed"}}❌{{else}}—{{end}}</td>
</tr>
{{end}}</tbody>
</table>
</div>
{{if .Details}}
<h2>Metric Details</h2>
{{range .Details}}<details{{if .Failed}} class="failed"{{end}}>
<summary class="icon-{{.Icon}}">{{.Summary}}</summary>
<div class="detail-body">
{{template "blocks" .Blocks}}</div>
</details>
{{end}}{{end}}{{if .Recommendations}}
<h2>Recommendations</h2>
<ul class="recommendations">
{{range .Recommendations}}<li>{{.}}</li>
{{end}}</ul>
{{end}}
<footer>Contributor Report evaluates based on public GitHub activity. Analysis period: {{.WindowStart}} to {{.WindowEnd}}</footer>
</section>
</body>
</html>
