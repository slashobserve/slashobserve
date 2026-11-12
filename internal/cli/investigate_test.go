package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const observationID = "cccccccc-0000-4000-8000-000000000001"

const investigationID = "bbbbbbbb-0000-4000-8000-000000000001"

func TestInvestigatePayloadCarriesTheQuestionAndTarget(t *testing.T) {
	got, err := investigatePayload([]string{" why", "did errors rise "}, investigateOptions{dashboard: repairID, panel: "p_err", from: "now-6h", to: "now", timeout: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	if want := `{"dashboard_id":"` + repairID + `","from":"now-6h","kind":"investigate","panel_id":"p_err","prompt":"why did errors rise","to":"now"}`; string(raw) != want {
		t.Errorf("payload = %s\nwant %s", raw, want)
	}
	got, err = investigatePayload([]string{"why"}, investigateOptions{timeout: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(got); string(raw) != `{"kind":"investigate","prompt":"why"}` {
		t.Errorf("bare payload = %s", raw)
	}
}

func TestInvestigatePayloadNamesTheObservationOnlyWhenGiven(t *testing.T) {
	got, err := investigatePayload([]string{"why"}, investigateOptions{observation: observationID, timeout: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(got); string(raw) != `{"kind":"investigate","observation_id":"`+observationID+`","prompt":"why"}` {
		t.Errorf("payload = %s", raw)
	}
}

func TestInvestigateSendsTheObservationFlag(t *testing.T) {
	server, requests := investigationServer(t, findingJSON)
	_, stderr, code := runCLI("--server", server.URL, "--token", "k", "-q", "investigate", "--observation", observationID, "why", "did", "errors", "rise")
	if code != 0 {
		t.Fatalf("code %d stderr %q", code, stderr)
	}
	if sent := (*requests)[0]; sent.path != "/api/v1/observe" || sent.body != `{"kind":"investigate","observation_id":"`+observationID+`","prompt":"why did errors rise"}` {
		t.Errorf("request = %+v", sent)
	}
}

func TestInvestigateFlagRulesAreUsageErrors(t *testing.T) {
	cases := map[string][]string{
		"panel without dashboard": {"investigate", "why", "--panel", "p1"},
		"from without to":         {"investigate", "why", "--from", "now-1h"},
		"to without from":         {"investigate", "why", "--to", "now"},
		"empty question":          {"investigate", "  "},
		"no question":             {"investigate"},
		"bad dashboard id":        {"investigate", "why", "--dashboard", "checkout"},
		"bad observation id":      {"investigate", "why", "--observation", "weekly-review"},
		"zero timeout":            {"investigate", "why", "--timeout", "0s"},
	}
	for name, args := range cases {
		server, requests := healthServer(t, `{}`)
		_, stderr, code := runCLI(append([]string{"--server", server.URL, "--token", "k"}, args...)...)
		if code != 2 || len(*requests) != 0 {
			t.Errorf("%s: code %d, %d requests, stderr %q", name, code, len(*requests), stderr)
		}
	}
}

func TestPrintFindingShowsTitleAnswerAndAddress(t *testing.T) {
	var stdout bytes.Buffer
	printFinding(&output{out: &stdout, err: io.Discard}, findingSummary{
		Title:    "Checkout errors rose after the deploy",
		Markdown: "Errors rose at 10:02.\n\n- **p99** \x1b[31mdoubled\n",
		URL:      "https://observe.example.com/findings/f1",
	})
	want := "Checkout errors rose after the deploy\n\nErrors rose at 10:02.\n\n- **p99**  [31mdoubled\n\nhttps://observe.example.com/findings/f1\n"
	if stdout.String() != want {
		t.Errorf("printed %q\nwant %q", stdout.String(), want)
	}
}

const findingJSON = `{"id":"f1","title":"Checkout errors rose after the deploy","markdown":"Errors rose at 10:02.\nThe deploy at 10:00 added a retry.","url":"https://observe.example.com/findings/f1"}`

func investigationServer(t *testing.T, finding string) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		requests = append(requests, recordedRequest{method: r.Method, path: r.URL.Path, body: string(data)})
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"request":{"id":"`+investigationID+`","source":"investigate","status":"queued","steps":[]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"request":{"id":"`+investigationID+`","source":"investigate","status":"succeeded","url":"https://observe.example.com/findings/f1","steps":[],"finding":`+finding+`}}`)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func TestInvestigateWaitsAndPrintsTheFinding(t *testing.T) {
	server, requests := investigationServer(t, findingJSON)
	stdout, stderr, code := runCLI("--server", server.URL, "--token", "k", "-q", "investigate", "why", "did", "checkout", "errors", "rise")
	if code != 0 {
		t.Fatalf("code %d stderr %q", code, stderr)
	}
	want := "Checkout errors rose after the deploy\n\nErrors rose at 10:02.\nThe deploy at 10:00 added a retry.\n\nhttps://observe.example.com/findings/f1\n"
	if stdout != want {
		t.Errorf("stdout %q\nwant %q", stdout, want)
	}
	if sent := (*requests)[0]; sent.path != "/api/v1/observe" || sent.body != `{"kind":"investigate","prompt":"why did checkout errors rise"}` {
		t.Errorf("request = %+v", sent)
	}
}

func TestInvestigateWithoutAFindingFailsNamingTheRequest(t *testing.T) {
	server, _ := investigationServer(t, "null")
	_, stderr, code := runCLI("--server", server.URL, "--token", "k", "-q", "investigate", "why")
	if code != 1 || !strings.Contains(stderr, "finished without a finding: observe status "+investigationID) {
		t.Errorf("code %d stderr %q", code, stderr)
	}
}

func TestStatusOfAnInvestigationPrintsItsFinding(t *testing.T) {
	server, _ := investigationServer(t, findingJSON)
	stdout, stderr, code := runCLI("--server", server.URL, "--token", "k", "status", investigationID)
	if code != 0 || !strings.Contains(stdout, "Finding      Checkout errors rose after the deploy") || !strings.Contains(stdout, "Finding URL  https://observe.example.com/findings/f1") {
		t.Errorf("code %d stdout %q stderr %q", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI("--server", server.URL, "--token", "k", "-q", "status", investigationID, "--wait")
	if code != 0 || !strings.HasPrefix(stdout, "Checkout errors rose after the deploy\n") {
		t.Errorf("--wait: code %d stdout %q stderr %q", code, stdout, stderr)
	}
}
