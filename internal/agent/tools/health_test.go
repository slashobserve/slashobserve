package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/health"
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
	cases := []struct {
		name    string
		in      listHealthInput
		message string
	}{
		{"limit 0", listHealthInput{Limit: ptr(0)}, "limit must be between 1 and 50"},
		{"limit 51", listHealthInput{Limit: ptr(51)}, "limit must be between 1 and 50"},
		{"folder not a uuid", listHealthInput{FolderID: "shop"}, "folder_id must be a UUID from list_folders"},
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
	if strings.Join(kindStrings(schema.Properties.Kind.Enum), ",") != strings.Join(kindStrings(health.Kinds), ",") {
		t.Errorf("kind enum = %v, want %v", schema.Properties.Kind.Enum, health.Kinds)
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
	bare := healthItemOf(health.Item{ID: "d1", Title: "Shop", URL: "https://observe.example.com/d/d1", Folder: folders.Folder{ID: folder, Title: "General"}, Problems: []health.Problem{{Kind: health.KindUnopened}}})
	raw, err := json.Marshal(bare)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"dashboard_id":"d1","title":"Shop","url":"https://observe.example.com/d/d1","folder_id":"` + folder.String() + `","folder_title":"General","problems":[{"kind":"unopened"}]}`
	if string(raw) != want {
		t.Errorf("bare item = %s\nwant %s", raw, want)
	}

	checked := time.Date(2026, 11, 6, 9, 30, 0, 0, time.FixedZone("IST", 19800))
	version := 3
	full := healthItemOf(health.Item{
		ID: "d2", Folder: folders.Folder{ID: folder}, CheckedAt: &checked, CheckedVersion: &version, Skipped: health.SkippedDependsOnViewer,
		Problems: []health.Problem{{Kind: health.KindQueryFailed, PanelID: "p1", PanelTitle: "Errors", Code: "query_failed", Message: strings.Repeat("x", 2*maxProblemTextBytes)}},
	})
	if full.CheckedAt != "2026-11-06T04:00:00Z" || full.CheckedVersion == nil || *full.CheckedVersion != 3 || full.Skipped != health.SkippedDependsOnViewer {
		t.Errorf("checked fields = %q %v %q", full.CheckedAt, full.CheckedVersion, full.Skipped)
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
