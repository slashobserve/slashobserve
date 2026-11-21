package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const savedConfig = `{"uid":"metrics","type":"prometheus","name":"Metrics","audience":"admins","source":"app","address":"https://prom.example.com","username":"","secrets_set":["bearer_token","header:X-Org"],"created_at":"2026-11-19T10:00:00Z","updated_at":"2026-11-19T10:00:00Z"}`

type datasourceAdminFake struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
	status   int
	reply    string
}

func newDatasourceAdminFake(t *testing.T) *datasourceAdminFake {
	t.Helper()
	f := &datasourceAdminFake{status: http.StatusOK, reply: savedConfig}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, recordedRequest{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, body: string(data)})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		if f.status != http.StatusNoContent {
			_, _ = io.WriteString(w, f.reply)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *datasourceAdminFake) run(stdin string, args ...string) (string, string, int) {
	var stdout, stderr strings.Builder
	code := Main(append([]string{"--server", f.server.URL, "--token", "k"}, args...), IO{In: strings.NewReader(stdin), Out: &stdout, Err: &stderr}, "test")
	return stdout.String(), stderr.String(), code
}

func (f *datasourceAdminFake) last() recordedRequest { return f.requests[len(f.requests)-1] }

func credentialFile(t *testing.T, value string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDatasourcesAddSendsCredentialsReadFromFilesAndStandardInput(t *testing.T) {
	f := newDatasourceAdminFake(t)
	f.status = http.StatusCreated
	token := credentialFile(t, "token-1\n")

	stdout, stderr, code := f.run("org-7\n", "datasources", "add", "metrics", "--type", "prometheus", "--address", "https://prom.example.com",
		"--bearer-token-file", token, "--header", "X-Org=-", "--max-rows", "50", "--timeout", "20s", "--grafana-uid", "prom-main")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Added datasource metrics") {
		t.Errorf("output = %q", stdout)
	}
	req := f.last()
	if req.method != http.MethodPost || req.path != "/api/v1/datasources" {
		t.Fatalf("request = %+v", req)
	}
	var body struct {
		UID, Type, Address string
		GrafanaUIDs        []string `json:"grafana_uids"`
		Limits             map[string]any
		Secrets            struct {
			BearerToken string            `json:"bearer_token"`
			Headers     map[string]string `json:"headers"`
		}
	}
	if err := json.Unmarshal([]byte(req.body), &body); err != nil {
		t.Fatal(err)
	}
	if body.UID != "metrics" || body.Type != "prometheus" || body.Secrets.BearerToken != "token-1" || body.Secrets.Headers["X-Org"] != "org-7" ||
		body.Limits["max_rows"] != float64(50) || body.Limits["timeout"] != "20s" || len(body.GrafanaUIDs) != 1 {
		t.Fatalf("body = %s", req.body)
	}
}

func TestDatasourcesAddNeedsATypeAndAnAddress(t *testing.T) {
	f := newDatasourceAdminFake(t)
	if _, stderr, code := f.run("", "datasources", "add", "metrics", "--type", "prometheus"); code == 0 || !strings.Contains(stderr, "--address") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if len(f.requests) != 0 {
		t.Fatal("a request went out without the required flags")
	}
}

func TestDatasourcesAddRefusesTwoCredentialsFromStandardInput(t *testing.T) {
	f := newDatasourceAdminFake(t)
	_, stderr, code := f.run("x", "datasources", "add", "metrics", "--type", "prometheus", "--address", "https://p.example.com", "--password-file", "-", "--bearer-token-file", "-")
	if code == 0 || !strings.Contains(stderr, "standard input") || len(f.requests) != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestDatasourcesEditSendsOnlyWhatChangedAndTheClearList(t *testing.T) {
	f := newDatasourceAdminFake(t)

	if _, stderr, code := f.run("", "datasources", "edit", "metrics", "--address", "https://prom2.example.com", "--clear", "header:X-Org"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	req := f.last()
	if req.method != http.MethodPatch || req.path != "/api/v1/datasources/metrics" {
		t.Fatalf("request = %+v", req)
	}
	if req.body != `{"address":"https://prom2.example.com","clear_secrets":["header:X-Org"]}` {
		t.Fatalf("body = %s", req.body)
	}
	if _, stderr, code := f.run("", "datasources", "edit", "metrics"); code == 0 || !strings.Contains(stderr, "at least one") {
		t.Fatalf("edit without changes: exit %d: %s", code, stderr)
	}
}

func TestDatasourcesGetPrintsWhichCredentialsAreSetNeverTheirValues(t *testing.T) {
	f := newDatasourceAdminFake(t)
	stdout, stderr, code := f.run("", "datasources", "get", "metrics")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"https://prom.example.com", "bearer_token, header:X-Org"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
	if req := f.last(); req.method != http.MethodGet || req.path != "/api/v1/datasources/metrics/config" {
		t.Fatalf("request = %+v", req)
	}
}

func TestDatasourcesTestExitsOneWhenTheCheckFails(t *testing.T) {
	f := newDatasourceAdminFake(t)
	f.reply = `{"healthy":false,"message":"The datasource is not answering","detail":"The server could not connect to the datasource's address"}`

	stdout, _, code := f.run("", "datasources", "test", "metrics", "--saved", "--address", "https://prom2.example.com")
	if code != 1 || !strings.Contains(stdout, "The server could not connect") {
		t.Fatalf("exit %d, output %q", code, stdout)
	}
	if req := f.last(); req.path != "/api/v1/datasources/test" || req.query != "base=metrics" {
		t.Fatalf("request = %+v", req)
	}

	f.reply = `{"healthy":true}`
	if _, _, code := f.run("", "datasources", "test", "metrics", "--type", "prometheus", "--address", "https://p.example.com"); code != 0 {
		t.Fatalf("exit %d for a healthy draft", code)
	}
	if req := f.last(); req.query != "" {
		t.Fatalf("a draft test sent the query %q", req.query)
	}
}

func TestDatasourcesRemoveListsWhatUsesTheDatasource(t *testing.T) {
	f := newDatasourceAdminFake(t)
	f.status = http.StatusConflict
	f.reply = `{"error":"This datasource is used by dashboards or saved searches; change them to another datasource first.","code":"datasource_in_use","used_by":[{"kind":"dashboard","id":"6f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11","title":"Checkout"}],"hidden":2}`

	_, stderr, code := f.run("", "datasources", "remove", "metrics")
	if code == 0 {
		t.Fatal("a refused removal exited 0")
	}
	for _, want := range []string{"datasource_in_use", "dashboard 6f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11 Checkout", "2 more"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}

	f.status = http.StatusNoContent
	stdout, stderr, code := f.run("", "datasources", "remove", "metrics")
	if code != 0 || !strings.Contains(stdout+stderr, "Removed datasource metrics") {
		t.Fatalf("exit %d: %s %s", code, stdout, stderr)
	}
	if req := f.last(); req.method != http.MethodDelete || req.path != "/api/v1/datasources/metrics" {
		t.Fatalf("request = %+v", req)
	}
}

func TestDatasourcesEditSendsTheAudienceAndGetPrintsIt(t *testing.T) {
	f := newDatasourceAdminFake(t)

	if _, stderr, code := f.run("", "datasources", "edit", "metrics", "--audience", "admins"); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if req := f.last(); req.method != http.MethodPatch || req.body != `{"audience":"admins"}` {
		t.Fatalf("request = %+v", req)
	}
	stdout, stderr, code := f.run("", "datasources", "get", "metrics")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "admins") {
		t.Errorf("output lacks the audience:\n%s", stdout)
	}
}

func TestDatasourcesEditPrintsTheContentThatBlocksMakingItAdminsOnly(t *testing.T) {
	f := newDatasourceAdminFake(t)
	f.status = http.StatusConflict
	f.reply = `{"error":"move them first","code":"restricted_content","datasource":"metrics","unlisted":7,"hidden":2,"items":[` +
		`{"kind":"saved_search","id":"s1","title":"Errors","folder_id":"f1","folder_title":"Team"},` +
		`{"kind":"dashboard","id":"d1","title":"Checkout","folder_id":"f2","folder_title":"General"}]}`

	_, stderr, code := f.run("", "datasources", "edit", "metrics", "--audience", "admins")
	for _, want := range []string{
		"move them first (restricted_content)",
		`saved search s1 "Errors" in folder "Team"`,
		`dashboard d1 "Checkout" in folder "General"`,
		"and 7 more not listed",
		"and 2 in people's My Dashboards",
	} {
		if code != 1 || !strings.Contains(stderr, want) {
			t.Fatalf("exit %d, stderr lacks %q:\n%s", code, want, stderr)
		}
	}
}
