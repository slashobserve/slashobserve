package tools

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func pointsFrame(service string, from time.Time, values ...float64) datasource.Frame {
	var times, nums []any
	for i, v := range values {
		times = append(times, from.Add(time.Duration(i)*time.Minute))
		nums = append(nums, v)
	}
	return datasource.Frame{Labels: map[string]string{"service_name": service}, Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime, Values: times},
		{Name: "Value", Type: datasource.FieldNumber, Values: nums},
	}}
}

func TestReadSeriesGivesEveryFinitePointOfEachSeries(t *testing.T) {
	from := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	table := datasource.Frame{Fields: []datasource.Field{{Name: "line", Type: datasource.FieldString, Values: []any{"x"}}}}
	got := pointsOf([]datasource.Frame{pointsFrame("cart", from, 1, math.NaN(), 3), table, pointsFrame("ad", from, 0.5)}, 120)
	raw, _ := json.Marshal(got)
	want := `{"from":"","to":"","series":[{"labels":{"service_name":"cart"},"points":[[1795168800,1],[1795168920,3]]},{"labels":{"service_name":"ad"},"points":[[1795168800,0.5]]}]}`
	if string(raw) != want {
		t.Errorf("series =\n%s\nwant\n%s", raw, want)
	}
}

func TestReadSeriesCutsTheSeriesThePointsAndTheBytes(t *testing.T) {
	from := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	var frames []datasource.Frame
	for i := range maxReadSeries + 5 {
		frames = append(frames, pointsFrame(fmt.Sprintf("svc%d", i), from, 1, 2))
	}
	if got := pointsOf(frames, 120); len(got.Series) != maxReadSeries || !got.Truncated {
		t.Errorf("kept %d series, truncated %t", len(got.Series), got.Truncated)
	}
	long := make([]float64, 30)
	if got := pointsOf([]datasource.Frame{pointsFrame("cart", from, long...)}, 10); len(got.Series[0].Points) != 20 || !got.Truncated {
		t.Errorf("kept %d points, truncated %t", len(got.Series[0].Points), got.Truncated)
	}
	wide := make([]float64, maxSeriesPoints)
	for i := range wide {
		wide[i] = 123456.789
	}
	frames = nil
	for i := range maxReadSeries {
		frames = append(frames, pointsFrame(fmt.Sprintf("svc%d", i), from, wide...))
	}
	got := pointsOf(frames, maxSeriesPoints)
	raw, _ := json.Marshal(got)
	if !got.Truncated || len(raw) > maxReadSeriesBytes+1024 || len(got.Series) == 0 {
		t.Errorf("kept %d series in %d bytes, truncated %t", len(got.Series), len(raw), got.Truncated)
	}
}
