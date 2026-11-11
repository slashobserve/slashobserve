package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const testWebhook = "ffffffff-0000-4000-8000-000000000003"

type webhooksFake struct {
	server       *httptest.Server
	mu           sync.Mutex
	requests     []recordedRequest
	testOutcome  string
	testStatus   string
	deliveries   string
	webhooksList string
}

func webhookJSON(enabled bool, reason string) string {
	disabled := "null"
	if reason != "" {
		disabled = fmt.Sprintf("%q", reason)
	}
	state := "sending"
	if !enabled {
		state = "disabled"
	}
	return fmt.Sprintf(`{"id":%q,"name":"Failed runs","url":"https://hooks.example.com/in","event_types":["run.failed","run.canceled"],"owner":{"kind":"user","id":"u","name":"Ada","origin":null,"client":null,"run_id":null,"label":"Ada"},"enabled":%t,"state":%q,"disabled_reason":%s,"disabled_at":null,"consecutive_failures":0,"next_attempt_at":"2026-11-11T09:00:00Z","created_at":"2026-11-11T09:00:00Z","updated_at":"2026-11-11T09:00:00Z"}`, testWebhook, enabled, state, disabled)
}

func newWebhooksFake(t *testing.T) *webhooksFake {
	t.Helper()
	f := &webhooksFake{
		testOutcome:  "delivered",
		testStatus:   "200",
		deliveries:   `{"deliveries":[{"id":"2","delivery_id":"d.2","event_type":"run.failed","activity_seq":9,"attempted_at":"2026-11-11T09:30:00Z","duration_ms":42,"status_code":500,"outcome":"failed","error_code":"status"},{"id":"1","delivery_id":"d.1","event_type":"webhook.test","activity_seq":null,"attempted_at":"2026-11-11T09:00:00Z","duration_ms":7,"status_code":null,"outcome":"failed","error_code":"unreachable"}],"next_cursor":null}`,
		webhooksList: `{"webhooks":[` + webhookJSON(true, "") + `,` + webhookJSON(false, "gone") + `]}`,
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, recordedRequest{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, body: string(data)})
		w.Header().Set("Content-Type", "application/json")
		prefix := "/api/v1/webhooks/" + testWebhook
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/webhooks":
			_, _ = io.WriteString(w, f.webhooksList)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/webhooks":
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"webhook":%s,"secret":"whsec_created"}`, webhookJSON(true, ""))
		case r.Method == http.MethodPatch && r.URL.Path == prefix:
			var body map[string]any
			_ = json.Unmarshal(data, &body)
			enabled, _ := body["enabled"].(bool)
			_, _ = io.WriteString(w, webhookJSON(enabled || body["enabled"] == nil, ""))
		case r.Method == http.MethodDelete && r.URL.Path == prefix:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == prefix+"/secret":
			_, _ = fmt.Fprintf(w, `{"webhook":%s,"secret":"whsec_rotated"}`, webhookJSON(true, ""))
		case r.Method == http.MethodPost && r.URL.Path == prefix+"/test":
			_, _ = fmt.Fprintf(w, `{"delivery":{"id":"3","delivery_id":"d.test","event_type":"webhook.test","activity_seq":null,"attempted_at":"2026-11-11T09:31:00Z","duration_ms":120,"status_code":%s,"outcome":%q,"error_code":%s}}`,
				f.testStatus, f.testOutcome, map[bool]string{true: "null", false: `"status"`}[f.testOutcome == "delivered"])
		case r.Method == http.MethodGet && r.URL.Path == prefix+"/deliveries":
			_, _ = io.WriteString(w, f.deliveries)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not found","code":"not_found"}`)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *webhooksFake) run(args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := Main(append([]string{"--server", f.server.URL, "--token", "k", "webhooks"}, args...), IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	return stdout.String(), stderr.String(), code
}

func TestWebhooksListPrintsATable(t *testing.T) {
	f := newWebhooksFake(t)
	stdout, stderr, code := f.run("list")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	got := spaces.ReplaceAllString(stdout, " ")
	want := "ID NAME URL EVENTS STATUS\n" +
		testWebhook + " Failed runs https://hooks.example.com/in 2 sending\n" +
		testWebhook + " Failed runs https://hooks.example.com/in 2 disabled (gone)\n"
	if got != want {
		t.Fatalf("stdout %q", got)
	}
	if f.requests[0].method != http.MethodGet || f.requests[0].path != "/api/v1/webhooks" {
		t.Fatalf("request %+v", f.requests[0])
	}
}

func TestWebhooksListEmptySaysSo(t *testing.T) {
	f := newWebhooksFake(t)
	f.webhooksList = `{"webhooks":[]}`
	stdout, stderr, code := f.run("list")
	if code != 0 || stdout != "" || !strings.Contains(stderr, "No webhooks yet") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestWebhooksAddSendsTheFlagsAndPrintsTheSecretOnce(t *testing.T) {
	f := newWebhooksFake(t)
	stdout, stderr, code := f.run("add", "--name", "Failed runs", "--url", "https://hooks.example.com/in", "--event", "run.failed", "--event", "run.canceled")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if f.requests[0].method != http.MethodPost || f.requests[0].path != "/api/v1/webhooks" {
		t.Fatalf("request %+v", f.requests[0])
	}
	want := `{"event_types":["run.failed","run.canceled"],"name":"Failed runs","url":"https://hooks.example.com/in"}`
	if f.requests[0].body != want {
		t.Fatalf("body %s", f.requests[0].body)
	}
	if strings.Count(stdout, "whsec_created") != 1 || !strings.Contains(stdout, "Created webhook "+testWebhook) {
		t.Fatalf("stdout %q", stdout)
	}
	if !strings.Contains(stderr, "shown only now") || strings.Contains(stderr, "whsec_created") {
		t.Fatalf("stderr %q", stderr)
	}
}

func TestWebhooksAddQuietStillPrintsTheSecret(t *testing.T) {
	f := newWebhooksFake(t)
	var stdout, stderr bytes.Buffer
	code := Main([]string{"--server", f.server.URL, "--token", "k", "-q", "webhooks", "add", "--name", "n", "--url", "https://a.example.com", "--event", "run.failed"}, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	if code != 0 || !strings.Contains(stdout.String(), "whsec_created") || stderr.Len() != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestWebhooksAddRefusesMissingFlagsBeforeAnyRequest(t *testing.T) {
	cases := [][]string{
		{"add", "--url", "https://a.example.com", "--event", "run.failed"},
		{"add", "--name", "n", "--event", "run.failed"},
		{"add", "--name", "n", "--url", "https://a.example.com"},
	}
	for _, args := range cases {
		f := newWebhooksFake(t)
		_, stderr, code := f.run(args...)
		if code != 2 || !strings.Contains(stderr, "--name, --url and at least one --event") || len(f.requests) != 0 {
			t.Errorf("%v: exit %d, stderr %q, requests %d", args, code, stderr, len(f.requests))
		}
	}
}

func TestWebhooksUpdateSendsOnlyTheChangedFields(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--name", "Renamed"}, `{"name":"Renamed"}`},
		{[]string{"--url", "https://b.example.com/x"}, `{"url":"https://b.example.com/x"}`},
		{[]string{"--event", "run.failed", "--event", "run.succeeded"}, `{"event_types":["run.failed","run.succeeded"]}`},
	}
	for _, tc := range cases {
		f := newWebhooksFake(t)
		stdout, stderr, code := f.run(append([]string{"update", testWebhook}, tc.args...)...)
		if code != 0 || f.requests[0].method != http.MethodPatch || f.requests[0].path != "/api/v1/webhooks/"+testWebhook || f.requests[0].body != tc.want {
			t.Errorf("%v: exit %d, request %+v, stderr %q", tc.args, code, f.requests[0], stderr)
		}
		if !strings.Contains(stdout, "Updated webhook "+testWebhook) {
			t.Errorf("stdout %q", stdout)
		}
	}
}

func TestWebhooksUpdateWithNothingToChangeIsAUsageError(t *testing.T) {
	f := newWebhooksFake(t)
	_, stderr, code := f.run("update", testWebhook)
	if code != 2 || !strings.Contains(stderr, "at least one of --name, --url or --event") || len(f.requests) != 0 {
		t.Fatalf("exit %d, stderr %q, requests %d", code, stderr, len(f.requests))
	}
}

func TestWebhooksEnableAndDisablePatchEnabled(t *testing.T) {
	for verb, want := range map[string]string{"enable": `{"enabled":true}`, "disable": `{"enabled":false}`} {
		f := newWebhooksFake(t)
		_, stderr, code := f.run(verb, testWebhook)
		if code != 0 || f.requests[0].method != http.MethodPatch || f.requests[0].body != want {
			t.Errorf("%s: exit %d, request %+v, stderr %q", verb, code, f.requests[0], stderr)
		}
	}
}

func TestWebhooksTestPrintsTheOutcomeAndExitsZeroWhenDelivered(t *testing.T) {
	f := newWebhooksFake(t)
	stdout, stderr, code := f.run("test", testWebhook)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if f.requests[0].method != http.MethodPost || f.requests[0].path != "/api/v1/webhooks/"+testWebhook+"/test" {
		t.Fatalf("request %+v", f.requests[0])
	}
	if got := spaces.ReplaceAllString(stdout, " "); got != "Outcome delivered\nStatus 200\nDuration 120 ms\nError -\n" {
		t.Fatalf("stdout %q", got)
	}
}

func TestWebhooksTestExitsOneWhenNotDelivered(t *testing.T) {
	f := newWebhooksFake(t)
	f.testOutcome, f.testStatus = "failed", "503"
	stdout, stderr, code := f.run("test", testWebhook)
	if code != 1 || !strings.Contains(stderr, "the test event was not delivered") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if got := spaces.ReplaceAllString(stdout, " "); !strings.Contains(got, "Outcome failed\nStatus 503\n") || !strings.Contains(got, "Error status") {
		t.Fatalf("stdout %q", got)
	}
}

func TestWebhooksTestJSONPrintsTheBodyAndStillFailsWhenNotDelivered(t *testing.T) {
	f := newWebhooksFake(t)
	f.testOutcome = "failed"
	stdout, _, code := f.run("--json", "test", testWebhook)
	if code != 1 || !strings.HasPrefix(stdout, `{"delivery":{`) {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestWebhooksRotateSecretPrintsTheNewSecretOnce(t *testing.T) {
	f := newWebhooksFake(t)
	stdout, stderr, code := f.run("rotate-secret", testWebhook)
	if code != 0 || f.requests[0].method != http.MethodPost || f.requests[0].path != "/api/v1/webhooks/"+testWebhook+"/secret" {
		t.Fatalf("exit %d, request %+v, stderr %q", code, f.requests[0], stderr)
	}
	if strings.Count(stdout, "whsec_rotated") != 1 || !strings.Contains(stderr, "shown only now") {
		t.Fatalf("stdout %q, stderr %q", stdout, stderr)
	}
}

func TestWebhooksDeleteSendsDelete(t *testing.T) {
	f := newWebhooksFake(t)
	stdout, stderr, code := f.run("delete", testWebhook)
	if code != 0 || stderr != "" || f.requests[0].method != http.MethodDelete || f.requests[0].path != "/api/v1/webhooks/"+testWebhook {
		t.Fatalf("exit %d, request %+v, stderr %q", code, f.requests[0], stderr)
	}
	if stdout != "Deleted webhook "+testWebhook+"\n" {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestWebhooksDeliveriesPrintsATableAndSendsTheLimit(t *testing.T) {
	f := newWebhooksFake(t)
	stdout, stderr, code := f.run("deliveries", testWebhook, "--limit", "5")
	if code != 0 || stderr != "" || f.requests[0].query != "limit=5" {
		t.Fatalf("exit %d, request %+v, stderr %q", code, f.requests[0], stderr)
	}
	want := "WHEN EVENT OUTCOME STATUS DURATION ERROR\n" +
		"2026-11-11T09:30Z run.failed failed 500 42 ms status\n" +
		"2026-11-11T09:00Z webhook.test failed - 7 ms unreachable\n"
	if got := spaces.ReplaceAllString(stdout, " "); got != want {
		t.Fatalf("stdout %q", got)
	}
}

func TestWebhooksDeliveriesDefaultsTheLimitAndRefusesBadOnes(t *testing.T) {
	f := newWebhooksFake(t)
	if _, _, code := f.run("deliveries", testWebhook); code != 0 || f.requests[0].query != "limit=20" {
		t.Fatalf("exit %d, request %+v", code, f.requests[0])
	}
	for _, limit := range []string{"0", "101"} {
		f := newWebhooksFake(t)
		_, stderr, code := f.run("deliveries", testWebhook, "--limit", limit)
		if code != 2 || !strings.Contains(stderr, "--limit must be 1 to 100") || len(f.requests) != 0 {
			t.Errorf("limit %s: exit %d, stderr %q, requests %d", limit, code, stderr, len(f.requests))
		}
	}
}

func TestWebhooksJSONPrintsTheResponseBodyUnchanged(t *testing.T) {
	f := newWebhooksFake(t)
	stdout, _, code := f.run("--json", "list")
	if code != 0 || strings.TrimSpace(stdout) != f.webhooksList {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
	stdout, _, code = f.run("--json", "add", "--name", "n", "--url", "https://a.example.com", "--event", "run.failed")
	if code != 0 || !strings.Contains(stdout, `"secret":"whsec_created"`) {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
	stdout, _, code = f.run("--json", "deliveries", testWebhook)
	if code != 0 || strings.TrimSpace(stdout) != f.deliveries {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestWebhooksRefuseABadID(t *testing.T) {
	for _, verb := range []string{"update", "enable", "disable", "test", "rotate-secret", "delete", "deliveries"} {
		f := newWebhooksFake(t)
		args := []string{verb, "nope"}
		if verb == "update" {
			args = append(args, "--name", "x")
		}
		_, stderr, code := f.run(args...)
		if code != 2 || stderr == "" || len(f.requests) != 0 {
			t.Errorf("%s: exit %d, stderr %q, requests %d", verb, code, stderr, len(f.requests))
		}
	}
}

func TestWebhookStatusReadsTheServerState(t *testing.T) {
	reason := "gone"
	tests := []struct {
		name string
		view webhookView
		want string
	}{
		{"sending", webhookView{State: "sending"}, "sending"},
		{"retrying", webhookView{State: "retrying"}, "retrying"},
		{"disabled with a reason", webhookView{State: "disabled", DisabledReason: &reason}, "disabled (gone)"},
		{"disabled without a reason", webhookView{State: "disabled"}, "disabled (-)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := webhookStatus(tt.view); got != tt.want {
				t.Errorf("webhookStatus = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWebhooksSurfaceAPIErrors(t *testing.T) {
	f := newWebhooksFake(t)
	_, stderr, code := f.run("delete", "ffffffff-0000-4000-8000-0000000000aa")
	if code != 1 || stderr == "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}
