package timeutil_test

import (
	"testing"

	"code-base-golang/internal/pkg/timeutil"
)

func TestFormatTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		seconds  float64
		expected string
	}{
		{name: "negative clamped to zero", seconds: -5, expected: "00:00"},
		{name: "zero seconds", seconds: 0, expected: "00:00"},
		{name: "fractional seconds ignored", seconds: 12.8, expected: "00:12"},
		{name: "59 seconds", seconds: 59, expected: "00:59"},
		{name: "exactly 1 minute", seconds: 60, expected: "01:00"},
		{name: "minutes and seconds", seconds: 125, expected: "02:05"},
		{name: "59 minutes 59 seconds", seconds: 3599, expected: "59:59"},
		{name: "exactly 1 hour", seconds: 3600, expected: "01:00:00"},
		{name: "hours minutes seconds", seconds: 3661, expected: "01:01:01"},
		{name: "multiple hours", seconds: 7325, expected: "02:02:05"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeutil.FormatTimestamp(tt.seconds)
			if got != tt.expected {
				t.Errorf("FormatTimestamp(%v) = %q, want %q", tt.seconds, got, tt.expected)
			}
		})
	}
}
