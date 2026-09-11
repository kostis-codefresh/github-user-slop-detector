package analysis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWindow(t *testing.T) {
	testCases := []struct {
		name     string
		now      string
		months   int
		expected string
	}{
		{name: "Happy Path scenario", now: "2026-09-11T13:45:00Z", months: 12, expected: "2025-09-11"},
		{name: "Crosses into the previous year", now: "2026-03-23T00:00:00Z", months: 12, expected: "2025-03-23"},
		{name: "Partial window stays in the same year", now: "2026-09-11T00:00:00Z", months: 3, expected: "2026-06-11"},
		// 31 May minus three months is 31 February, which must clamp to the
		// last day of the target month rather than spilling into March.
		{name: "Clamps to the target month's last day", now: "2026-05-31T00:00:00Z", months: 3, expected: "2026-02-28"},
		{name: "Clamps into a leap February", now: "2024-03-31T00:00:00Z", months: 1, expected: "2024-02-29"},
		{name: "Zero window is today", now: "2026-09-11T18:00:00Z", months: 0, expected: "2026-09-11"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.now)
			assert.NoError(t, err, "test case has an unparseable date")

			got := Window(now, tc.months)

			assert.Equal(t, tc.expected, got.Format("2006-01-02"), "window start mismatch")
			assert.Equal(t, time.UTC, got.Location(), "window must be in UTC")
			assert.Equal(t, 0, got.Hour(), "window must start at midnight")
		})
	}
}
