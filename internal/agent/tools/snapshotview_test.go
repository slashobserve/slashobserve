package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/observations"
)

func seriesFrame(labels map[string]string, times []any, values []any) datasource.Frame {
	return datasource.Frame{
		Name:   "http_requests",
		Labels: labels,
		Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: times},
			{Name: "Value", Type: datasource.FieldNumber, Values: values},
		},
	}
}

func TestSummarizeSnapshotReadsMinMaxLastAndWhenFromEachSeries(t *testing.T) {
	frame := seriesFrame(
		map[string]string{"service": "checkout"},
		[]any{"2026-11-20T09:00:00Z", "2026-11-20T09:01:00Z", float64(time.Date(2026, 11, 20, 9, 2, 0, 0, time.UTC).UnixMilli()), "2026-11-20T09:03:00Z"},
		[]any{float64(4), float64(9), float64(1), nil},
	)
	got := summarizeSnapshot([]datasource.Frame{frame})
	if got.SeriesTotal != 1 || len(got.Series) != 1 {
		t.Fatalf("summary = %+v", got)
	}
	s := got.Series[0]
	if *s.Min != 1 || s.MinAt != "2026-11-20T09:02:00Z" || *s.Max != 9 || s.MaxAt != "2026-11-20T09:01:00Z" || *s.Last != 1 || s.Points != 4 {
		t.Fatalf("series = %+v", s)
	}
	if s.Name != "http_requests" || s.Labels["service"] != "checkout" {
		t.Fatalf("series name and labels = %q %v", s.Name, s.Labels)
	}
}

func TestSummarizeSnapshotCapsTheSeriesAndCountsTheRest(t *testing.T) {
	var frames []datasource.Frame
	for i := range maxSnapshotSeries + 5 {
		frames = append(frames, seriesFrame(map[string]string{"pod": fmt.Sprint(i)}, []any{"2026-11-20T09:00:00Z"}, []any{float64(i)}))
	}
	frames = append(frames, datasource.Frame{Fields: []datasource.Field{{Name: "msg", Type: datasource.FieldString, Values: []any{"a"}}}})
	got := summarizeSnapshot(frames)
	if len(got.Series) != maxSnapshotSeries || got.SeriesTotal != maxSnapshotSeries+5 || got.OtherFrames != 1 {
		t.Fatalf("series %d, total %d, other %d", len(got.Series), got.SeriesTotal, got.OtherFrames)
	}
}

func TestSummarizeSnapshotClipsLongNamesAndLabels(t *testing.T) {
	long := strings.Repeat("x", 1000)
	frame := seriesFrame(map[string]string{long: long}, []any{"2026-11-20T09:00:00Z"}, []any{float64(1)})
	frame.Name = long
	s := summarizeSnapshot([]datasource.Frame{frame}).Series[0]
	if len(s.Name) > maxSnapshotNameBytes+8 {
		t.Fatalf("name is %d bytes", len(s.Name))
	}
	for k, v := range s.Labels {
		if len(k) > maxSnapshotLabelBytes+8 || len(v) > maxSnapshotLabelBytes+8 {
			t.Fatalf("label %d:%d bytes", len(k), len(v))
		}
	}
}

func TestSnapshotViewNamesThePanelItsRangeAndItsDataButNeverAQuery(t *testing.T) {
	snap := observations.Snapshot{
		ID:    "s1",
		Panel: json.RawMessage(`{"id":"p1","type":"timeseries","title":"Request rate","unit":"reqps","gridPos":{"x":0,"y":0,"w":12,"h":8}}`),
		Query: json.RawMessage(`[{"ref":"A","expr":"sum(rate(secret_metric_total[5m]))"}]`),
		Data:  json.RawMessage(`{"frames":[{"fields":[{"name":"Time","type":"time","values":["2026-11-20T09:00:00Z"]},{"name":"Value","type":"number","values":[3]}]}],"meta":{"kind":"range"}}`),
		From:  time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
	}
	view := snapshotView(blockViewOut{ID: "b1", Kind: "snapshot", SnapshotID: "s1"}, snap)
	if view.Title != "Request rate" || view.Type != "timeseries" || view.Unit != "reqps" || view.From != "2026-11-20T08:00:00Z" || view.To != "2026-11-20T10:00:00Z" {
		t.Fatalf("view = %+v", view)
	}
	if view.Data == nil || view.Data.SeriesTotal != 1 || *view.Data.Series[0].Last != 3 {
		t.Fatalf("data = %+v", view.Data)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret_metric") || strings.Contains(string(raw), "expr") {
		t.Fatalf("the view carries query text: %s", raw)
	}
}

func TestSnapshotViewKeepsTheBlockWhenTheFramesCannotBeRead(t *testing.T) {
	view := snapshotView(blockViewOut{ID: "b1"}, observations.Snapshot{Panel: json.RawMessage(`{"title":"T","type":"stat"}`), Data: json.RawMessage(`not json`)})
	if view.Title != "T" || view.Data != nil {
		t.Fatalf("view = %+v", view)
	}
}
