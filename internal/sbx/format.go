package sbx

import (
	"fmt"
	"strings"
	"time"
)

func FormatBytes(value int64) string {
	units := [...]string{"B", "KB", "MB", "GB", "TB"}
	size := float64(value)
	unit := 0
	for unit < len(units)-1 && size >= 1000 {
		size /= 1000
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", value)
	}
	return fmt.Sprintf("%.1f %s", size, units[unit])
}

func FormatRate(value int64) string {
	return FormatBytes(value) + "/s"
}

func FormatAgo(value, now time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return FormatShortDuration(now.Sub(value)) + " ago"
}

func FormatShortDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	switch {
	case duration < time.Minute:
		return fmt.Sprintf("%ds", duration/time.Second)
	case duration < time.Hour:
		return fmt.Sprintf("%dm", duration/time.Minute)
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh", duration/time.Hour)
	default:
		return fmt.Sprintf("%dd", duration/(24*time.Hour))
	}
}

func FormatDuration(duration time.Duration) string {
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int64(duration/time.Second))
	}

	days := duration / (24 * time.Hour)
	duration %= 24 * time.Hour
	hours := duration / time.Hour
	duration %= time.Hour
	minutes := duration / time.Minute
	parts := make([]string, 0, 3)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	return strings.Join(parts, " ")
}
