package command

import (
	"strings"
)

// SplitArgs splits a command string into arguments, respecting quoted strings.
func SplitArgs(text string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range text {
		if inQuote {
			if r == quoteChar {
				inQuote = false
			} else {
				current.WriteRune(r)
			}
		} else {
			switch r {
			case '"', '\'':
				inQuote = true
				quoteChar = r
			case ' ', '\t':
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			default:
				current.WriteRune(r)
			}
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// ExpandCmd expands template variables in a command string.
func ExpandCmd(cmd string, vars map[string]string) string {
	result := cmd
	for key, value := range vars {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
		result = strings.ReplaceAll(result, "${"+key+"}", value)
	}
	return result
}
