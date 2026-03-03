package present

import "strings"

// RemoveWhitespace returns "" if s contains only whitespace, otherwise returns s unchanged.
func RemoveWhitespace(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return s
}
