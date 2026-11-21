package anchor

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	shortForms   = "row:<label>=<value>,..., time:<instant>, range:<instant>..<instant>, series:<label>=<value>,... or pos:<x>,<y>"
	clockLayout  = "15:04Z07:00"
	minuteLayout = "2006-01-02T15:04Z07:00"
	day          = 24 * time.Hour
)

var centre = Pos{X: 0.5, Y: 0.5}

type instant struct {
	at        time.Time
	clockOnly bool
}

func parseShort(text string, now time.Time) (Anchor, error) {
	form, rest, found := strings.Cut(text, ":")
	if !found {
		return Anchor{}, fmt.Errorf("unrecognised anchor %q; use the anchor JSON object, %s", text, shortForms)
	}
	a := Anchor{Pos: centre}
	var err error
	switch form {
	case "row":
		a.Kind = Row
		a.Row, err = parseLabels(rest)
	case "series":
		a.Kind = Series
		a.Series, err = parseLabels(rest)
	case "time":
		a.Kind = Time
		a.From, err = parseTime(rest, now)
	case "range":
		a.Kind = Range
		a.From, a.To, err = parseRange(rest, now)
	case "pos":
		a.Kind = Position
		a.Pos, err = parsePos(rest)
	default:
		return Anchor{}, fmt.Errorf("unrecognised anchor %q; use the anchor JSON object, %s", text, shortForms)
	}
	if err != nil {
		return Anchor{}, fmt.Errorf("%s: %w", form, err)
	}
	if err := a.Validate(); err != nil {
		return Anchor{}, err
	}
	return a, nil
}

func parseLabels(text string) (map[string]string, error) {
	labels := map[string]string{}
	for pair := range strings.SplitSeq(text, ",") {
		key, value, found := strings.Cut(pair, "=")
		if !found || key == "" {
			return nil, fmt.Errorf("%q is not label=value", pair)
		}
		if _, dup := labels[key]; dup {
			return nil, fmt.Errorf("label %q is given twice", key)
		}
		labels[key] = value
	}
	return labels, nil
}

func parsePos(text string) (Pos, error) {
	x, y, found := strings.Cut(text, ",")
	if !found {
		return Pos{}, errors.New("want <x>,<y>, each a fraction of the panel from 0 to 1")
	}
	px, errX := strconv.ParseFloat(x, 64)
	py, errY := strconv.ParseFloat(y, 64)
	if errX != nil || errY != nil {
		return Pos{}, errors.New("want <x>,<y>, each a fraction of the panel from 0 to 1")
	}
	return Pos{X: px, Y: py}, nil
}

func parseInstant(text string) (instant, error) {
	for _, layout := range []string{time.RFC3339, minuteLayout} {
		if t, err := time.Parse(layout, text); err == nil {
			return instant{at: t}, nil
		}
	}
	if t, err := time.Parse(clockLayout, text); err == nil {
		return instant{at: t.UTC(), clockOnly: true}, nil
	}
	return instant{}, fmt.Errorf("%q is not a time such as 2026-11-20T23:56Z or a time of day such as 23:56Z", text)
}

func parseTime(text string, now time.Time) (*time.Time, error) {
	at, err := parseInstant(text)
	if err != nil {
		return nil, err
	}
	resolved := at.at
	if at.clockOnly {
		resolved = latestAtOrBefore(now, at.at)
	}
	return &resolved, nil
}

func parseRange(text string, now time.Time) (*time.Time, *time.Time, error) {
	first, second, found := strings.Cut(text, "..")
	if !found {
		return nil, nil, errors.New("want <instant>..<instant>")
	}
	start, err := parseInstant(first)
	if err != nil {
		return nil, nil, err
	}
	end, err := parseInstant(second)
	if err != nil {
		return nil, nil, err
	}
	from, to := start.at, end.at
	if start.clockOnly {
		reference := now
		if !end.clockOnly {
			reference = end.at
		}
		from = latestAtOrBefore(reference, start.at)
	}
	if end.clockOnly {
		to = earliestAtOrAfter(from, end.at)
	}
	return &from, &to, nil
}

func clockOf(t time.Time) time.Duration {
	t = t.UTC()
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute + time.Duration(t.Second())*time.Second
}

func latestAtOrBefore(reference, clock time.Time) time.Time {
	reference = reference.UTC()
	midnight := reference.Truncate(day)
	candidate := midnight.Add(clockOf(clock))
	if candidate.After(reference) {
		candidate = candidate.Add(-day)
	}
	return candidate
}

func earliestAtOrAfter(reference, clock time.Time) time.Time {
	reference = reference.UTC()
	midnight := reference.Truncate(day)
	candidate := midnight.Add(clockOf(clock))
	if candidate.Before(reference) {
		candidate = candidate.Add(day)
	}
	return candidate
}
