package brief

import "time"

func init() {
	clock = func() time.Time { return time.Date(2026, 11, 25, 5, 29, 0, 0, time.UTC) }
}
