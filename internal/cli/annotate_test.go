package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/forbidden"
)

type annotateFake struct {
	server   *httptest.Server
	requests []recordedRequest
	status   int
	answer   string
}

func newAnnotateFake(t *testing.T) *annotateFake {
	t.Helper()
	f := &annotateFake{status: http.StatusCreated}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, recordedRequest{method: r.Method, path: r.URL.Path, body: string(data)})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		if f.answer != "" {
			_, _ = io.WriteString(w, f.answer)
			return
		}
		var sent annotateBody
		_ = json.Unmarshal(data, &sent)
		dashboard := "null"
		if sent.DashboardID != nil {
			dashboard = fmt.Sprintf("%q", *sent.DashboardID)
		}
		from := "2026-11-01T10:00:00Z"
		if sent.TimeFrom != nil {
			from = *sent.TimeFrom
		}
		_, _ = fmt.Fprintf(w, `{"id":"aaaaaaaa-0000-4000-8000-000000000001","dashboard_id":%s,"panel_id":null,"time_from":%q,"time_to":null,"text":%q,"tags":[],"created_by":null,"created_by_name":"ci","created_at":"2026-11-01T10:00:00Z"}`, dashboard, from, sent.Text)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *annotateFake) run(args ...string) (string, string, int) {
	return runCLI(append([]string{"--server", f.server.URL, "--token", "k"}, args...)...)
}

func sentBody(t *testing.T, r recordedRequest) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(r.body), &body); err != nil {
		t.Fatalf("body %q: %v", r.body, err)
	}
	return body
}

func TestAnnotateAddsAGlobalAnnotation(t *testing.T) {
	f := newAnnotateFake(t)
	stdout, stderr, code := f.run("annotate", "Deploy", "checkout", "v1.4.2", "--tags", "deploy, checkout,,")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if len(f.requests) != 1 || f.requests[0].method != http.MethodPost || f.requests[0].path != "/api/v1/annotations" {
		t.Fatalf("requests %+v", f.requests)
	}
	body := sentBody(t, f.requests[0])
	if body["text"] != "Deploy checkout v1.4.2" || fmt.Sprint(body["tags"]) != "[deploy checkout]" {
		t.Errorf("body %v", body)
	}
	if _, ok := body["dashboard_id"]; ok {
		t.Errorf("body %v sends a dashboard", body)
	}
	if _, ok := body["time_from"]; ok {
		t.Errorf("body %v sends a time", body)
	}
	if want := "Added annotation aaaaaaaa-0000-4000-8000-000000000001 at 2026-11-01T10:00Z on every dashboard\n"; stdout != want {
		t.Errorf("stdout %q, want %q", stdout, want)
	}
}

func TestAnnotateAddsADashboardAnnotationAtATime(t *testing.T) {
	f := newAnnotateFake(t)
	stdout, _, code := f.run("annotate", "failover", "--dashboard", testDashboard, "--at", "2026-11-01T09:30:00Z")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	body := sentBody(t, f.requests[0])
	if body["dashboard_id"] != testDashboard || body["time_from"] != "2026-11-01T09:30:00Z" || fmt.Sprint(body["tags"]) != "[]" {
		t.Errorf("body %v", body)
	}
	if !strings.HasSuffix(stdout, "at 2026-11-01T09:30Z on dashboard "+testDashboard+"\n") {
		t.Errorf("stdout %q", stdout)
	}
}

func TestParseAt(t *testing.T) {
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.FixedZone("IST", 5*3600+1800))
	cases := []struct {
		raw  string
		want string
	}{
		{"", ""},
		{"now", ""},
		{"now-15m", "2026-11-01T04:15:00Z"},
		{"now-2h", "2026-11-01T02:30:00Z"},
		{"now-1d", "2026-10-31T04:30:00Z"},
		{"now-30s", "2026-11-01T04:29:30Z"},
		{"2026-11-01T12:00:00+02:00", "2026-11-01T12:00:00+02:00"},
	}
	for _, tc := range cases {
		got, err := parseAt(tc.raw, now)
		if err != nil {
			t.Errorf("parseAt(%q): %v", tc.raw, err)
			continue
		}
		if (got == nil) != (tc.want == "") || got != nil && *got != tc.want {
			t.Errorf("parseAt(%q) = %v, want %q", tc.raw, got, tc.want)
		}
	}
	for _, raw := range []string{"yesterday", "now-0m", "now+1h", "now-1w", "2026-11-01"} {
		if _, err := parseAt(raw, now); err == nil {
			t.Errorf("parseAt(%q) accepted", raw)
		}
	}
}

func TestAnnotateRefusesBadInputBeforeAnyRequest(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"unknown time", []string{"annotate", "x", "--at", "yesterday"}, "is not a time"},
		{"bad dashboard id", []string{"annotate", "x", "--dashboard", "nope"}, "is not a valid id"},
		{"no text", []string{"annotate"}, "takes the annotation text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAnnotateFake(t)
			_, stderr, code := f.run(tc.args...)
			if code != 2 || !strings.Contains(stderr, tc.want) || len(f.requests) != 0 {
				t.Fatalf("exit %d, stderr %q, requests %d", code, stderr, len(f.requests))
			}
		})
	}
}

func TestAnnotateForbiddenRoleExitsOneWithTheHint(t *testing.T) {
	f := newAnnotateFake(t)
	f.status = http.StatusForbidden
	f.answer = fmt.Sprintf(`{"error":%q,"code":%q}`, forbidden.RoleMessage, forbidden.Code)
	_, stderr, code := f.run("annotate", "x")
	if code != 1 || !strings.Contains(stderr, forbiddenRoleHint) {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestAnnotateJSONPrintsTheResponse(t *testing.T) {
	f := newAnnotateFake(t)
	stdout, _, code := f.run("annotate", "x", "--json")
	if code != 0 || !strings.HasPrefix(stdout, `{"id":"aaaaaaaa-0000-4000-8000-000000000001"`) || !strings.Contains(stdout, `"created_by_name":"ci"`) {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestAnnotateQuietHidesTheConfirmation(t *testing.T) {
	f := newAnnotateFake(t)
	stdout, stderr, code := f.run("annotate", "x", "-q")
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}
