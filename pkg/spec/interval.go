package spec

import (
	"fmt"
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
	return time.Duration(n) * intervalUnits[m[2]], nil
}
