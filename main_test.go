package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFallbackWindows(t *testing.T) {
	testCases := []struct {
		name     string
		months   int
		expected []int
	}{
		{name: "Default window falls back through every step", months: 12, expected: []int{12, 6, 3, 1}},
		{name: "A window on a step skips that step", months: 6, expected: []int{6, 3, 1}},
		{name: "A window between steps keeps the smaller ones", months: 4, expected: []int{4, 3, 1}},
		{name: "A short window falls back to one month", months: 2, expected: []int{2, 1}},
		{name: "One month has nothing to fall back to", months: 1, expected: []int{1}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, fallbackWindows(tc.months), "fallback sequence mismatch")
		})
	}
}
