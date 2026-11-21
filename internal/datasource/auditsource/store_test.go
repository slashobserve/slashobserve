package auditsource

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/audit/trail/trailtest"
	"github.com/slashobserve/slashobserve/internal/auth"
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

func TestAReadThroughTheDatasourceIsOneQueryRunPerQueryAndPeriodInTenMinutes(t *testing.T) {
	r := newTrailRig(t)
	recorder := audit.New(audit.Config{DB: r.d.DB})
	mw, err := recorder.Middleware()
	if err != nil {
		t.Fatal(err)
	}
	admin := auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Role: auth.RoleAdmin}
	engine := gin.New()
	engine.POST("/api/v1/datasources/:uid/query", mw, func(c *gin.Context) {
		ctx := auth.WithPrincipal(c.Request.Context(), admin)
		audit.Update(ctx, func(s *audit.Scope) { s.Actor = auth.AuditActor(admin) })
		minutes := 30 + 30*len(c.Query("wider"))
		_, err := r.ds.Lines(ctx, datasource.LinesRequest{Query: datasource.Query{Text: c.Query("q")}, Range: datasource.TimeRange{From: r.base, To: r.base.Add(time.Duration(minutes) * time.Minute)}, Limit: 10})
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "query failed", "code": "query_invalid"})
			return
		}
		c.JSON(http.StatusOK, gin.H{})
	})

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.Run(ctx)
	}()
	for _, query := range []string{"q=outcome:denied", "q=outcome:denied&wider=x", "q=origin:mcp", "q=outcome:denied", "q=no_such_field:x", "q=no_such_field:x"} {
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+UID+"/query?"+query, strings.NewReader("{}")))
	}
	stop()
	<-done

	rows, err := r.d.SQLDB(t).QueryContext(t.Context(), `SELECT action, outcome, coalesce(data->>'from', ''), coalesce(data->>'to', ''), coalesce(datasource_uid, '') FROM audit_events WHERE route = '/api/v1/datasources/{uid}/query' ORDER BY occurred_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var action, outcome, from, to, uid string
		if err := rows.Scan(&action, &outcome, &from, &to, &uid); err != nil {
			t.Fatal(err)
		}
		if outcome == "success" && uid != UID {
			t.Errorf("a successful query.run names datasource %q, want %q", uid, UID)
		}
		if len(got) == 0 && (from != r.base.UTC().Format(time.RFC3339) || to != r.base.Add(30*time.Minute).UTC().Format(time.RFC3339)) {
			t.Errorf("the first query.run holds from %q and to %q, want its 30 minute range", from, to)
		}
		got = append(got, action+" "+outcome)
	}
	want := []string{"query.run success", "query.run success", "query.run success", "query.run failed", "query.run failed"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("recorded %v, want %v: a repeat of a query over the same period is not recorded again, a wider period and a failure always are", got, want)
	}
}
