package datasource

import "testing"

func TestMergeMetaSumsCounts(t *testing.T) {
	frames := []Frame{
		{Meta: &FrameMeta{Partial: true, RowsTotal: 10, JobsDone: 1, JobsTotal: 6, InspectedBytes: 100, Bytes: 7}},
		{},
		{Meta: &FrameMeta{Truncated: true, RowsTotal: 20, JobsDone: 2, JobsTotal: 3, InspectedBytes: 50, Bytes: 8}},
	}
	got := MergeMeta(frames)
	want := FrameMeta{Truncated: true, Partial: true, RowsTotal: 10, JobsDone: 3, JobsTotal: 9, InspectedBytes: 150, Bytes: 15}
	if got != want {
		t.Fatalf("merged = %+v, want %+v", got, want)
	}
	if got := MergeMeta(nil); got != (FrameMeta{}) {
		t.Fatalf("merged nothing = %+v, want zero", got)
	}
}
