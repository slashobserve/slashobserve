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

const UnopenedAfter = 90 * 24 * time.Hour

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
