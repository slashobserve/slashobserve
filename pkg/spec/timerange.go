package spec

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var timeExpression = regexp.MustCompile(`^now(?:([+-])([0-9]+)([smhdwMy]))?(?:/([smhdwMy]))?$`)

var fixedUnits = map[string]time.Duration{
	"s": time.Second,
	"m": time.Minute,
	"h": time.Hour,
	"d": 24 * time.Hour,
	"w": 7 * 24 * time.Hour,
}

const (
	minResolvedYear = 1
	maxResolvedYear = 9999
	maxEpochDigits  = 16
)

func ResolveTime(expr string, now time.Time, roundUp bool) (time.Time, error) {
	t, ok := resolveTime(expr, now.UTC(), roundUp)
	if !ok {
		return time.Time{}, fmt.Errorf("%q is not a valid time", expr)
	}
	return t, nil
}

func ResolveTimeRange(tr TimeRange, now time.Time) (from, to time.Time, err error) {
	from, err = ResolveTime(tr.From, now, false)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("time.from %w", err)
	}
	to, err = ResolveTime(tr.To, now, true)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("time.to %w", err)
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, errors.New("time.from must be before time.to")
	}
	return from, to, nil
}

func resolveTime(expr string, now time.Time, roundUp bool) (time.Time, bool) {
	var t time.Time
	if m := timeExpression.FindStringSubmatch(expr); m != nil {
		t = resolveRelative(m, now, roundUp)
	} else if isDigits(expr) {
		if len(expr) > maxEpochDigits {
			return time.Time{}, false
		}
		ms, err := strconv.ParseInt(expr, 10, 64)
		if err != nil {
			return time.Time{}, false
		}
		t = time.UnixMilli(ms).UTC()
	} else {
		parsed, err := time.Parse(time.RFC3339, expr)
		if err != nil {
			return time.Time{}, false
		}
		t = parsed.UTC()
	}
	if t.IsZero() || t.Year() < minResolvedYear || t.Year() > maxResolvedYear {
		return time.Time{}, false
	}
	return t, true
}

func resolveRelative(m []string, now time.Time, roundUp bool) time.Time {
	t := now
	if m[2] != "" {
		n, err := strconv.ParseInt(m[2], 10, 32)
		if err != nil {
			return time.Time{}
		}
		if m[1] == "-" {
			n = -n
		}
		t = offset(t, n, m[3])
		if t.IsZero() {
			return t
		}
	}
	if m[4] == "" {
		return t
	}
	start := startOf(t, m[4])
	if !roundUp {
		return start
	}
	return advance(start, 1, m[4]).Add(-time.Millisecond)
}

func offset(t time.Time, n int64, unit string) time.Time {
	if d, ok := fixedUnits[unit]; ok {
		seconds := n * int64(d/time.Second)
		return time.Unix(t.Unix()+seconds, int64(t.Nanosecond())).UTC()
	}
	return advance(t, int(n), unit)
}

func advance(t time.Time, n int, unit string) time.Time {
	switch unit {
	case "M":
		return addMonthsClamped(t, n)
	case "y":
		return addMonthsClamped(t, 12*n)
	case "w":
		return t.AddDate(0, 0, 7*n)
	case "d":
		return t.AddDate(0, 0, n)
	default:
		return t.Add(time.Duration(n) * fixedUnits[unit])
	}
}

func addMonthsClamped(t time.Time, months int) time.Time {
	y, mo, d := t.Date()
	first := time.Date(y, mo+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	lastDay := first.AddDate(0, 1, -1).Day()
	hh, mm, ss := t.Clock()
	return time.Date(first.Year(), first.Month(), min(d, lastDay), hh, mm, ss, t.Nanosecond(), time.UTC)
}

func startOf(t time.Time, unit string) time.Time {
	y, mo, d := t.Date()
	switch unit {
	case "s":
		return t.Truncate(time.Second)
	case "m":
		return t.Truncate(time.Minute)
	case "h":
		return t.Truncate(time.Hour)
	case "d":
		return time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)
	case "w":
		sinceMonday := (int(t.Weekday()) + 6) % 7
		return time.Date(y, mo, d-sinceMonday, 0, 0, 0, 0, time.UTC)
	case "M":
		return time.Date(y, mo, 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
