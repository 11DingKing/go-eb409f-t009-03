package api

import "time"

// parseTime parses an RFC3339 timestamp, returning a zero time on empty input.
func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}
