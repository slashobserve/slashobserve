package trail

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/audit/trail/trailtest"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(dbtest.Main(m))
}

type readerRig struct {
	db     *dbtest.Database
	reader *Reader
	base   time.Time
}

func newReaderRig(t *testing.T) readerRig {
	t.Helper()
	d := dbtest.New(t)
	base := time.Now().UTC().Truncate(time.Hour).Add(2 * time.Hour)
	return readerRig{db: d, reader: NewReader(d.DB, slog.New(slog.DiscardHandler)), base: base}
}

func (r readerRig) window(t *testing.T, text string, from, to time.Duration) Prepared {
	t.Helper()
	p, err := Prepare(Request{Text: text, Since: r.base.Add(from).Format(time.RFC3339), Until: r.base.Add(to).Format(time.RFC3339)}, r.reader.Now())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (r readerRig) at(minutes int) time.Time {
	return r.base.Add(time.Duration(minutes) * time.Minute)
}

func idsOf(events []Event) []string {
	ids := make([]string, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.ID)
	}
	return ids
}

func TestPagesNeitherRepeatNorSkipWhileEventsArrive(t *testing.T) {
	r := newReaderRig(t)
	var seeds []trailtest.Event
	for i := range 20 {
		seeds = append(seeds, trailtest.Event{At: r.at(-50 + i*2)})
	}
	tie := r.at(-11)
	seeds = append(seeds, trailtest.Event{At: tie}, trailtest.Event{At: tie}, trailtest.Event{At: tie})
	ids := trailtest.Insert(t, r.db, seeds...)
	want := make([]struct {
		at time.Time
		id string
	}, len(ids))
	for i, id := range ids {
		want[i].at, want[i].id = seeds[i].At, id
	}
	slices.SortFunc(want, func(a, b struct {
		at time.Time
		id string
	}) int {
		if c := b.at.Compare(a.at); c != 0 {
			return c
		}
		return strings.Compare(b.id, a.id)
	})

	p := r.window(t, "", -time.Hour, 0)
	var got []string
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("paging did not end")
		}
		page, err := r.reader.Search(t.Context(), p, 4)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, idsOf(page.Events)...)
		trailtest.Insert(t, r.db, trailtest.Event{At: r.at(-1)}, trailtest.Event{At: r.base.Add(-time.Second)})
		if page.NextCursor == "" {
			break
		}
		if p, err = Prepare(Request{Cursor: page.NextCursor, Since: "now-1m"}, r.reader.Now()); err != nil {
			t.Fatal(err)
		}
	}
	var wantIDs []string
	for _, w := range want {
		wantIDs = append(wantIDs, w.id)
	}
	if !slices.Equal(got, wantIDs) {
		t.Fatalf("paged ids\n%v\nwant\n%v", got, wantIDs)
	}
}

func TestAnActorIsFoundByEmailAndByNameInEveryQueryLanguage(t *testing.T) {
	r := newReaderRig(t)
	const adaID = "7d1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11"
	r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1, 'i', 's', 'Admin@Example.com', 'Ada Admin')`, adaID)
	ids := trailtest.Insert(t, r.db,
		trailtest.Event{At: r.at(-5), Actor: "Ada Admin", ActorID: adaID},
		trailtest.Event{At: r.at(-6), Actor: "Grace"},
	)
	for _, text := range []string{
		`actor.email:"admin@example.com"`, `actor:"admin@example.com"`, `actor.email:admin@exam*`, `actor:Ada*`, `actor.name:"Ada Admin"`, `actor_name:"Ada Admin"`,
		`lucene: actor.email:"admin@example.com"`, `{"query":{"term":{"actor.email":"admin@example.com"}}}`,
		`actor.id:` + adaID,
	} {
		page, err := r.reader.Search(t.Context(), r.window(t, text, -time.Hour, 0), 10)
		if err != nil {
			t.Fatalf("Search(%q): %v", text, err)
		}
		if got := idsOf(page.Events); len(got) != 1 || got[0] != ids[0] {
			t.Errorf("Search(%q) = %v, want only the event of the user", text, got)
		}
	}
	for _, text := range []string{`actor.name:"admin@example.com"`, `actor.email:"Ada Admin"`, `actor.email:nobody@example.com`} {
		page, err := r.reader.Search(t.Context(), r.window(t, text, -time.Hour, 0), 10)
		if err != nil || len(page.Events) != 0 {
			t.Errorf("Search(%q) = %v, %v, want no event", text, idsOf(page.Events), err)
		}
	}
}

func TestFacetsCountTheMatchingValuesMostFirst(t *testing.T) {
	r := newReaderRig(t)
	var seeds []trailtest.Event
	for actor, n := range map[string]int{"Ada": 3, "Bo": 2, "Cy": 1} {
		for range n {
			seeds = append(seeds, trailtest.Event{At: r.at(-10), Actor: actor, Outcome: "denied"})
		}
	}
	seeds = append(seeds, trailtest.Event{At: r.at(-10), Actor: "Bo"}, trailtest.Event{At: r.at(-10), Actor: "Bo"}, trailtest.Event{At: r.at(-10), Actor: "Dee"})
	trailtest.Insert(t, r.db, seeds...)
	values, err := r.reader.Facets(t.Context(), r.window(t, "outcome:denied", -time.Hour, 0), "actor_name", 10)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Facet{{"Ada", 3}, {"Bo", 2}, {"Cy", 1}}; !slices.Equal(values, want) {
		t.Fatalf("facets = %v, want %v", values, want)
	}
	values, err = r.reader.Facets(t.Context(), r.window(t, "", -time.Hour, 0), "actor_name", 1)
	if err != nil || !slices.Equal(values, []Facet{{"Bo", 4}}) {
		t.Fatalf("top value = %v, %v", values, err)
	}
	_, err = r.reader.Facets(t.Context(), r.window(t, "", -time.Hour, 0), "no_such_field", 10)
	if problem, ok := ProblemOf(err); !ok || problem.Code != CodeQueryInvalid {
		t.Fatalf("an unknown field: %v", err)
	}
}

func TestTheHistogramCountsEachBucketAndKeepsEmptyOnes(t *testing.T) {
	r := newReaderRig(t)
	trailtest.Insert(t, r.db,
		trailtest.Event{At: r.at(-55)}, trailtest.Event{At: r.at(-5)}, trailtest.Event{At: r.at(-5), Outcome: "failed"},
		trailtest.Event{At: r.at(-61)}, trailtest.Event{At: r.at(1)},
	)
	h, err := r.reader.Histogram(t.Context(), r.window(t, "", -time.Hour, 0), 6)
	if err != nil {
		t.Fatal(err)
	}
	var counts []int64
	for _, b := range h.Buckets {
		counts = append(counts, b.Count)
	}
	if h.StepMs != (10*time.Minute).Milliseconds() || h.Total != 3 || !slices.Equal(counts, []int64{1, 0, 0, 0, 0, 2}) {
		t.Fatalf("histogram = %+v", h)
	}
	if h.Buckets[0].Time != r.at(-60).Format(time.RFC3339) {
		t.Fatalf("first bucket at %s", h.Buckets[0].Time)
	}
	h, err = r.reader.Histogram(t.Context(), r.window(t, "outcome:failed", -time.Hour, 0), 6)
	if err != nil || h.Total != 1 {
		t.Fatalf("failed only = %+v, %v", h, err)
	}
}

func TestAggregateRunsTermsAndMetrics(t *testing.T) {
	r := newReaderRig(t)
	trailtest.Insert(t, r.db,
		trailtest.Event{At: r.at(-5), Origin: "web", Actor: "Ada"}, trailtest.Event{At: r.at(-5), Origin: "web", Actor: "Bo"},
		trailtest.Event{At: r.at(-5), Origin: "web", Actor: "Ada"}, trailtest.Event{At: r.at(-5), Origin: "mcp", Actor: "Ada"},
	)
	p := r.window(t, `{"aggs":{"by":{"terms":{"field":"origin"},"aggs":{"people":{"cardinality":{"field":"actor_name"}}}}}}`, -time.Hour, 0)
	agg, err := r.reader.Aggregate(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][2]float64{}
	for _, row := range agg.Rows {
		got[row.Keys[0].(string)] = [2]float64{float64(row.DocCount), *row.Values[0]}
	}
	if len(agg.Levels) != 1 || len(agg.Metrics) != 1 || got["web"] != [2]float64{3, 2} || got["mcp"] != [2]float64{1, 1} || agg.Truncated {
		t.Fatalf("aggregation = %+v, rows %v", agg, got)
	}
	_, err = r.reader.Aggregate(t.Context(), r.window(t, `{"aggs":{"t":{"date_histogram":{"field":"occurred_at","fixed_interval":"1m"},"aggs":{"o":{"terms":{"field":"actor_name","size":100}}}}}}`, -24*time.Hour, 0))
	if problem, ok := ProblemOf(err); !ok || problem.Code != CodeQueryInvalid || !strings.Contains(problem.Message, "100000") {
		t.Fatalf("an aggregation over the row limit: %+v, %v", problem, err)
	}
}

func TestAnEventComesWithItsRequestAndRun(t *testing.T) {
	r := newReaderRig(t)
	request, run := uuid.NewString(), uuid.NewString()
	ids := trailtest.Insert(t, r.db,
		trailtest.Event{At: r.at(-10), Request: request, Run: run},
		trailtest.Event{At: r.at(-10), Request: request},
		trailtest.Event{At: r.at(-9), Request: request},
		trailtest.Event{At: r.at(-30), Run: run},
		trailtest.Event{At: r.at(-10)},
	)
	d, err := r.reader.Event(t.Context(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if d.Event.ID != ids[0] || len(d.SameRequest) != 3 || len(d.SameRun) != 2 || d.SameRun[1].ID != ids[3] {
		t.Fatalf("detail = %+v", d)
	}
	d, err = r.reader.Event(t.Context(), ids[4])
	if err != nil || len(d.SameRequest) != 0 || len(d.SameRun) != 0 {
		t.Fatalf("an event with no request = %+v, %v", d, err)
	}
	for _, id := range []string{trailtest.IDAt(r.at(-10)).String(), uuid.NewString(), "nope"} {
		if _, err := r.reader.Event(t.Context(), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Event(%q) = %v, want not found", id, err)
		}
	}
}

type bufferSink struct {
	buf   bytes.Buffer
	begun int
}

func (s *bufferSink) sink() Sink {
	return Sink{Begin: func() io.Writer { s.begun++; return &s.buf }, Flush: func() {}}
}

func TestExportStopsAtItsLimitAndPagesPastOneRead(t *testing.T) {
	r := newReaderRig(t)
	r.db.Exec(t, `INSERT INTO audit_events (id, occurred_at, category, action, outcome, actor_kind, initiator, origin, server_id, server_version)
SELECT gen_random_uuid(), $1::timestamptz - make_interval(secs => n), 'content', 'dashboard.update', 'success', 'user', 'person', 'web', 'test', 'test'
FROM generate_series(1, 1203) AS n`, r.base)
	p := r.window(t, "", -time.Hour, 0)

	var all bufferSink
	result, err := r.reader.Export(t.Context(), p, FormatNDJSON, MaxExportRows, all.sink())
	if err != nil {
		t.Fatal(err)
	}
	lines, seen := 0, map[string]bool{}
	scanner := bufio.NewScanner(&all.buf)
	for scanner.Scan() {
		var e struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		seen[e.ID] = true
		lines++
	}
	if result.Rows != 1203 || result.Truncated || lines != 1203 || len(seen) != 1203 || all.begun != 1 {
		t.Fatalf("export = %+v, %d lines, %d ids, begun %d", result, lines, len(seen), all.begun)
	}

	var capped bufferSink
	result, err = r.reader.Export(t.Context(), p, FormatCSV, 5, capped.sink())
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&capped.buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows != 5 || !result.Truncated || len(records) != 6 || !slices.Equal(records[0], csvHeader) {
		t.Fatalf("capped export = %+v, %d records", result, len(records))
	}

	var exact bufferSink
	result, err = r.reader.Export(t.Context(), p, FormatNDJSON, 1203, exact.sink())
	if err != nil || result.Rows != 1203 || result.Truncated {
		t.Fatalf("an export of exactly the limit = %+v, %v", result, err)
	}
}

func TestAFailedReadWritesNoExportHeaders(t *testing.T) {
	r := newReaderRig(t)
	trailtest.Insert(t, r.db, trailtest.Event{At: r.at(-5)})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var sink bufferSink
	if _, err := r.reader.Export(ctx, r.window(t, "", -time.Hour, 0), FormatNDJSON, MaxExportRows, sink.sink()); err == nil || sink.begun != 0 {
		t.Fatalf("err = %v, begun %d", err, sink.begun)
	}
}

func TestALongReadIsCutAtTheStatementTimeout(t *testing.T) {
	r := newReaderRig(t)
	r.reader.timeout = 50 * time.Millisecond
	err := r.reader.read(t.Context(), func(tx db.Rows) error {
		rows, err := tx.Query(t.Context(), "SELECT pg_sleep(5)")
		if err != nil {
			return err
		}
		rows.Close()
		return rows.Err()
	})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want the timeout", err)
	}
}

func TestReadsRunReadOnly(t *testing.T) {
	r := newReaderRig(t)
	err := r.reader.read(t.Context(), func(tx db.Rows) error {
		rows, err := tx.Query(t.Context(), "INSERT INTO audit_events (id, occurred_at, category, action, outcome, actor_kind, initiator, origin, server_id, server_version) VALUES (gen_random_uuid(), now(), 'content', 'a.b', 'success', 'user', 'person', 'web', 't', 't')")
		if err != nil {
			return err
		}
		rows.Close()
		return rows.Err()
	})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("a write in a read = %v", err)
	}
}
