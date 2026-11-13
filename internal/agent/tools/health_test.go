package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/health"
	"github.com/slashobserve/slashobserve/internal/health/healthrule"
)

func TestHealthParamsBoundTheInput(t *testing.T) {
	got, _, ok := healthParams(listHealthInput{})
	if !ok || got.Limit != defaultListHealth || got.Kind != "" || got.FolderID.Valid {
		t.Fatalf("defaults = %+v, ok %v", got, ok)
	}
	got, _, ok = healthParams(listHealthInput{FolderID: "0b9e9a3e-5d5e-4f0e-9e1a-7d2f3c4b5a61", Kind: "no_data", Limit: ptr(50), Cursor: "c"})
	if !ok || got.Limit != 50 || got.Kind != health.KindNoData || !got.FolderID.Valid || got.Cursor != "c" {
		t.Fatalf("full input = %+v, ok %v", got, ok)
	}
	one, _, ok := healthParams(listHealthInput{DashboardID: "0b9e9a3e-5d5e-4f0e-9e1a-7d2f3c4b5a61"})
	if !ok || len(one.DashboardIDs) != 1 || one.DashboardIDs[0].String() != "0b9e9a3e-5d5e-4f0e-9e1a-7d2f3c4b5a61" {
		t.Fatalf("dashboard_id = %+v, ok %v", one.DashboardIDs, ok)
	}
	cases := []struct {
		name    string
		in      listHealthInput
		message string
	}{
		{"limit 0", listHealthInput{Limit: ptr(0)}, "limit must be between 1 and 50"},
		{"limit 51", listHealthInput{Limit: ptr(51)}, "limit must be between 1 and 50"},
		{"folder not a uuid", listHealthInput{FolderID: "shop"}, "folder_id must be a UUID from list_folders"},
		{"dashboard not a uuid", listHealthInput{DashboardID: "shop"}, "dashboard_id must be a UUID from list_dashboards"},
		{"unknown kind", listHealthInput{Kind: "slow"}, "kind must be one of no_data, query_failed, variable_failed, duplicate_title, unopened"},
		{"long cursor", listHealthInput{Cursor: strings.Repeat("c", maxNameBytes+1)}, "cursor must be at most 256 bytes"},
	}
	for _, c := range cases {
		_, res, ok := healthParams(c.in)
		body, _ := res.Content.(errorBody)
		if ok || body.Code != codeInvalidRequest || body.Message != c.message {
			t.Errorf("%s: ok %v, %+v", c.name, ok, body)
		}
	}
}

func TestHealthSchemaOffersEveryKind(t *testing.T) {
	var schema struct {
		Properties struct {
			Kind struct {
				Enum []health.Kind `json:"enum"`
			} `json:"kind"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(schemaListDashboardHealth), &schema); err != nil {
		t.Fatal(err)
	}
	if strings.Join(kindStrings(schema.Properties.Kind.Enum), ",") != strings.Join(kindStrings(healthrule.Kinds), ",") {
		t.Errorf("kind enum = %v, want %v", schema.Properties.Kind.Enum, healthrule.Kinds)
	}
}

func kindStrings(kinds []health.Kind) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

func TestHealthItemOfLeavesOutEmptyFields(t *testing.T) {
	folder := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	bare := healthItemOf(health.Item{ID: "d1", Title: "Shop", URL: "https://observe.example.com/d/d1", Folder: folders.Folder{ID: folder, Title: "General"}, State: "healthy", Problems: []health.Problem{{Kind: health.KindUnopened}}})
	raw, err := json.Marshal(bare)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"dashboard_id":"d1","title":"Shop","url":"https://observe.example.com/d/d1","folder_id":"` + folder.String() + `","folder_title":"General","state":"healthy","problems":[{"kind":"unopened"}]}`
	if string(raw) != want {
		t.Errorf("bare item = %s\nwant %s", raw, want)
	}

	checked := time.Date(2026, 11, 6, 9, 30, 0, 0, time.FixedZone("IST", 19800))
	version := 3
	full := healthItemOf(health.Item{
		ID: "d2", Folder: folders.Folder{ID: folder}, CheckedAt: &checked, CheckedVersion: &version, Skipped: health.SkippedDependsOnViewer,
		Repair:   &health.Repair{State: "succeeded", Outcome: "still_failing", Version: &version},
		Problems: []health.Problem{{Kind: health.KindQueryFailed, PanelID: "p1", PanelTitle: "Errors", Code: "query_failed", Message: strings.Repeat("x", 2*maxProblemTextBytes)}},
	})
	if full.CheckedAt != "2026-11-06T04:00:00Z" || full.CheckedVersion == nil || *full.CheckedVersion != 3 || full.Skipped != health.SkippedDependsOnViewer {
		t.Errorf("checked fields = %q %v %q", full.CheckedAt, full.CheckedVersion, full.Skipped)
	}
	if r := full.Repair; r == nil || r.State != "succeeded" || r.Outcome != "still_failing" || r.Version == nil || *r.Version != 3 {
		t.Errorf("repair = %+v", r)
	}
	if p := full.Problems[0]; p.PanelID != "p1" || p.PanelTitle != "Errors" || p.Code != "query_failed" || len(p.Message) >= 2*maxProblemTextBytes {
		t.Errorf("problem = %+v (message %d bytes)", p, len(p.Message))
	}
}

func TestHealthJobOfFormatsTimesInUTC(t *testing.T) {
	started := time.Date(2026, 11, 6, 10, 0, 0, 0, time.FixedZone("CET", 3600))
	got := healthJobOf(health.JobStatus{LastStartedAt: &started, LastOutcome: "succeeded", Running: true})
	if got != (healthJobOut{LastStartedAt: "2026-11-06T09:00:00Z", LastOutcome: "succeeded", Running: true}) {
		t.Errorf("job = %+v", got)
	}
}

func bigHealthItem(id string, problems int) health.Item {
	it := health.Item{ID: id, Title: "Board " + id, Folder: folders.Folder{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}}
	for range problems {
		it.Problems = append(it.Problems, health.Problem{Kind: health.KindQueryFailed, PanelID: "p", Message: strings.Repeat("m", maxProblemTextBytes)})
	}
	return it
}

func TestHealthPageAsksForFewerDashboardsWhenTheyDoNotFit(t *testing.T) {
	var items []health.Item
	for i := range 10 {
		items = append(items, bigHealthItem(string(rune('a'+i)), 5))
	}
	_, smaller, retry := healthPage(health.Page{Items: items, NextCursor: "next"})
	if !retry || smaller < 1 || smaller >= len(items) {
		t.Fatalf("retry %v with limit %d of %d", retry, smaller, len(items))
	}
	out, _, retry := healthPage(health.Page{Items: items[:smaller], NextCursor: "next"})
	if retry || len(out.Dashboards) != smaller || jsonSize(out) > listBudget || out.NextCursor == nil || *out.NextCursor != "next" {
		t.Errorf("second page: retry %v, %d dashboards, %d bytes", retry, len(out.Dashboards), jsonSize(out))
	}
}

func TestHealthPageCutsTheProblemsOfOneDashboardThatDoesNotFit(t *testing.T) {
	_, smaller, retry := healthPage(health.Page{Items: []health.Item{bigHealthItem("a", 100), bigHealthItem("b", 1)}})
	if !retry || smaller != 1 {
		t.Fatalf("retry %v with limit %d", retry, smaller)
	}
	out, _, retry := healthPage(health.Page{Items: []health.Item{bigHealthItem("a", 100)}})
	if retry || len(out.Dashboards) != 1 || jsonSize(out) > listBudget {
		t.Fatalf("retry %v, %d dashboards, %d bytes", retry, len(out.Dashboards), jsonSize(out))
	}
	if d := out.Dashboards[0]; !d.ProblemsTruncated || len(d.Problems) == 0 || len(d.Problems) >= 100 {
		t.Errorf("truncated %v with %d problems", d.ProblemsTruncated, len(d.Problems))
	}
}

func TestHealthDescriptionSaysDashboardIDAnswersHealthyDashboards(t *testing.T) {
	for _, want := range []string{"dashboard_id reads one dashboard whether or not it has a problem", "empty problems", "unchecked"} {
		if !strings.Contains(descListDashboardHealth, want) {
			t.Errorf("description does not say %q", want)
		}
	}
	if !strings.Contains(schemaListDashboardHealth, "answered even when it has no problem") {
		t.Error("the dashboard_id schema does not say a healthy dashboard is answered")
	}
}

func TestListedHealthCallsDashboardsUnhealthyOnlyWhenTheListFiltersToProblems(t *testing.T) {
	id := []pgtype.UUID{{Valid: true}}
	cases := []struct {
		name string
		p    health.ListParams
		want string
	}{
		{"no filter", health.ListParams{}, "2 unhealthy dashboards"},
		{"a dashboard id", health.ListParams{DashboardIDs: id}, "2 dashboards"},
		{"a dashboard id and a kind", health.ListParams{DashboardIDs: id, Kind: health.KindNoData}, "2 unhealthy dashboards"},
	}
	for _, c := range cases {
		if got := listedHealth(2, c.p); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestHealthDescriptionNamesTheRanking(t *testing.T) {
	for _, want := range []string{"Failing queries or variables come first, then no data, duplicate titles and unopened dashboards", "the most recently opened first", "summary", "latest repair"} {
		if !strings.Contains(descListDashboardHealth, want) {
			t.Errorf("description does not say %q", want)
		}
	}
}

func TestHealthItemOfCarriesTheLatestRepair(t *testing.T) {
	version, failure := 4, strings.Repeat("e", 2*maxProblemTextBytes)
	cases := []struct {
		repair *health.Repair
		want   *healthRepairOut
	}{
		{nil, nil},
		{&health.Repair{RequestID: "r1", State: "queued"}, &healthRepairOut{State: "queued"}},
		{&health.Repair{RequestID: "r2", State: "succeeded", Version: &version}, &healthRepairOut{State: "succeeded", Version: &version}},
		{&health.Repair{RequestID: "r3", State: "failed", Error: &failure}, &healthRepairOut{State: "failed", Error: clip(failure, maxProblemTextBytes)}},
	}
	for _, c := range cases {
		got := healthItemOf(health.Item{ID: "d", Repair: c.repair}).Repair
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(c.want)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("repair %+v = %s, want %s", c.repair, gotJSON, wantJSON)
		}
	}
}

func TestHealthPageKeepsTheSummaryWhenItTrims(t *testing.T) {
	summary := &health.Summary{Dashboards: 9, Failing: 2, QueryFailed: 1, VariableFailed: 1, NoData: 3, DuplicateTitles: 2, Unopened: 2}
	out, _, retry := healthPage(health.Page{Items: []health.Item{bigHealthItem("a", 100)}, Summary: summary})
	if retry || jsonSize(out) > listBudget || !out.Dashboards[0].ProblemsTruncated {
		t.Fatalf("retry %v, %d bytes, truncated %v", retry, jsonSize(out), out.Dashboards[0].ProblemsTruncated)
	}
	if out.Summary == nil || *out.Summary != healthSummaryOut(*summary) {
		t.Errorf("summary = %+v, want %+v", out.Summary, summary)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), `"summary":{"dashboards":9,"failing":2,"query_failed":1,"variable_failed":1,"no_data":3,"duplicate_titles":2,"unopened":2}`) {
		t.Errorf("first page JSON has no summary: %.200s", raw)
	}
	later, _, _ := healthPage(health.Page{Items: []health.Item{bigHealthItem("b", 1)}})
	if raw, _ := json.Marshal(later); strings.Contains(string(raw), "summary") {
		t.Errorf("later page carries a summary: %.200s", raw)
	}
}
