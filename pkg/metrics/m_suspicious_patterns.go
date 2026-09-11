// Suspicious Patterns: the aggregate verdict of the spam heuristics.
package metrics

import (
	"fmt"
	"strings"

	"github.com/kostis-codefresh/github-user-slop-detector/pkg/analysis"
)

// SuspiciousPatterns reports the spam heuristics that fired. Critical findings
// fail the metric; warnings are surfaced but do not.
type SuspiciousPatterns struct{}

func (m *SuspiciousPatterns) Metadata(_ Config) Metadata {
	return Metadata{
		Key:         "suspiciousPatterns",
		Name:        "Suspicious Patterns",
		DocSlug:     "suspicious-patterns",
		Description: "Spam-like activity detection",
	}
}

func (m *SuspiciousPatterns) Evaluate(c *analysis.Contributor, cfg Config) Result {
	if !cfg.SpamDetection {
		return Result{}
	}

	critical, warnings := splitBySeverity(detectAll(c, cfg))

	result := Result{Active: true, Passed: true, Value: "0", Threshold: "N/A", DataPoints: 1}
	switch {
	case len(critical) > 0:
		result.Passed = false
		result.Value = count(len(critical))
		result.Recommendation = "CRITICAL: Suspicious activity patterns detected. This account " +
			"exhibits characteristics commonly associated with spam or automated contributions."
	case len(warnings) > 0:
		result.Value = count(len(warnings))
	}
	return result
}

func (m *SuspiciousPatterns) DetailGroup() string { return "suspiciousPatterns" }

func (m *SuspiciousPatterns) Detail(c *analysis.Contributor, cfg Config) Detail {
	if !cfg.SpamDetection {
		return Detail{
			Summary: "➖ Spam detection disabled",
			Blocks:  []Block{text("Run without --spam-detection=false to enable.")},
		}
	}

	detections := detectAll(c, cfg)
	if len(detections) == 0 {
		return Detail{
			Summary: "✅ No suspicious patterns detected",
			Blocks:  []Block{text("Activity appears normal.")},
		}
	}

	critical, warnings := splitBySeverity(detections)
	var parts []string
	if len(critical) > 0 {
		parts = append(parts, fmt.Sprintf("%d critical", len(critical)))
	}
	if len(warnings) > 0 {
		parts = append(parts, fmt.Sprintf("%d warning", len(warnings)))
	}

	var blocks []Block
	for _, detection := range detections {
		heading := fmt.Sprintf("%s %s: %s", severityIcon(detection.Severity), detection.Severity, detection.Type)
		blocks = append(blocks, para(Line{Label: heading}, Line{Text: detection.Description}))

		items := make([]string, 0, len(detection.Evidence))
		for _, evidence := range detection.Evidence {
			items = append(items, evidence.Key+": "+evidence.Value)
		}
		blocks = append(blocks, list(items...))
	}

	return Detail{
		Summary: fmt.Sprintf("🚨 %d pattern%s detected (%s)",
			len(detections), plural(len(detections)), strings.Join(parts, ", ")),
		Blocks: blocks,
	}
}

func splitBySeverity(detections []Detection) (critical, warnings []Detection) {
	for _, detection := range detections {
		if detection.Severity == SeverityCritical {
			critical = append(critical, detection)
			continue
		}
		warnings = append(warnings, detection)
	}
	return critical, warnings
}

func severityIcon(s Severity) string {
	if s == SeverityCritical {
		return "🔴"
	}
	return "🟡"
}
