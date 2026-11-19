package queryproxy

import (
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func routeFrame(route string, values ...any) datasource.Frame {
	times := make([]any, len(values))
	for i := range values {
		times[i] = time.Unix(int64(i), 0)
	}
	return datasource.Frame{
		Labels: map[string]string{"route": route},
		Fields: []datasource.Field{
			{Name: "time", Type: datasource.FieldTime, Values: times},
			{Name: "value", Type: datasource.FieldNumber, Values: values},
		},
	}
}

func routeNames(frames []datasource.Frame) []string {
	var out []string
	for _, f := range frames {
		out = append(out, f.Labels["route"])
	}
	return out
}

func TestCapSeriesKeepsTheHighestPeaksInTheirOriginalOrder(t *testing.T) {
	frames := []datasource.Frame{
		routeFrame("/a", 1.0, 2.0),
		routeFrame("/b", 9.0, nil),
		routeFrame("/c", nil, nil),
		routeFrame("/d", 5.0, 4.0),
		routeFrame("/e", 3.0, 7.0),
	}
	kept, shown, total := CapSeries(frames, 3)
	if got := routeNames(kept); len(got) != 3 || got[0] != "/b" || got[1] != "/d" || got[2] != "/e" {
		t.Fatalf("kept = %v", got)
	}
	if shown != 3 || total != 5 {
		t.Fatalf("shown/total = %d/%d", shown, total)
	}
	if len(frames) != 5 {
		t.Fatal("the input slice was changed")
	}
}

func TestCapSeriesLeavesASmallResultAlone(t *testing.T) {
	frames := []datasource.Frame{routeFrame("/a", 1.0), routeFrame("/b", 2.0)}
	kept, shown, total := CapSeries(frames, 2)
	if len(kept) != 2 || shown != 2 || total != 2 {
		t.Fatalf("kept/shown/total = %d/%d/%d", len(kept), shown, total)
	}
}

func TestCapSeriesCountsEachValueFieldOfAWideFrame(t *testing.T) {
	wide := datasource.Frame{Fields: []datasource.Field{
		{Name: "time", Type: datasource.FieldTime, Values: []any{time.Unix(0, 0)}},
		{Name: "low", Type: datasource.FieldNumber, Values: []any{1.0}},
		{Name: "high", Type: datasource.FieldNumber, Values: []any{8.0}},
		{Name: "mid", Type: datasource.FieldNumber, Values: []any{4.0}},
	}}
	kept, shown, total := CapSeries([]datasource.Frame{wide}, 2)
	if len(kept) != 1 || len(kept[0].Fields) != 3 || kept[0].Fields[1].Name != "high" || kept[0].Fields[2].Name != "mid" {
		t.Fatalf("kept = %+v", kept)
	}
	if shown != 2 || total != 3 || len(wide.Fields) != 4 {
		t.Fatalf("shown/total/original = %d/%d/%d", shown, total, len(wide.Fields))
	}
}

func TestCapSeriesKeepsFramesThatAreNotSeries(t *testing.T) {
	table := datasource.Frame{Fields: []datasource.Field{{Name: "route", Type: datasource.FieldString, Values: []any{"/a"}}}}
	kept, _, total := CapSeries([]datasource.Frame{table, routeFrame("/a", 1.0), routeFrame("/b", 2.0)}, 1)
	if len(kept) != 2 || total != 2 || kept[0].Fields[0].Name != "route" || kept[1].Labels["route"] != "/b" {
		t.Fatalf("kept = %+v total=%d", kept, total)
	}
}
