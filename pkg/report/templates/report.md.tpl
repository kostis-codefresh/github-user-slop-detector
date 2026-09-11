## {{.StatusEmoji}} Contributor Report

**User:** @{{.Username}}
**Status:** {{.StatusText}} ({{.PassedCount}}/{{.TotalCount}} metrics passed)
{{if .NewAccountNote}}
> **Note:** This is a new GitHub account (< {{.NewAccountDays}} days old). Limited history is available for evaluation.
{{end}}{{if .LimitedDataNote}}
> **Note:** Limited contribution data available. Results may be affected.
{{end}}
| Metric | Description | Value | Threshold | Status |
|--------|-------------|-------|-----------|--------|
{{range .Rows}}| [{{.Name}}]({{.URL}}) | {{.Description}} | {{.Value}} | {{.Threshold}} | {{.Status}} |
{{end}}{{if .Details}}
### Metric Details
{{range .Details}}
<details>
<summary>{{.Summary}}</summary>

{{.Body}}

</details>
{{end}}{{end}}{{if .Recommendations}}
### Recommendations

{{range .Recommendations}}- {{.}}
{{end}}{{end}}
---
<sub>Contributor Report evaluates based on public GitHub activity. Analysis period: {{.WindowStart}} to {{.WindowEnd}}</sub>
