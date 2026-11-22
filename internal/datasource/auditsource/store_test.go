package auditsource

import (
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/audit/trail/trailtest"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type trailRig struct {
	d    *dbtest.Database
	ds   *elasticsearch.Datasource
	base time.Time
	span datasource.TimeRange
}

func newTrailRig(t *testing.T) trailRig {
	t.Helper()
	d := dbtest.New(t)
	base := time.Now().UTC().Truncate(time.Hour)
	trailtest.Insert(t, d,
		trailtest.Event{At: base.Add(1 * time.Minute), Origin: "web", Actor: "Ada", Status: 200},
		trailtest.Event{At: base.Add(2 * time.Minute), Origin: "mcp", Actor: "Riya", Outcome: "denied", Status: 403, TargetKind: "dashboard", TargetName: "Checkout"},
		trailtest.Event{At: base.Add(3 * time.Minute), Origin: "mcp", Actor: "Riya", Action: "query.run", Status: 200},
		trailtest.Event{At: base.Add(4 * time.Minute), Origin: "web", Actor: "Ada", Outcome: "denied", Status: 403},
		trailtest.Event{At: base.Add(90 * time.Minute), Origin: "web", Actor: "Bo"},
	)
	in, err := Instance(trail.NewReader(d.DB, slog.New(slog.DiscardHandler)), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return trailRig{d: d, ds: in.Datasource.(*elasticsearch.Datasource), base: base, span: datasource.TimeRange{From: base, To: base.Add(time.Hour)}}
}

func TestTheAdapterReadsTheAuditTrailInEveryQueryShape(t *testing.T) {
	r := newTrailRig(t)
	ctx := t.Context()
	if err := r.ds.CheckHealth(ctx); err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}

	frames, err := r.ds.Lines(ctx, datasource.LinesRequest{Query: datasource.Query{Text: "outcome:denied"}, Range: r.span, Direction: datasource.Forward, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	body := column(t, frames[0], "Body")
	if len(body) != 2 || body[0] != `dashboard.update denied by Riya via mcp on dashboard "Checkout"` {
		t.Fatalf("denied lines = %v", body)
	}

	frames, err = r.ds.QueryRange(ctx, datasource.Query{Text: "origin:mcp"}, r.span, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var counted float64
	for _, v := range frames[0].Fields[1].Values {
		n, _ := v.(float64)
		counted += n
	}
	if counted != 2 || len(frames[0].Fields[1].Values) != 7 {
		t.Fatalf("mcp count series = %v", frames[0].Fields[1].Values)
	}

	_, info, err := r.ds.QueryPage(ctx, datasource.PageRequest{Query: datasource.Query{Text: ""}, Range: r.span, Page: 2, PageSize: 3})
	if err != nil || info.Total != 4 || info.TotalRelation != datasource.TotalExact || info.HasNext {
		t.Fatalf("page 2 = %+v, %v", info, err)
	}

	facets, err := r.ds.LogFacets(ctx, datasource.FacetRequest{Query: datasource.Query{Text: ""}, Range: r.span, Fields: []string{"actor_name"}})
	if err != nil {
		t.Fatal(err)
	}
	if f := facets.Fields[0]; len(f.Values) != 2 || f.Values[0].Count != 2 || f.Cardinality != 2 {
		t.Fatalf("actor facet = %+v", f)
	}

	frames, err = r.ds.QueryInstant(ctx, datasource.Query{Text: `{"aggs":{"by":{"terms":{"field":"origin"},"aggs":{"slowest":{"max":{"field":"status"}}}}}}`}, r.span)
	if err != nil {
		t.Fatal(err)
	}
	origins, slowest := column(t, frames[0], "origin"), column(t, frames[0], "slowest")
	if len(origins) != 2 || len(slowest) != 2 {
		t.Fatalf("terms table = %+v", frames[0])
	}

	frames, err = r.ds.Lines(ctx, datasource.LinesRequest{Query: datasource.Query{Text: `{"query":{"query_string":{"query":"actor_name:Ada AND outcome:denied"}}}`}, Range: r.span, Limit: 10})
	if err != nil || len(column(t, frames[0], "Body")) != 1 {
		t.Fatalf("lucene lines = %+v, %v", frames, err)
	}

	frames, err = r.ds.QueryRange(ctx, datasource.Query{Text: "", TimeZone: "Asia/Kolkata"}, datasource.TimeRange{From: r.base.Add(-24 * time.Hour), To: r.base.Add(2 * time.Hour)}, 24*time.Hour)
	if err != nil {
		t.Fatalf("a count in a named zone: %v", err)
	}
	var all float64
	for _, v := range frames[0].Fields[1].Values {
		n, _ := v.(float64)
		all += n
	}
	if all != 5 {
		t.Fatalf("zoned daily counts = %v", frames[0].Fields[1].Values)
	}

	_, err = r.ds.QueryInstant(ctx, datasource.Query{Text: `{"query":{"term":{"no_such_field":"x"}}}`}, r.span)
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("an unknown field: %v", err)
	}
}
