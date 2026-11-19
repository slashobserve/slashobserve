package tempo_test

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestAnUnfinishedSearchWithNoRowsStillSaysItIsPartial(t *testing.T) {
	body := `{"traces":[],"metrics":{"inspectedBytes":"1805425","totalBlocks":4,"completedJobs":1,"totalJobs":7}}`
	ds, _ := serve(t, map[string]reply{"/api/search": {0, body}})

	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{}`}, lastHour)
	if err != nil {
		t.Fatal(err)
	}

	meta := datasource.MergeMeta(frames)
	if !meta.Partial || meta.JobsDone != 1 || meta.JobsTotal != 7 {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestAFinishedSearchWithNoRowsIsNotPartial(t *testing.T) {
	body := `{"traces":[],"metrics":{"completedJobs":7,"totalJobs":7}}`
	ds, _ := serve(t, map[string]reply{"/api/search": {0, body}})

	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{}`}, lastHour)
	if err != nil {
		t.Fatal(err)
	}

	if len(frames) != 0 {
		t.Fatalf("frames = %+v", frames)
	}
}
