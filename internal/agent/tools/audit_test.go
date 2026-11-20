package tools

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/audit/trail/trailtest"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
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

func TestAuditToolsAreReadOnlyAndMCPOnly(t *testing.T) {
	registry := New(Deps{Logger: slog.New(slog.DiscardHandler)})
	for _, name := range auditTools {
		e, ok := registry.entries[name]
		tool := e.tool
		if !ok || !tool.ReadOnly || !tool.MCPOnly || tool.MinRole != auth.RoleAdmin {
			t.Errorf("%s = %+v", name, tool)
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
		out := res.Content.(trail.SearchJSON)
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

func TestAuditToolsNameTheBytesOfAParseError(t *testing.T) {
	r := newAuditToolRig(t)
	for _, name := range []string{nameSearchAuditEvents, nameListAuditFacets, nameGetAuditHistogram, nameAggregateAuditEvents} {
		res := r.call(t, name, map[string]any{"q": "outcome:denied and status:x", "field": "actor_name"})
		body, ok := res.Content.(errorBody)
		if !res.IsError || !ok || body.Code != trail.CodeQueryInvalid || !strings.Contains(body.Message, "(bytes 19 to 27 of q)") {
			t.Errorf("%s = %+v", name, res)
		}
	}
	res := r.call(t, nameSearchAuditEvents, map[string]any{"since": "now-40d"})
	if body, ok := res.Content.(errorBody); !res.IsError || !ok || body.Code != codeInvalidRequest || !strings.Contains(body.Message, "31 days") {
		t.Errorf("a long window = %+v", res)
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
	facets["field"], facets["q"] = "actor_name", "outcome:denied"
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
	aggregate["q"] = `{"aggs":{"by":{"terms":{"field":"actor_name"}}}}`
	res = r.call(t, nameAggregateAuditEvents, aggregate)
	if out, ok := res.Content.(trail.AggregationJSON); !ok || len(out.Rows) != 2 || out.Truncated {
		t.Errorf("aggregate = %+v", res)
	}

	res = r.call(t, nameGetAuditEvent, map[string]any{"id": ids[0]})
	if out, ok := res.Content.(trail.DetailJSON); !ok || out.Event.ID != ids[0] || len(out.SameRequest) != 1 || out.SameRequest[0].ID != ids[1] {
		t.Errorf("event = %+v", res)
	}
	res = r.call(t, nameGetAuditEvent, map[string]any{"id": trailtest.IDAt(r.base).String()})
	if body, ok := res.Content.(errorBody); !res.IsError || !ok || body.Code != trail.CodeNotFound {
		t.Errorf("an unknown event = %+v", res)
	}
}
