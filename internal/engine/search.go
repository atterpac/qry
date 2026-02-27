package engine

import "strings"

// ParseSearchInput splits user input into SearchFilter entries.
// Supports "column:value" syntax for targeted search and plain terms for all-column search.
// Invalid column names fall back to all-column search.
func ParseSearchInput(input string, validCols []string) []SearchFilter {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	colSet := make(map[string]bool, len(validCols))
	for _, c := range validCols {
		colSet[strings.ToLower(c)] = true
	}

	parts := strings.Fields(input)
	var filters []SearchFilter

	for _, part := range parts {
		if idx := strings.IndexByte(part, ':'); idx > 0 && idx < len(part)-1 {
			col := part[:idx]
			val := part[idx+1:]
			if colSet[strings.ToLower(col)] {
				filters = append(filters, SearchFilter{Column: col, Value: val})
				continue
			}
		}
		filters = append(filters, SearchFilter{Value: part})
	}

	return filters
}
