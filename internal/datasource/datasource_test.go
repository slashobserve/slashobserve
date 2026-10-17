package datasource_test

import (
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestTimeRangeValidate(t *testing.T) {
	at := func(year int) time.Time { return time.Date(year, 6, 1, 0, 0, 0, 0, time.UTC) }
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		r       datasource.TimeRange
		invalid bool
	}{
		{name: "valid", r: datasource.TimeRange{From: now.Add(-time.Hour), To: now}},
		{name: "pre-1970", r: datasource.TimeRange{From: at(1960), To: at(1961)}},
		{name: "year 1 to year 9999", r: datasource.TimeRange{From: at(1), To: at(9999)}},
		{name: "zero from", r: datasource.TimeRange{To: now}, invalid: true},
		{name: "zero to", r: datasource.TimeRange{From: now}, invalid: true},
		{name: "both zero", invalid: true},
		{name: "year 0 from", r: datasource.TimeRange{From: at(0), To: now}, invalid: true},
		{name: "year 10000 to", r: datasource.TimeRange{From: now, To: at(10000)}, invalid: true},
		{name: "year 10000 both", r: datasource.TimeRange{From: at(10000), To: at(10001)}, invalid: true},
		{name: "from equals to", r: datasource.TimeRange{From: now, To: now}, invalid: true},
		{name: "from after to", r: datasource.TimeRange{From: now, To: now.Add(-time.Second)}, invalid: true},
		{name: "offset zone in year 0", r: datasource.TimeRange{From: time.Date(1, 1, 1, 0, 0, 0, 0, time.FixedZone("x", 3600)), To: now}, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.r.Validate()
			switch {
			case tt.invalid && !errors.Is(err, datasource.ErrInvalidQuery):
				t.Fatalf("want ErrInvalidQuery, got %v", err)
			case !tt.invalid && err != nil:
				t.Fatalf("want valid, got %v", err)
			}
		})
	}
}
