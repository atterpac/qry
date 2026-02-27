package view

import (
	"fmt"
	"strings"
)

// getString safely extracts a string from a map[string]any.
func getString(values map[string]any, key string) string {
	if v, ok := values[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// formatBytes formats a byte count into a human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// redactDSN masks the password portion of a DSN for display.
func redactDSN(dsn string) string {
	// Handle postgres://user:pass@host format
	if idx := strings.Index(dsn, "://"); idx >= 0 {
		prefix := dsn[:idx+3]
		rest := dsn[idx+3:]
		if atIdx := strings.Index(rest, "@"); atIdx >= 0 {
			userInfo := rest[:atIdx]
			hostPart := rest[atIdx:]
			if colonIdx := strings.Index(userInfo, ":"); colonIdx >= 0 {
				return prefix + userInfo[:colonIdx+1] + "***" + hostPart
			}
		}
	}
	return dsn
}

// truncate shortens a string to maxLen with ellipsis.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
