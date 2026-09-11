<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Contributor Report - @{{.Username}}</title>
<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  line-height: 1.6;
  color: #333;
  background-color: #f8f9fa;
  padding: 2rem;
}

h1 {
  color: #2c3e50;
  font-size: 2.5rem;
  font-weight: 600;
  margin-bottom: 0.5rem;
  text-align: center;
}

.report-info {
  text-align: center;
  color: #6c757d;
  font-size: 1rem;
  margin-bottom: 2rem;
}

h2 {
  color: #34495e;
  font-size: 1.8rem;
  font-weight: 500;
  margin: 2.5rem 0 1.5rem 0;
  padding-bottom: 0.5rem;
  border-bottom: 2px solid #e9ecef;
}

.report {
  max-width: 1200px;
  margin: 0 auto;
  background: white;
  padding: 2rem;
  border-radius: 8px;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
}

/* Headline verdict, tinted the same way as the table rows it summarises */
.verdict {
  font-size: 1.25rem;
  font-weight: 600;
  padding: 1rem 1.25rem;
  border-radius: 6px;
  border-left: 4px solid #6c757d;
}

.verdict-passed {
  background-color: #d4edda;
  border-left-color: #28a745;
  color: #155724;
}

.verdict-needs-review {
  background-color: #fff3cd;
  border-left-color: #ffc107;
  color: #856404;
}

.note {
  margin-top: 1rem;
  padding: 0.75rem 1rem;
  background-color: #f1f3f4;
  border-left: 4px solid #6c757d;
  border-radius: 4px;
  color: #495057;
  font-size: 0.95rem;
}

/* Metric table */
.metrics-table {
  width: 100%;
  border-collapse: collapse;
  background: white;
  border-radius: 6px;
  overflow: hidden;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.metrics-table thead {
  background: #495057;
  color: white;
}

.metrics-table th {
  padding: 1rem;
  text-align: left;
  font-weight: 600;
  font-size: 0.9rem;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.metrics-table td {
  padding: 1rem;
  border-bottom: 1px solid #e9ecef;
  vertical-align: top;
}

.metrics-table tbody tr:hover {
  background-color: #f8f9fa;
}

.metrics-table tbody tr:last-child td {
  border-bottom: none;
}

.metric-name a {
  color: #2c3e50;
  font-weight: 600;
  text-decoration: none;
  border-bottom: 1px dotted currentColor;
}

.metric-name a:hover,
.metric-name a:focus {
  border-bottom-style: solid;
}

.metric-description {
  font-size: 0.95rem;
  color: #2c3e50;
}

.metric-value,
.metric-threshold {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  font-size: 0.9rem;
  white-space: nowrap;
}

.metric-value {
  font-weight: 700;
}

.metric-threshold {
  color: #6c757d;
}

.metric-status {
  text-align: center;
}

/* One table, with the verdict carried by a left border per row */
.metrics-table .status-passed {
  border-left: 4px solid #28a745;
}

.metrics-table .status-failed {
  border-left: 4px solid #dc3545;
}

.metrics-table .status-failed .metric-value {
  color: #721c24;
}

/* An inactive metric was never evaluated, so it is dimmed rather than judged */
.metrics-table .status-inactive {
  border-left: 4px solid #6c757d;
  opacity: 0.7;
}

.metrics-table .status-inactive .metric-description {
  color: #6c757d;
}

/* Collapsible metric details */
details {
  margin-bottom: 1rem;
  background: white;
  border: 1px solid #e9ecef;
  border-left: 4px solid #6c757d;
  border-radius: 6px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

details.failed {
  border-left-color: #dc3545;
}

summary {
  padding: 0.85rem 1rem;
  font-weight: 600;
  color: #2c3e50;
  cursor: pointer;
  background-color: #f8f9fa;
  border-radius: 4px;
}

summary:hover {
  background-color: #f1f3f4;
}

details[open] summary {
  border-bottom: 1px solid #e9ecef;
  border-radius: 4px 4px 0 0;
}

.detail-body {
  padding: 1rem;
  font-size: 0.95rem;
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
  border-bottom: 1px solid #e9ecef;
}

.detail-body th {
  font-size: 0.85rem;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: #495057;
  background-color: #f8f9fa;
}

.detail-body em {
  color: #6c757d;
}

.recommendations {
  margin-left: 1.25rem;
}

.recommendations li {
  margin-bottom: 0.5rem;
  color: #2c3e50;
}

footer {
  margin-top: 2.5rem;
  padding-top: 1rem;
  border-top: 1px solid #e9ecef;
  color: #6c757d;
  font-size: 0.85rem;
  text-align: center;
}

@media (max-width: 768px) {
  body {
    padding: 1rem;
  }

  h1 {
    font-size: 2rem;
  }

  .report {
    padding: 1rem;
  }

  .metrics-table th,
  .metrics-table td {
    padding: 0.75rem 0.5rem;
  }
}

@media (max-width: 480px) {
  .metrics-table {
    font-size: 0.85rem;
  }

  .metrics-table th,
  .metrics-table td {
    padding: 0.5rem 0.4rem;
  }

  h2 {
    font-size: 1.5rem;
  }
}
</style>
</head>
<body>
<h1>{{.StatusEmoji}} Contributor Report</h1>
<p class="report-info">@{{.Username}}</p>

<section class="report">
<p class="verdict verdict-{{.StatusClass}}">{{.StatusText}} ({{.PassedCount}}/{{.TotalCount}} metrics passed)</p>
{{if .NewAccountNote}}<p class="note"><strong>Note:</strong> This is a new GitHub account (&lt; {{.NewAccountDays}} days old). Limited history is available for evaluation.</p>
{{end}}{{if .LimitedDataNote}}<p class="note"><strong>Note:</strong> Limited contribution data available. Results may be affected.</p>
{{end}}
<h2>Metrics</h2>
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
<td class="metric-status">{{.Status}}</td>
</tr>
{{end}}</tbody>
</table>
{{if .Details}}
<h2>Metric Details</h2>
{{range .Details}}<details{{if .Failed}} class="failed"{{end}}>
<summary>{{.Summary}}</summary>
<div class="detail-body">
{{.Body}}</div>
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
