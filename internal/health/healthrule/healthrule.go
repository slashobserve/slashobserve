package healthrule

import (
	"slices"
	"time"
)

type Kind string

const (
	KindNoData         Kind = "no_data"
	KindQueryFailed    Kind = "query_failed"
	KindVariableFailed Kind = "variable_failed"
	KindDuplicateTitle Kind = "duplicate_title"
	KindUnopened       Kind = "unopened"
)

const AnyProblem = "any"

const (
	CheckFrom   = "now-7d"
	CheckWindow = "the last 7 days"
)

const UnopenedAfter = 90 * 24 * time.Hour

const (
	ViewWindowDays    = 30
	ViewRetentionDays = 90
)

var Kinds = []Kind{KindNoData, KindQueryFailed, KindVariableFailed, KindDuplicateTitle, KindUnopened}

func ValidKind(raw string) (Kind, bool) {
	k := Kind(raw)
	return k, slices.Contains(Kinds, k)
}

func ValidFilter(raw string) (string, bool) {
	if raw == AnyProblem {
		return raw, true
	}
	k, ok := ValidKind(raw)
	return string(k), ok
}

func UnopenedBefore(now time.Time) time.Time {
	return now.Add(-UnopenedAfter)
}

func ViewsSince(now time.Time) time.Time {
	return utcDay(now).AddDate(0, 0, -(ViewWindowDays - 1))
}

func ViewsKeptSince(now time.Time) time.Time {
	return utcDay(now).AddDate(0, 0, -ViewRetentionDays)
}

func utcDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
