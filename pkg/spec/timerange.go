package spec

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var timeExpression = regexp.MustCompile(`^now(?:([+-])([0-9]+)([smhdwMy]))?(?:/(fy|fQ|[smhdwMy]))?$`)

var absoluteUnitSeconds = map[string]int64{
	"s": 1,
	"m": 60,
	"h": 3600,
}

const (
	minResolvedYear = 1
	maxResolvedYear = 9999
	maxEpochDigits  = 16
)

func ResolveTime(expr string, now time.Time, roundUp bool, opts TimeOptions) (time.Time, error) {
	t, ok := resolveTime(expr, opts.localNow(now), roundUp, opts)
	if !ok {
		return time.Time{}, fmt.Errorf("%q is not a valid time", expr)
	}
	return t, nil
}

func ResolveTimeRange(tr TimeRange, now time.Time, opts TimeOptions) (from, to time.Time, err error) {
	from, err = ResolveTime(tr.From, now, false, opts)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("time.from %w", err)
	}
	to, err = ResolveTime(tr.To, now, true, opts)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("time.to %w", err)
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, errors.New("time.from must be before time.to")
	}
	return from, to, nil
}

func resolveTime(expr string, now time.Time, roundUp bool, opts TimeOptions) (time.Time, bool) {
	var t time.Time
	if m := timeExpression.FindStringSubmatch(expr); m != nil {
		t = resolveRelative(m, now, roundUp, opts)
	} else if isDigits(expr) {
		if len(expr) > maxEpochDigits {
			return time.Time{}, false
		}
		ms, err := strconv.ParseInt(expr, 10, 64)
		if err != nil {
			return time.Time{}, false
		}
		t = time.UnixMilli(ms)
	} else {
		parsed, err := time.Parse(time.RFC3339, expr)
		if err != nil {
			return time.Time{}, false
		}
		t = parsed
	}
	t = t.UTC()
	if t.IsZero() || t.Year() < minResolvedYear || t.Year() > maxResolvedYear {
		return time.Time{}, false
	}
	return t, true
}

func resolveRelative(m []string, now time.Time, roundUp bool, opts TimeOptions) time.Time {
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
	start := opts.startOf(t, m[4])
	if !roundUp {
		return start
	}
	return advance(start, 1, m[4]).Add(-time.Millisecond)
}

func offset(t time.Time, n int64, unit string) time.Time {
	if seconds, ok := absoluteUnitSeconds[unit]; ok {
		return time.Unix(t.Unix()+n*seconds, int64(t.Nanosecond())).In(t.Location())
	}
	return advance(t, int(n), unit)
}

func advance(t time.Time, n int, unit string) time.Time {
	if months, ok := monthsPerUnit[unit]; ok {
		return addMonthsClamped(t, int(months)*n)
	}
	switch unit {
	case "w":
		return t.AddDate(0, 0, 7*n)
	case "d":
		return t.AddDate(0, 0, n)
	default:
		return t.Add(time.Duration(n) * time.Duration(absoluteUnitSeconds[unit]) * time.Second)
	}
}

func addMonthsClamped(t time.Time, months int) time.Time {
	y, mo, d := t.Date()
	loc := t.Location()
	first := time.Date(y, mo+time.Month(months), 1, 0, 0, 0, 0, loc)
	lastDay := first.AddDate(0, 1, -1).Day()
	hh, mm, ss := t.Clock()
	return time.Date(first.Year(), first.Month(), min(d, lastDay), hh, mm, ss, t.Nanosecond(), loc)
}

func (o TimeOptions) startOf(t time.Time, unit string) time.Time {
	y, mo, d := t.Date()
	loc := t.Location()
	switch unit {
	case "s":
		return t.Truncate(time.Second)
	case "m":
		return t.Add(-time.Duration(t.Second())*time.Second - time.Duration(t.Nanosecond()))
	case "h":
		return t.Add(-time.Duration(t.Minute())*time.Minute - time.Duration(t.Second())*time.Second - time.Duration(t.Nanosecond()))
	case "d":
		return time.Date(y, mo, d, 0, 0, 0, 0, loc)
	case "w":
		sinceStart := (int(t.Weekday()) - int(o.WeekStart.weekday()) + 7) % 7
		return time.Date(y, mo, d-sinceStart, 0, 0, 0, 0, loc)
	case "M":
		return time.Date(y, mo, 1, 0, 0, 0, 0, loc)
	case "fQ":
		intoQuarter := o.monthsIntoFiscalYear(mo) % 3
		return time.Date(y, mo-time.Month(intoQuarter), 1, 0, 0, 0, 0, loc)
	case "fy":
		return time.Date(y, mo-time.Month(o.monthsIntoFiscalYear(mo)), 1, 0, 0, 0, 0, loc)
	default:
		return time.Date(y, time.January, 1, 0, 0, 0, 0, loc)
	}
}

func (o TimeOptions) monthsIntoFiscalYear(month time.Month) int {
	return (int(month) - 1 - o.FiscalYearStartMonth + 12) % 12
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
