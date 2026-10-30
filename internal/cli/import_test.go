package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseMappings(t *testing.T) {
	cases := []struct {
		name    string
		values  []string
		want    map[string]string
		wantErr string
	}{
		{name: "none", values: nil, want: nil},
		{name: "one pair", values: []string{"prometheus=prom"}, want: map[string]string{"prometheus": "prom"}},
		{name: "splits at the last equals sign", values: []string{"a=b=c"}, want: map[string]string{"a=b": "c"}},
		{name: "commas stay in the key", values: []string{"a,b=c"}, want: map[string]string{"a,b": "c"}},
		{name: "two keys", values: []string{"a=1", "b=2"}, want: map[string]string{"a": "1", "b": "2"}},
		{name: "no equals sign", values: []string{"nope"}, wantErr: `--map wants <grafana datasource>=<our uid>, got "nope"`},
		{name: "empty key", values: []string{"=x"}, wantErr: `--map wants <grafana datasource>=<our uid>, got "=x"`},
		{name: "empty uid", values: []string{"x="}, wantErr: `--map wants <grafana datasource>=<our uid>, got "x="`},
		{name: "repeated key", values: []string{"a=1", "a=2"}, wantErr: `--map names "a" twice`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMappings(tc.values)
			if tc.wantErr != "" {
				var usage usageError
				if !errors.As(err, &usage) || usage.message != tc.wantErr {
					t.Fatalf("error %v, want usage error %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for key, uid := range tc.want {
				if got[key] != uid {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestReadImportSource(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, size int) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(strings.Repeat("a", size)), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	exact := write("exact.json", maxImportBytes)
	over := write("over.json", maxImportBytes+1)
	empty := write("empty.json", 0)
	small := write("small.json", 3)

	cases := []struct {
		name    string
		arg     string
		stdin   string
		wantLen int
		wantErr string
	}{
		{name: "a file", arg: small, wantLen: 3},
		{name: "a file at the limit", arg: exact, wantLen: maxImportBytes},
		{name: "a file over the limit", arg: over, wantErr: over + " is larger than 8 MiB, the import limit"},
		{name: "an empty file", arg: empty, wantErr: empty + " is empty"},
		{name: "a missing file", arg: filepath.Join(dir, "missing.json"), wantErr: "read " + filepath.Join(dir, "missing.json") + ": "},
		{name: "a directory", arg: dir, wantErr: "read " + dir + ": "},
		{name: "standard input", arg: "-", stdin: "{}", wantLen: 2},
		{name: "empty standard input", arg: "-", wantErr: "standard input is empty"},
		{name: "standard input over the limit", arg: "-", stdin: strings.Repeat("a", maxImportBytes+1), wantErr: "standard input is larger than 8 MiB, the import limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readImportSource(strings.NewReader(tc.stdin), tc.arg)
			if tc.wantErr != "" {
				if err == nil || !strings.HasPrefix(err.Error(), tc.wantErr) {
					t.Fatalf("error %v, want prefix %q", err, tc.wantErr)
				}
				var usage usageError
				if errors.As(err, &usage) {
					t.Fatalf("a read problem is exit 1, got a usage error %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("read %d bytes, want %d", len(got), tc.wantLen)
			}
		})
	}
}

func TestShortReason(t *testing.T) {
	long := strings.Repeat("é", reasonRunes+1)
	cases := []struct{ name, in, want string }{
		{name: "short", in: "plain", want: "plain"},
		{name: "exactly the limit", in: strings.Repeat("é", reasonRunes), want: strings.Repeat("é", reasonRunes)},
		{name: "over the limit", in: long, want: strings.Repeat("é", reasonRunes) + "..."},
		{name: "control characters", in: "a\nb\u202ec", want: "a b c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortReason(tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReportAttentionTable(t *testing.T) {
	all := importReport{Panels: []importPanelLine{{Title: "ok", SourceType: "stat", Status: "imported"}}}
	if got := all.attention(); got != "" {
		t.Fatalf("a fully imported report has no table, got %q", got)
	}

	report := importReport{Panels: []importPanelLine{
		{Title: "ok", SourceType: "stat", Status: "imported"},
		{Title: "Requests by pod", SourceType: "heatmap", Status: "placeholder", Reasons: []string{"first", "second"}},
		{Title: "", SourceType: "graph", Status: "unverified"},
		{Title: "Latency", SourceType: "timeseries", Status: "partial", Reasons: []string{"bad\u202eoption"}},
	}}
	lines := strings.Split(report.attention(), "\n")
	if len(lines) != 4 {
		t.Fatalf("want a header and three rows, got %q", lines)
	}
	want := [][]string{
		{"STATUS", "PANEL", "TYPE", "REASON"},
		{"placeholder", "Requests", "by", "pod", "heatmap", "first"},
		{"unverified", "-", "graph", "-"},
		{"partial", "Latency", "timeseries", "bad", "option"},
	}
	for i, fields := range want {
		if got := strings.Fields(lines[i]); strings.Join(got, " ") != strings.Join(fields, " ") {
			t.Fatalf("line %d is %q, want %q", i, got, fields)
		}
	}
}

func TestCountsLine(t *testing.T) {
	got := importCounts{Total: 9, Imported: 4, Partial: 3, Placeholder: 1, Unverified: 1}.line()
	want := "9 panels, 4 imported, 3 partial, 1 placeholders, 1 unverified"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPreviewFields(t *testing.T) {
	preview := importPreview{Summary: importSummary{Format: "export", Title: "Shop", SchemaVersion: 39, Panels: 5, Rows: 1, Variables: 2}}
	flat := func(p importPreview) string {
		var parts []string
		for _, pair := range previewFields(p) {
			parts = append(parts, pair[0]+"="+pair[1])
		}
		return strings.Join(parts, ";")
	}
	want := "Title=Shop;Format=export;Schema version=39;Panels=5;Rows=1;Variables=2;Ready=no"
	if got := flat(preview); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	preview.Ready = true
	if got := flat(preview); !strings.HasSuffix(got, "Ready=yes") {
		t.Fatalf("got %q", got)
	}
}

func TestDatasourcesTableShowsDashesForNulls(t *testing.T) {
	prom, uid := "prometheus", "prom"
	table := datasourcesTable([]importDatasource{
		{Key: "a", Type: &prom, Panels: 2, Match: &uid, MatchedBy: &prom},
		{Key: "b", Panels: 1},
	})
	lines := strings.Split(table, "\n")
	if len(lines) != 3 {
		t.Fatalf("want a header and two rows, got %q", lines)
	}
	for i, want := range []string{"GRAFANA TYPE PANELS MATCH BY", "a prometheus 2 prom prometheus", "b - 1 - -"} {
		if got := strings.Join(strings.Fields(lines[i]), " "); got != want {
			t.Fatalf("line %d is %q, want %q", i, got, want)
		}
	}
}

func TestUnresolvedFrom(t *testing.T) {
	body := []byte(`{"error":"datasources need a match","code":"datasources_unresolved","datasources":[` +
		`{"key":"zzz","type":"prometheus","panels":3,"match":null,"matched_by":null,"candidates":["prom","prom_tight"]},` +
		`{"key":"ok","type":"prometheus","panels":1,"match":"prom","matched_by":"only","candidates":["prom"]},` +
		`{"key":"lonely","type":null,"panels":1,"match":null,"matched_by":null,"candidates":[]}]}`)
	err := unresolvedFrom(decodeAPIError(422, body))
	var unresolved *unresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("want an unresolvedError, got %T %v", err, err)
	}
	message := err.Error()
	for _, want := range []string{
		"datasources need a match (datasources_unresolved)\n",
		"Map each one with --map '<grafana>=<uid>'; candidates: zzz: prom, prom_tight\n",
		"Map each one with --map '<grafana>=<uid>'; candidates: lonely: -",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message lacks %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "candidates: ok") {
		t.Fatalf("a matched datasource needs no hint:\n%s", message)
	}
	if !strings.HasPrefix(message, "datasources need a match") {
		t.Fatalf("the error comes first:\n%s", message)
	}
}

func TestUnresolvedFromLeavesOtherErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "another 422", err: decodeAPIError(422, []byte(`{"error":"bad","code":"invalid_json"}`))},
		{name: "another status", err: decodeAPIError(500, []byte(`{"error":"x","code":"datasources_unresolved"}`))},
		{name: "not an API error", err: errors.New("boom")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unresolvedFrom(tc.err); !errors.Is(got, tc.err) {
				t.Fatalf("got %v, want the error unchanged", got)
			}
		})
	}
	unreadable := decodeAPIError(422, []byte(`{"error":"x","code":"datasources_unresolved","datasources":"nope"}`))
	if got := unresolvedFrom(unreadable); !errors.Is(got, unreadable) {
		t.Fatalf("an unreadable body keeps the plain error, got %v", got)
	}
}

func TestWithTimeoutCopiesTheClient(t *testing.T) {
	original := &client{http: &http.Client{Timeout: requestTimeout}}
	longer := original.withTimeout(importTimeout)
	if longer.http.Timeout != importTimeout || original.http.Timeout != requestTimeout {
		t.Fatalf("timeouts: copy %s, original %s", longer.http.Timeout, original.http.Timeout)
	}
	if importTimeout != 120*time.Second {
		t.Fatalf("import timeout is %s", importTimeout)
	}
}

type importServer struct {
	path   string
	posted string
	reply  string
}

func (s *importServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.path, s.posted = r.URL.Path, string(body)
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/api/v1/imports" {
		w.WriteHeader(http.StatusCreated)
	}
	_, _ = fmt.Fprint(w, s.reply)
}

func runImportCommand(t *testing.T, s *importServer, args ...string) (int, string, string) {
	t.Helper()
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	file := filepath.Join(t.TempDir(), "dashboard.json")
	if err := os.WriteFile(file, []byte(`{"title":"Shop"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	full := append([]string{"--server", server.URL, "--token", "k", "import"}, args...)
	full = append(full, file)
	code := Main(full, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return code, stdout.String(), stderr.String()
}

const importedReply = `{"dashboard":{"id":"d1","folder":{"id":"f1","title":"Payments","general":false}},"version":{},` +
	`"import":{"title":"Shop","report":{"counts":{"total":1,"imported":1},"panels":[]}},"folder_note":null}`

const fallbackReply = `{"dashboard":{"id":"d1","folder":{"id":"f2","title":"General","general":true}},"version":{},` +
	`"import":{"title":"Shop","report":{"counts":{"total":1,"imported":1},"panels":[]}},` +
	`"folder_note":"Folder \"Secret\" is not available to you, so the dashboard went to \"General\"."}`

func TestImportSendsTheFolderAndShowsWhereTheDashboardWent(t *testing.T) {
	s := &importServer{reply: importedReply}
	code, stdout, stderr := runImportCommand(t, s, "--folder", "Payments")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if s.path != "/api/v1/imports" || s.posted != `{"json":"{\"title\":\"Shop\"}","folder":"Payments"}` {
		t.Fatalf("posted %s to %s", s.posted, s.path)
	}
	if !strings.HasSuffix(strings.TrimSpace(stdout), "/dashboards/d1") {
		t.Fatalf("stdout = %q, want the dashboard address alone", stdout)
	}
	if !strings.Contains(stderr, "\nFolder: Payments\n") || strings.Contains(stderr, "not available") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestImportShowsTheFolderNote(t *testing.T) {
	s := &importServer{reply: fallbackReply}
	code, _, stderr := runImportCommand(t, s, "--folder", "Secret")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := "Folder: General\nFolder \"Secret\" is not available to you, so the dashboard went to \"General\".\n"
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr = %q, want it to contain %q", stderr, want)
	}
}

func TestQuietImportHidesTheFolderLineButKeepsTheFolderNote(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  string
	}{
		{"no note", importedReply, ""},
		{"a note", fallbackReply, "Folder \"Secret\" is not available to you, so the dashboard went to \"General\".\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &importServer{reply: tt.reply}
			code, stdout, stderr := runImportCommand(t, s, "-q", "--folder", "Secret")
			if code != 0 {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			if stderr != tt.want {
				t.Fatalf("stderr = %q, want %q", stderr, tt.want)
			}
			if !strings.HasSuffix(strings.TrimSpace(stdout), "/dashboards/d1") {
				t.Fatalf("stdout = %q, want the dashboard address", stdout)
			}
		})
	}
}

func TestImportWithoutAFolderFlagSendsNoFolder(t *testing.T) {
	s := &importServer{reply: importedReply}
	if code, _, stderr := runImportCommand(t, s); code != 0 || strings.Contains(s.posted, "folder") {
		t.Fatalf("exit %d, posted %s, stderr %s", code, s.posted, stderr)
	}
}

func TestPreviewDoesNotSendTheFolder(t *testing.T) {
	s := &importServer{reply: `{"summary":{},"datasources":[],"ready":true,"report":{"counts":{},"panels":[]}}`}
	if code, _, stderr := runImportCommand(t, s, "--preview", "--folder", "Payments"); code != 0 || s.path != "/api/v1/imports/preview" || strings.Contains(s.posted, "folder") {
		t.Fatalf("exit %d, posted %s to %s, stderr %s", code, s.posted, s.path, stderr)
	}
}
