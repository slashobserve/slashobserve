package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	exportFolder = "aaaaaaaa-0000-4000-8000-000000000001"
	exportA      = "aaaaaaaa-0000-4000-8000-00000000000a"
	exportB      = "aaaaaaaa-0000-4000-8000-00000000000b"
	exportC      = "aaaaaaaa-0000-4000-8000-00000000000c"
	exportD      = "aaaaaaaa-0000-4000-8000-00000000000d"
)

func TestExportNamesKeepBoundNamesAndNumberSlugCollisions(t *testing.T) {
	cases := []struct {
		name  string
		items []exportItem
		bound map[string]string
		want  map[string]string
	}{
		{
			name:  "titles become slugs",
			items: []exportItem{{exportA, "Checkout Health"}, {exportB, "API / Latency"}},
			want:  map[string]string{exportA: "checkout-health.json", exportB: "api-latency.json"},
		},
		{
			name:  "colliding slugs are numbered in id order",
			items: []exportItem{{exportC, "checkout"}, {exportA, "Checkout"}, {exportB, "CHECKOUT!"}},
			want:  map[string]string{exportA: "checkout.json", exportB: "checkout-2.json", exportC: "checkout-3.json"},
		},
		{
			name:  "a bound name wins over a slug that would take it",
			items: []exportItem{{exportA, "Checkout"}, {exportB, "Renamed"}},
			bound: map[string]string{exportB: "checkout.json"},
			want:  map[string]string{exportA: "checkout-2.json", exportB: "checkout.json"},
		},
		{
			name:  "two bound names that collide without case",
			items: []exportItem{{exportA, "One"}, {exportB, "Two"}},
			bound: map[string]string{exportA: "Board.json", exportB: "board.json"},
			want:  map[string]string{exportA: "Board.json", exportB: "board-2.json"},
		},
		{
			name:  "a title with no letters or digits",
			items: []exportItem{{exportA, "!!!"}, {exportB, "日本"}},
			want:  map[string]string{exportA: "dashboard.json", exportB: "dashboard-2.json"},
		},
		{
			name:  "a numbered slug already taken by a title",
			items: []exportItem{{exportA, "a"}, {exportB, "a 2"}, {exportC, "a"}},
			want:  map[string]string{exportA: "a.json", exportB: "a-2.json", exportC: "a-3.json"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := exportNames(c.items, c.bound); !maps.Equal(got, c.want) {
				t.Errorf("exportNames = %v, want %v", got, c.want)
			}
		})
	}
}

func TestBoundNamesTakeTheFileNameOfEachBinding(t *testing.T) {
	var rows appliedRows
	if err := json.Unmarshal([]byte(`{"applied":[{"path":"Shop/checkout.json","dashboard_id":"`+exportA+`"},{"path":"root.json","dashboard_id":"`+exportB+`"}]}`), &rows); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{exportA: "checkout.json", exportB: "root.json"}
	if got := boundNames(rows); !maps.Equal(got, want) {
		t.Errorf("boundNames = %v, want %v", got, want)
	}
}

func TestPickFolderByIDOrTitleWithoutCase(t *testing.T) {
	folders := []folderRow{{ID: exportFolder, Title: "Payments"}, {ID: exportA, Title: "Team / Ops"}, {ID: exportB, Title: "twin"}, {ID: exportC, Title: "Twin"}}
	cases := []struct {
		arg, want string
		fails     bool
	}{
		{exportFolder, exportFolder, false},
		{"payments", exportFolder, false},
		{"Team / Ops", exportA, false},
		{"twin", "", true},
		{"missing", "", true},
	}
	for _, c := range cases {
		got, err := pickFolder(folders, c.arg)
		if (err != nil) != c.fails || got.ID != c.want {
			t.Errorf("pickFolder(%q) = %+v, %v", c.arg, got, err)
		}
	}
}

func TestDashboardsExportWritesOnePrettyFilePerDashboard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/folders":
			_, _ = fmt.Fprintf(w, `{"folders":[{"id":%q,"title":"Team / Payments","general":false,"personal":false}]}`, exportFolder)
		case "/api/v1/dashboards":
			if r.URL.Query().Get("folder") != exportFolder {
				t.Errorf("listed folder %q", r.URL.Query().Get("folder"))
			}
			_, _ = fmt.Fprintf(w, `{"dashboards":[{"id":%q,"title":"Checkout"},{"id":%q,"title":"Renamed"}],"next_cursor":null}`, exportA, exportB)
		case "/api/v1/provision/applied":
			_, _ = fmt.Fprintf(w, `{"applied":[{"path":"Old/kept.json","dashboard_id":%q}]}`, exportB)
		case "/api/v1/dashboards/" + exportA, "/api/v1/dashboards/" + exportB:
			_, _ = fmt.Fprintf(w, `{"dashboard":{"id":"x"},"version":{"version":1,"spec":{"specVersion":"0","title":"T","panels":[]}}}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := Main([]string{"--server", server.URL, "--token", "k", "dashboards", "export", "team / payments", "--dir", dir}, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	for _, name := range []string{"checkout.json", "kept.json"} {
		raw, err := os.ReadFile(filepath.Join(dir, "Team %2F Payments", name)) //nolint:gosec // a test file under t.TempDir
		if err != nil {
			t.Fatal(err)
		}
		want := "{\n  \"specVersion\": \"0\",\n  \"title\": \"T\",\n  \"panels\": []\n}\n"
		if string(raw) != want {
			t.Errorf("%s = %q, want %q", name, raw, want)
		}
	}
	if !strings.Contains(stdout.String(), "Team %2F Payments/kept.json") {
		t.Errorf("stdout %q lacks the kept file", stdout.String())
	}
}

func TestDashboardsApplyPostsEachFileAndFailsWhenOneFails(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"root.json":                   `{"title":"Root"}`,
		"Team %2F Payments/api.json":  `{"title":"API"}`,
		"Team %2F Payments/twin.json": `{"title":"Twin"}`,
		"broken.json":                 `{not json`,
		".git/ignored.json":           `{}`,
		"notes.md":                    `# not a spec`,
	}
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var posted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req applyRequest
		if r.URL.Path != "/api/v1/provision/apply" || json.Unmarshal(body, &req) != nil {
			t.Errorf("unexpected request %s %s", r.URL.Path, body)
			return
		}
		posted = append(posted, req.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(req.Path, "twin.json") {
			w.WriteHeader(http.StatusConflict)
			_, _ = fmt.Fprint(w, `{"error":"more than one dashboard in the folder has this title","code":"ambiguous"}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"path":%q,"outcome":"created","dashboard_id":%q,"version":1}`, req.Path, exportA)
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := Main([]string{"--server", server.URL, "--token", "k", "--json", "dashboards", "apply", "--dir", dir}, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	if code != 1 {
		t.Fatalf("exit %d, want 1 (stderr %q)", code, stderr.String())
	}
	wantPosted := []string{"Team %2F Payments/api.json", "Team %2F Payments/twin.json", "root.json"}
	if fmt.Sprint(posted) != fmt.Sprint(wantPosted) {
		t.Errorf("posted %v, want %v", posted, wantPosted)
	}
	var results []applied
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("stdout %q: %v", stdout.String(), err)
	}
	outcomes := map[string]string{}
	for _, r := range results {
		outcomes[r.Path] = r.Outcome + " " + r.Code
	}
	want := map[string]string{
		"Team %2F Payments/api.json":  "created ",
		"Team %2F Payments/twin.json": "failed ambiguous",
		"broken.json":                 "failed invalid_json",
		"root.json":                   "created ",
	}
	if !maps.Equal(outcomes, want) {
		t.Errorf("outcomes = %v, want %v", outcomes, want)
	}
	if !strings.Contains(stderr.String(), "2 of 4 files failed") {
		t.Errorf("stderr %q", stderr.String())
	}
}
