package view

import (
	"strings"
)

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
