package tw

import "time"

const twTimeLayout = "20060102T150405Z"

// ParseTime parses a Taskwarrior YYYYMMDDTHHMMSSZ timestamp. Empty input
// returns the zero time and no error so callers can distinguish "no value"
// from a parse failure.
func ParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(twTimeLayout, s)
}

func FormatTime(t time.Time) string {
	return t.UTC().Format(twTimeLayout)
}
