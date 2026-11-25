package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/audit/trail/trailtest"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/origin"
)

var auditTools = []string{nameSearchAuditEvents, nameGetAuditEvent, nameListAuditFacets, nameGetAuditHistogram, nameAggregateAuditEvents, nameListAuditFields}

func TestAuditToolsRefuseEveryoneBelowAdmin(t *testing.T) {
	registry := New(Deps{Logger: slog.New(slog.DiscardHandler)})
	for _, p := range []auth.Principal{viewerCaller, editorCaller, {Kind: auth.KindServiceAccount, ID: editorCaller.ID, Role: auth.RoleEditor}} {
		for _, name := range auditTools {
			res, err := registry.Call(auth.WithPrincipal(t.Context(), p), name, []byte(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if body, ok := res.Content.(errorBody); !res.IsError || !ok || body.Code != codeForbiddenRole {
				t.Errorf("%s as %s = %+v", name, p.Role, res)
			}
		}
	}
}

func TestAuditToolsAreReadOnlyAdminToolsOfferedInRuns(t *testing.T) {
	registry := New(Deps{Logger: slog.New(slog.DiscardHandler)})
	for _, name := range auditTools {
		e, ok := registry.entries[name]
		tool := e.tool
		if !ok || !tool.ReadOnly || tool.MCPOnly || !tool.OfferedIn(ModeCreate) || tool.MinRole != auth.RoleAdmin {
			t.Errorf("%s = %+v", name, tool)
		}
	}
}

func TestAuditToolsTakeTheirFilterAsQuery(t *testing.T) {
	registry := New(Deps{Logger: slog.New(slog.DiscardHandler)})
	for _, name := range []string{nameSearchAuditEvents, nameListAuditFacets, nameGetAuditHistogram, nameAggregateAuditEvents} {
		properties := schemaProperties(t, registry.entries[name].tool.InputSchema)
		if _, ok := properties["query"]; !ok {
			t.Errorf("%s has no query property", name)
		}
		if _, ok := properties["q"]; ok {
			t.Errorf("%s still has a q property", name)
		}
		if desc := registry.entries[name].tool.Description; strings.Contains(desc, " q ") {
			t.Errorf("%s description still names q: %s", name, desc)
		}
	}
}

func TestListAuditFieldsFitsOneAnswer(t *testing.T) {
	registry := New(Deps{Logger: slog.New(slog.DiscardHandler)})
	res, err := registry.Call(auth.WithPrincipal(t.Context(), adminCaller), nameListAuditFields, nil)
	if err != nil || res.IsError {
		t.Fatalf("%+v %v", res, err)
	}
	if size := jsonSize(res.Content); size > maxContentBytes {
		t.Fatalf("the field list is %d bytes", size)
	}
}

type auditToolRig struct {
	registry *Registry
	db       *dbtest.Database
	base     time.Time
}

func newAuditToolRig(t *testing.T) auditToolRig {
	t.Helper()
	d := dbtest.New(t)
	return auditToolRig{
		registry: New(Deps{Audit: trail.NewReader(d.DB, slog.New(slog.DiscardHandler)), Logger: slog.New(slog.DiscardHandler)}),
		db:       d,
		base:     time.Now().UTC().Truncate(time.Hour).Add(2 * time.Hour),
	}
}

func (r auditToolRig) call(t *testing.T, name string, input any) Result {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.registry.Call(auth.WithPrincipal(t.Context(), adminCaller), name, raw)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (r auditToolRig) window() map[string]any {
	return map[string]any{"since": r.base.Add(-time.Hour).Format(time.RFC3339), "until": r.base.Format(time.RFC3339)}
}

func TestSearchAuditEventsCutsLargePagesAndPagesOnWithoutGaps(t *testing.T) {
	r := newAuditToolRig(t)
	events := make([]trailtest.Event, 0, 30)
	for i := range 30 {
		events = append(events, trailtest.Event{At: r.base.Add(-time.Duration(i+1) * time.Minute), TargetKind: "dashboard", TargetName: fmt.Sprintf("%02d %s", i, strings.Repeat("x", 1500))})
	}
	want := trailtest.Insert(t, r.db, events...)
	input := r.window()
	var seen []string
	for pages := 0; ; pages++ {
		if pages > len(want) {
			t.Fatalf("still paging after %d pages", pages)
		}
		res := r.call(t, nameSearchAuditEvents, input)
		if res.IsError {
			t.Fatalf("%+v", res)
		}
		if size := jsonSize(res.Content); size > maxContentBytes {
			t.Fatalf("page %d is %d bytes", pages, size)
		}
		out := res.Content.(auditSearchAnswer)
		if len(out.Events) == 0 || len(out.Events) >= defaultAuditEvents {
			t.Fatalf("page %d kept %d events", pages, len(out.Events))
		}
		for _, e := range out.Events {
			seen = append(seen, e.ID)
		}
		if out.NextCursor == nil {
			break
		}
		input["cursor"] = *out.NextCursor
	}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("paged %d events, want the %d inserted newest first without gaps or repeats", len(seen), len(want))
	}
}

func TestSearchAuditEventsSaysWhenTheSizeCapCutAPage(t *testing.T) {
	r := newAuditToolRig(t)
	events := make([]trailtest.Event, 0, 30)
	for i := range 30 {
		events = append(events, trailtest.Event{At: r.base.Add(-time.Duration(i+1) * time.Minute), TargetKind: "dashboard", TargetName: fmt.Sprintf("%02d %s", i, strings.Repeat("x", 1500))})
	}
	trailtest.Insert(t, r.db, events...)
	read := func(input map[string]any) (events int, truncated bool, note string, more bool) {
		raw, err := json.Marshal(r.call(t, nameSearchAuditEvents, input).Content)
		var out struct {
			Events     []json.RawMessage `json:"events"`
			NextCursor *string           `json:"next_cursor"`
			Truncated  bool              `json:"truncated"`
			Note       string            `json:"note"`
		}
		if err != nil || json.Unmarshal(raw, &out) != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		return len(out.Events), out.Truncated, out.Note, out.NextCursor != nil
	}
	input := r.window()
	input["limit"] = 100
	n, truncated, note, more := read(input)
	want := fmt.Sprintf("the answer size cap cut this page to %d events; continue with next_cursor", n)
	if n >= 30 || !truncated || note != want || !more {
		t.Errorf("a cut page: %d events, truncated %v, note %q, more %v; want note %q", n, truncated, note, more, want)
	}
	input["limit"] = 3
	if n, truncated, note, more = read(input); n != 3 || truncated || note != "" || !more {
		t.Errorf("a page ended by the limit: %d events, truncated %v, note %q, more %v", n, truncated, note, more)
	}
	input["since"] = r.base.Add(-time.Minute).Add(-time.Second).Format(time.RFC3339)
	input["limit"] = 100
	if n, truncated, note, more = read(input); n != 1 || truncated || note != "" || more {
		t.Errorf("the end of the results: %d events, truncated %v, note %q, more %v", n, truncated, note, more)
	}
}

func TestAuditToolsNameTheBytesOfAParseError(t *testing.T) {
	r := newAuditToolRig(t)
	for _, name := range []string{nameSearchAuditEvents, nameListAuditFacets, nameGetAuditHistogram, nameAggregateAuditEvents} {
		input := map[string]any{"query": "outcome:denied and status:x"}
		if name == nameListAuditFacets {
			input["field"] = "actor_name"
		}
		res := r.call(t, name, input)
		body, ok := res.Content.(errorBody)
		if !res.IsError || !ok || body.Code != trail.CodeQueryInvalid || !strings.Contains(body.Message, "(bytes 19 to 27 of query)") {
			t.Errorf("%s = %+v", name, res)
		}
	}
	res := r.call(t, nameSearchAuditEvents, map[string]any{"since": "now-400d"})
	if body, ok := res.Content.(errorBody); !res.IsError || !ok || body.Code != codeInvalidRequest || !strings.Contains(body.Message, "366 days") {
		t.Errorf("a long window = %+v", res)
	}
}

func TestAuditToolsWarnAboutAMisspelledOperator(t *testing.T) {
	r := newAuditToolRig(t)
	warning := `"andd" is searched as text; did you mean "and"?`
	for _, name := range []string{nameSearchAuditEvents, nameListAuditFacets, nameGetAuditHistogram} {
		input := r.window()
		input["query"] = "outcome:denied andd origin:mcp"
		if name == nameListAuditFacets {
			input["field"] = "actor_name"
		}
		res := r.call(t, name, input)
		raw, err := json.Marshal(res.Content)
		var body struct {
			Warnings []string `json:"warnings"`
		}
		if err != nil || json.Unmarshal(raw, &body) != nil || res.IsError || len(body.Warnings) != 1 || body.Warnings[0] != warning {
			t.Errorf("%s = %s", name, raw)
		}
	}
}

func TestAuditToolsReadFacetsHistogramAggregateAndOneEvent(t *testing.T) {
	r := newAuditToolRig(t)
	request := "0b3c9a52-4f0e-4c8a-9a1b-2d3e4f5a6b7c"
	ids := trailtest.Insert(t, r.db,
		trailtest.Event{At: r.base.Add(-5 * time.Minute), Actor: "Ada", Outcome: "denied", Status: 403, Request: request},
		trailtest.Event{At: r.base.Add(-6 * time.Minute), Actor: "Ada", Outcome: "denied", Status: 403, Request: request},
		trailtest.Event{At: r.base.Add(-50 * time.Minute), Actor: "Sam"},
	)

	facets := r.window()
	facets["field"], facets["query"] = "actor_name", "outcome:denied"
	res := r.call(t, nameListAuditFacets, facets)
	if out, ok := res.Content.(trail.FacetsJSON); !ok || len(out.Values) != 1 || out.Values[0].Value != "Ada" || out.Values[0].Count != 2 {
		t.Errorf("facets = %+v", res)
	}

	histogram := r.window()
	histogram["buckets"] = 6
	res = r.call(t, nameGetAuditHistogram, histogram)
	if out, ok := res.Content.(trail.HistogramJSON); !ok || out.Total != 3 || len(out.Buckets) != 6 {
		t.Errorf("histogram = %+v", res)
	}

	aggregate := r.window()
	aggregate["query"] = `{"aggs":{"by":{"terms":{"field":"actor_name"}}}}`
	res = r.call(t, nameAggregateAuditEvents, aggregate)
	if out, ok := res.Content.(trail.AggregationJSON); !ok || len(out.Rows) != 2 || out.Truncated {
		t.Errorf("aggregate = %+v", res)
	}

	res = r.call(t, nameGetAuditEvent, map[string]any{"id": ids[0]})
	if out, ok := res.Content.(trail.DetailJSON); !ok || out.Event.ID != ids[0] || len(out.SameRequest) != 2 || out.SameRequest[1].ID != ids[1] {
		t.Errorf("event = %+v", res)
	}
	res = r.call(t, nameGetAuditEvent, map[string]any{"id": trailtest.IDAt(r.base).String()})
	if body, ok := res.Content.(errorBody); !res.IsError || !ok || body.Code != trail.CodeNotFound {
		t.Errorf("an unknown event = %+v", res)
	}
}

func TestEveryAuditToolReadIsRecordedAsThatToolOverMCP(t *testing.T) {
	r := newAuditToolRig(t)
	recorder := audit.New(audit.Config{DB: r.db.DB, Server: "test", Version: "test", Logger: slog.New(slog.DiscardHandler)})
	audit.Install(recorder)
	t.Cleanup(func() { audit.Install(nil) })
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		recorder.Run(ctx)
	}()
	ids := trailtest.Insert(t, r.db, trailtest.Event{At: r.base.Add(-5 * time.Minute)})
	facets := r.window()
	facets["field"] = "origin"
	aggregate := r.window()
	aggregate["query"] = `{"aggs":{"by":{"terms":{"field":"origin"}}}}`
	calls := []struct {
		name  string
		input any
	}{
		{nameSearchAuditEvents, r.window()},
		{nameGetAuditEvent, map[string]any{"id": ids[0]}},
		{nameListAuditFacets, facets},
		{nameGetAuditHistogram, r.window()},
		{nameAggregateAuditEvents, aggregate},
		{nameListAuditFields, map[string]any{}},
	}
	caller := origin.With(auth.WithPrincipal(t.Context(), adminCaller), origin.Origin{Kind: origin.MCP, Client: "claude-code"})
	for _, c := range calls {
		raw, err := json.Marshal(c.input)
		if err != nil {
			t.Fatal(err)
		}
		if res, err := r.registry.Call(caller, c.name, raw); err != nil || res.IsError {
			t.Fatalf("%s = %+v %v", c.name, res, err)
		}
	}
	stop()
	<-done
	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT tool || ' ' || origin || ' ' || coalesce(origin_client, '') || ' ' || coalesce(actor_name, '') || ' ' || (data->>'view') FROM audit_events WHERE action = 'audit.search' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	tag := " mcp claude-code " + adminCaller.Name + " "
	want := []string{
		nameSearchAuditEvents + tag + "events",
		nameGetAuditEvent + tag + "event",
		nameListAuditFacets + tag + "facets",
		nameGetAuditHistogram + tag + "histogram",
		nameAggregateAuditEvents + tag + "aggregate",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("recorded %q, want %q", got, want)
	}
}
