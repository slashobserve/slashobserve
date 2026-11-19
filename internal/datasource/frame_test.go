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

func TestMarkPartialGivesAnEmptyResultAFrameToCarryTheFlag(t *testing.T) {
	frames := MarkPartial(nil, "store a is down")

	got := MergeMeta(frames)
	if len(frames) != 1 || len(frames[0].Fields) != 0 || !got.Partial || got.Warning != "store a is down" {
		t.Fatalf("frames = %+v, merged = %+v", frames, got)
	}
}

func TestMergeMetaJoinsEachWarningOnce(t *testing.T) {
	frames := []Frame{
		{Meta: &FrameMeta{Partial: true, Warning: "store a is down"}},
		{Meta: &FrameMeta{Partial: true, Warning: "store b is slow"}},
		{Meta: &FrameMeta{Partial: true, Warning: "store a is down"}},
	}

	if got := MergeMeta(frames).Warning; got != "store a is down; store b is slow" {
		t.Fatalf("warning = %q", got)
	}
}
