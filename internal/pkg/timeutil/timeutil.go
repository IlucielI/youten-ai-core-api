package timeutil

import (
	"fmt"
)

// FormatTimestamp converts seconds into a human-readable [MM:SS] or [HH:MM:SS] string.
// Negative seconds are safely clamped to 00:00.
func FormatTimestamp(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	totalSec := int(seconds)
	hours := totalSec / 3600
	minutes := (totalSec % 3600) / 60
	secs := totalSec % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}
