package cli

import (
	"fmt"
	"strings"
	"time"
)

func formatBytes(value int64) string {
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

func formatRate(value int64) string {
	return formatBytes(value) + "/s"
}

func formatDuration(duration time.Duration) string {
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
