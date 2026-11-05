package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type doctorFake struct {
	server      *httptest.Server
	ready       int
	readyBody   string
	me          int
	role        string
	version     string
	datasources string
	agent       string
	paths       []string
}

func newDoctorFake(t *testing.T) *doctorFake {
	t.Helper()
	f := &doctorFake{
		ready:       http.StatusOK,
		readyBody:   `{"status":"ok","checks":{"database":"ok"}}`,
		me:          http.StatusOK,
		role:        "editor",
		version:     "v1.4.0",
		datasources: `{"datasources":[{"uid":"prom","type":"prometheus","name":"Prometheus","healthy":true,"checked_at":"2026-11-01T10:00:00Z","dashboard_count":3}]}`,
		agent:       `{"available":true,"provider":"anthropic","model":"claude-haiku-5-5","effort":"low"}`,
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.paths = append(f.paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/readyz":
			if r.Header.Get("Authorization") != "" {
				t.Errorf("readyz got a key")
			}
			w.WriteHeader(f.ready)
			_, _ = io.WriteString(w, f.readyBody)
		case "/api/v1/me":
			w.WriteHeader(f.me)
			_, _ = io.WriteString(w, `{"kind":"service_account","id":"x","name":"ci-deploy","role":"`+f.role+`"}`)
		case "/api/v1/instance":
			_, _ = io.WriteString(w, `{"version":"`+f.version+`"}`)
		case "/api/v1/datasources":
			_, _ = io.WriteString(w, f.datasources)
		case "/api/v1/agent":
			_, _ = io.WriteString(w, f.agent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *doctorFake) run(version string, args ...string) (string, string, int) {
	var stdout, stderr strings.Builder
	code := Main(append([]string{"--server", f.server.URL, "--token", "k", "doctor"}, args...), IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, version)
	return stdout.String(), stderr.String(), code
}

func statusOf(t *testing.T, stdout, check string) string {
	t.Helper()
	for line := range strings.SplitSeq(stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == check {
			return fields[1]
		}
	}
	t.Fatalf("no %s row in %q", check, stdout)
	return ""
}

func TestDoctorAllHealthyExitsZero(t *testing.T) {
	f := newDoctorFake(t)
	stdout, stderr, code := f.run("v1.4.0")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	if !strings.HasPrefix(stdout, "CHECK") {
		t.Fatalf("stdout %q", stdout)
	}
	for _, check := range []string{"server", "key", "role", "version", "datasource", "model"} {
		if got := statusOf(t, stdout, check); got != "ok" {
			t.Errorf("%s is %s", check, got)
		}
	}
	for _, want := range []string{"is ready", "ci-deploy (service_account)", "prom (prometheus)", "anthropic claude-haiku-5-5, effort low"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q lacks %q", stdout, want)
		}
	}
}

func TestDoctorUnhealthyDatasourceFails(t *testing.T) {
	f := newDoctorFake(t)
	f.datasources = `{"datasources":[{"uid":"prom","type":"prometheus","name":"P","healthy":true,"checked_at":"x","dashboard_count":0},{"uid":"logs","type":"loki","name":"L","healthy":false,"message":"connection refused","checked_at":"x","dashboard_count":0}]}`
	stdout, stderr, code := f.run("v1.4.0")
	if code != 1 || !strings.Contains(stdout, "logs (loki): connection refused") || !strings.Contains(stderr, "1 check failed") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestDoctorNoDatasourcesWarns(t *testing.T) {
	f := newDoctorFake(t)
	f.datasources = `{"datasources":[]}`
	stdout, _, code := f.run("v1.4.0")
	if code != 0 || statusOf(t, stdout, "datasource") != "warn" {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestDoctorRefusedKeySkipsTheRest(t *testing.T) {
	f := newDoctorFake(t)
	f.me = http.StatusUnauthorized
	stdout, _, code := f.run("v1.4.0")
	if code != 1 || statusOf(t, stdout, "key") != "fail" || !strings.Contains(stdout, "the server refused the key; run observe login") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
	for _, check := range []string{"role", "version", "datasource", "model"} {
		if got := statusOf(t, stdout, check); got != "skip" {
			t.Errorf("%s is %s", check, got)
		}
	}
	for _, path := range f.paths {
		if path != "/readyz" && path != "/api/v1/me" {
			t.Errorf("called %s after the key failed", path)
		}
	}
}

func TestDoctorClosedServerSkipsEverythingElse(t *testing.T) {
	f := newDoctorFake(t)
	f.server.Close()
	stdout, _, code := f.run("v1.4.0")
	if code != 1 || statusOf(t, stdout, "server") != "fail" || !strings.Contains(stdout, "cannot reach the server") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
	for _, check := range []string{"key", "role", "version", "datasource", "model"} {
		if got := statusOf(t, stdout, check); got != "skip" {
			t.Errorf("%s is %s", check, got)
		}
	}
}

func TestDoctorNotReadyNamesTheFailingChecks(t *testing.T) {
	f := newDoctorFake(t)
	f.ready = http.StatusServiceUnavailable
	f.readyBody = `{"status":"unavailable","checks":{"database":"error","migrations":"ok"}}`
	stdout, _, code := f.run("v1.4.0")
	if code != 1 || !strings.Contains(stdout, "is not ready: database failing") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestDoctorWithoutAModelFails(t *testing.T) {
	f := newDoctorFake(t)
	f.agent = `{"available":false,"provider":null,"model":null,"effort":null}`
	stdout, _, code := f.run("v1.4.0")
	if code != 1 || statusOf(t, stdout, "model") != "fail" || !strings.Contains(stdout, "no model is configured; prompts cannot run") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestDoctorViewerRoleWarns(t *testing.T) {
	f := newDoctorFake(t)
	f.role = "viewer"
	stdout, _, code := f.run("v1.4.0")
	if code != 0 || statusOf(t, stdout, "role") != "warn" {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestDoctorVersionMismatch(t *testing.T) {
	cases := []struct{ cli, server, want string }{
		{"v1.3.0", "v1.4.0", "warn"},
		{"dev", "v1.4.0", "ok"},
		{"v1.4.0", "dev", "ok"},
		{"v1.4.0", "", "ok"},
	}
	for _, tc := range cases {
		f := newDoctorFake(t)
		f.version = tc.server
		stdout, _, code := f.run(tc.cli)
		if code != 0 || statusOf(t, stdout, "version") != tc.want {
			t.Errorf("cli %s, server %q: exit %d, stdout %q", tc.cli, tc.server, code, stdout)
		}
	}
	f := newDoctorFake(t)
	f.version = "v1.4.0"
	stdout, _, _ := f.run("v1.3.0")
	if !strings.Contains(stdout, "the CLI is v1.3.0 and the server v1.4.0") {
		t.Errorf("stdout %q", stdout)
	}
}

func TestDoctorJSON(t *testing.T) {
	f := newDoctorFake(t)
	f.agent = `{"available":false,"provider":null,"model":null,"effort":null}`
	stdout, _, code := f.run("v1.4.0", "--json")
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	var report struct {
		OK     bool `json:"ok"`
		Checks []struct {
			Check  string `json:"check"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout %q: %v", stdout, err)
	}
	if report.OK || len(report.Checks) != 6 || report.Checks[0].Check != "server" || report.Checks[5].Check != "model" || report.Checks[5].Status != "fail" {
		t.Fatalf("report %+v", report)
	}
}

func TestDoctorWithoutAKeyFailsTheKeyCheck(t *testing.T) {
	f := newDoctorFake(t)
	t.Setenv("OBSERVE_TOKEN", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr strings.Builder
	code := Main([]string{"--server", f.server.URL, "doctor"}, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "v1.4.0")
	if code != 1 || !strings.Contains(stdout.String(), "no key; run observe login") {
		t.Fatalf("exit %d, stdout %q", code, stdout.String())
	}
}
