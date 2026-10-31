package grafana

import (
	"strconv"
	"strings"
	"sync"
)

const (
	minPercentile = 1
	maxPercentile = 99

	percentileSuffix = " %"
)

var numericReducerNames = map[string]string{
	"last": "Last", "lastNotNull": "Last *", "first": "First", "firstNotNull": "First *",
	"min": "Min", "max": "Max", "mean": "Mean", "median": "Median", "sum": "Total",
	"count": "Count", "range": "Range", "delta": "Delta", "diff": "Difference",
	"stdDev": "StdDev", "distinctCount": "Distinct Count", "changeCount": "Change Count",
	"variance": "Variance", "step": "Step", "diffperc": "Difference percent",
	"logmin": "Min (above zero)", "countAll": "Count all",
}

var nonNumberReducerNames = map[string]string{
	"allIsNull": "All nulls", "allIsZero": "All zeros",
	"allValues": "All values", "uniqueValues": "All unique values",
}

func percentile(id string) (int, bool) {
	digits, ok := strings.CutPrefix(id, "p")
	if !ok || digits == "" || digits[0] == '0' || len(digits) > 2 {
		return 0, false
	}
	if strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	return n, err == nil && n >= minPercentile && n <= maxPercentile
}

func percentileName(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(n) + suffix + percentileSuffix
}

func isReducer(id string) bool {
	if _, ok := numericReducerNames[id]; ok {
		return true
	}
	_, ok := percentile(id)
	return ok
}

func isAnyReducer(id string) bool {
	_, ok := nonNumberReducerNames[id]
	return ok || isReducer(id)
}

func reducerDisplayName(id string) (string, bool) {
	if name, ok := numericReducerNames[id]; ok {
		return name, true
	}
	if name, ok := nonNumberReducerNames[id]; ok {
		return name, true
	}
	if n, ok := percentile(id); ok {
		return percentileName(n), true
	}
	return "", false
}

var reducerIDsByDisplayName = sync.OnceValue(func() map[string]string {
	ids := map[string]string{}
	for id, name := range numericReducerNames {
		ids[name] = id
	}
	for id, name := range nonNumberReducerNames {
		ids[name] = id
	}
	for n := minPercentile; n <= maxPercentile; n++ {
		ids[percentileName(n)] = "p" + strconv.Itoa(n)
	}
	return ids
})

func reducerFromDisplayName(name string) (string, bool) {
	id, ok := reducerIDsByDisplayName()[name]
	return id, ok
}
