package utils

import (
	"fmt"
	"time"
)

const (
	dateLayout     = "2006-01-02"
	dateTimeLayout = "2006-01-02T15:04:05"
)

// SerializeDate formats a time as an RFC3339 calendar date (YYYY-MM-DD) in UTC.
func SerializeDate(t time.Time) string {
	return t.UTC().Format(dateLayout)
}

// DeserializeDate parses a calendar date string into a time.Time.
func DeserializeDate(value string) (time.Time, error) {
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("utils: failed to parse date %q: %w", value, err)
	}
	return t, nil
}

// SerializeDateTime formats a time as the Box date-time string, truncated to
// seconds and suffixed with the UTC offset, mirroring dateTimeToString.
func SerializeDateTime(t time.Time) string {
	return t.UTC().Format(dateTimeLayout) + "+00:00"
}

// DeserializeDateTime parses a Box date-time string into a time.Time. It accepts
// the common RFC3339 representations Box returns.
func DeserializeDateTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, dateTimeLayout + "-07:00", dateTimeLayout} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("utils: failed to parse date-time %q", value)
}

// EpochSecondsToDateTime converts Unix seconds to a UTC time.
func EpochSecondsToDateTime(seconds int64) time.Time {
	return time.Unix(seconds, 0).UTC()
}

// DateTimeToEpochSeconds converts a time to Unix seconds.
func DateTimeToEpochSeconds(t time.Time) int64 {
	return t.Unix()
}
