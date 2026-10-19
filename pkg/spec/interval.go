package spec

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"
)

var intervalExpression = regexp.MustCompile(`^([1-9][0-9]{0,4})(ms|[smhdwy])$`)

var intervalUnits = map[string]time.Duration{
	"ms": time.Millisecond,
	"s":  time.Second,
	"m":  time.Minute,
	"h":  time.Hour,
	"d":  24 * time.Hour,
	"w":  7 * 24 * time.Hour,
	"y":  365 * 24 * time.Hour,
}

func ParseInterval(s string) (time.Duration, error) {
	m := intervalExpression.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("%q is not a valid interval", s)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid interval", s)
	}
	unit := intervalUnits[m[2]]
	if int64(n) > math.MaxInt64/int64(unit) {
		return 0, fmt.Errorf("%q is too long an interval", s)
	}
	return time.Duration(n) * unit, nil
}

var formatUnits = []struct {
	suffix string
	size   time.Duration
}{
	{"y", intervalUnits["y"]},
	{"w", intervalUnits["w"]},
	{"d", intervalUnits["d"]},
	{"h", intervalUnits["h"]},
	{"m", intervalUnits["m"]},
	{"s", intervalUnits["s"]},
	{"ms", intervalUnits["ms"]},
}

func FormatInterval(d time.Duration) string {
	for _, u := range formatUnits {
		if d >= u.size && d%u.size == 0 {
			return strconv.FormatInt(int64(d/u.size), 10) + u.suffix
		}
	}
	return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
}
