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

func TestInvestigatePrivateSendsKindAsk(t *testing.T) {
	got, err := investigatePayload([]string{"why"}, investigateOptions{observation: observationID, private: true, timeout: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(got); string(raw) != `{"kind":"ask","observation_id":"`+observationID+`","prompt":"why"}` {
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
		"private with no subject": {"investigate", "why", "--private"},
		"private with both":       {"investigate", "why", "--private", "--dashboard", repairID, "--observation", observationID},
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
	}, investigationID)
	want := "Checkout errors rose after the deploy\n\nErrors rose at 10:02.\n\n- **p99**  [31mdoubled\n\nhttps://observe.example.com/findings/f1\n"
	if stdout.String() != want {
		t.Errorf("printed %q\nwant %q", stdout.String(), want)
	}
}

func TestPrintFindingOffersToSaveAnAnswerThatIsNotAnObservation(t *testing.T) {
	var stdout bytes.Buffer
	printFinding(&output{out: &stdout, err: io.Discard}, findingSummary{Title: "Log sources", Markdown: "Two sources."}, investigationID)
	want := "Log sources\n\nTwo sources.\n\nThis answer is kept in the chat only. Save it as an observation: observe save-answer " + investigationID + "\n"
	if stdout.String() != want {
		t.Errorf("printed %q\nwant %q", stdout.String(), want)
	}
}

func TestPrintFindingShowsTheSuggestedChangeAsARevision(t *testing.T) {
	var stdout bytes.Buffer
	printFinding(&output{out: &stdout, err: io.Discard}, findingSummary{Title: "Errors", Markdown: "One pod.", URL: "https://o.example/o/1", SuggestedChange: "Split errors by pod.", DashboardID: repairID}, investigationID)
	want := "Errors\n\nOne pod.\n\nSuggested change: Split errors by pod.\n  observe --revise --dashboard " + repairID + " 'Split errors by pod.'\n\nhttps://o.example/o/1\n"
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
			_, _ = io.WriteString(w, `{"request":{"id":"`+investigationID+`","kind":"investigate","intent":"ask","investigation":true,"status":"queued","steps":[]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"request":{"id":"`+investigationID+`","kind":"investigate","intent":"ask","investigation":true,"status":"succeeded","url":"https://observe.example.com/findings/f1","steps":[],"finding":`+finding+`}}`)
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
	if code != 0 || !strings.Contains(stdout, "Investigation  yes") || !strings.Contains(stdout, "Finding        Checkout errors rose after the deploy") || !strings.Contains(stdout, "Finding URL    https://observe.example.com/findings/f1") {
		t.Errorf("code %d stdout %q stderr %q", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI("--server", server.URL, "--token", "k", "-q", "status", investigationID, "--wait")
	if code != 0 || !strings.HasPrefix(stdout, "Checkout errors rose after the deploy\n") {
		t.Errorf("--wait: code %d stdout %q stderr %q", code, stdout, stderr)
	}
}

func TestInvestigatePayloadPointsAtASpotOfABlock(t *testing.T) {
	at := `{"kind":"position","pos":{"x":0.4,"y":0.3}}`
	got, err := investigatePayload([]string{"why"}, investigateOptions{observation: observationID, block: investigationID, at: at, timeout: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"anchor":` + at + `,"block_id":"` + investigationID + `","kind":"investigate","observation_id":"` + observationID + `","prompt":"why"}`
	if raw, _ := json.Marshal(got); string(raw) != want {
		t.Errorf("payload = %s\nwant %s", raw, want)
	}
	if _, err := investigatePayload([]string{"why"}, investigateOptions{observation: observationID, at: `{"kind":"position"`, timeout: time.Hour}); err == nil || !strings.Contains(err.Error(), "--at") {
		t.Errorf("a broken anchor: error = %v, want one naming --at", err)
	}
}

func TestInvestigatePayloadAsksAboutAWholeBlock(t *testing.T) {
	got, err := investigatePayload([]string{"why"}, investigateOptions{observation: observationID, block: investigationID, private: true, timeout: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"block_id":"` + investigationID + `","kind":"ask","observation_id":"` + observationID + `","prompt":"why"}`
	if raw, _ := json.Marshal(got); string(raw) != want {
		t.Errorf("payload = %s\nwant %s", raw, want)
	}
	if _, err := investigatePayload([]string{"why"}, investigateOptions{block: investigationID, timeout: time.Hour}); err == nil || !strings.Contains(err.Error(), "--block needs --observation") {
		t.Errorf("a block without an observation: error = %v", err)
	}
}

func TestPromptPayloadRevisesOnlyWithReviseAndADashboard(t *testing.T) {
	got, err := promptOptions{dashboard: repairID, revise: true}.payload("add a latency panel")
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(got); string(raw) != `{"dashboard_id":"`+repairID+`","kind":"revise","prompt":"add a latency panel"}` {
		t.Errorf("payload = %s", raw)
	}
	if _, err := (promptOptions{revise: true}).payload("add a latency panel"); err == nil || !strings.Contains(err.Error(), "--revise needs --dashboard") {
		t.Errorf("--revise alone: error = %v", err)
	}
}

func TestPromptPayloadStartsANewChatOnlyWithoutANamedOne(t *testing.T) {
	got, err := promptOptions{dashboard: repairID, revise: true, newChat: true}.payload("add a latency panel")
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(got); string(raw) != `{"dashboard_id":"`+repairID+`","kind":"revise","new_chat":true,"prompt":"add a latency panel"}` {
		t.Errorf("payload = %s", raw)
	}
	for _, o := range []promptOptions{{newChat: true, chat: repairID}} {
		if _, err := o.payload("x"); err == nil || !strings.Contains(err.Error(), "--new-chat excludes --chat") {
			t.Errorf("%+v: error = %v", o, err)
		}
	}
}

func TestPromptPayloadCarriesTheAnchor(t *testing.T) {
	at := `{"kind":"time","pos":{"x":0.5,"y":0.5},"from":"2026-11-20T23:56:00Z"}`
	got, err := promptOptions{dashboard: repairID, at: at}.payload("why this spike")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"anchor":` + at + `,"dashboard_id":"` + repairID + `","prompt":"why this spike"}`
	if raw, _ := json.Marshal(got); string(raw) != want {
		t.Errorf("payload = %s\nwant %s", raw, want)
	}
}

func TestInvestigateSendsAsOfOnlyWhenGiven(t *testing.T) {
	got, err := investigatePayload([]string{"why"}, investigateOptions{asOf: "2026-11-20T10:30:00Z", from: "now-1h", to: "now", timeout: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	if want := `{"as_of":"2026-11-20T10:30:00Z","from":"now-1h","kind":"investigate","prompt":"why","to":"now"}`; string(raw) != want {
		t.Errorf("payload = %s\nwant %s", raw, want)
	}
}

func TestPrintFindingShowsTheStructuredFieldsAboveTheAnswerOnlyWhenSet(t *testing.T) {
	var stdout bytes.Buffer
	printFinding(&output{out: &stdout, err: io.Discard}, findingSummary{
		Title:      "Checkout errors",
		Markdown:   "Errors rose.",
		URL:        "https://o.example/o/1",
		RootCause:  &findingRootCause{Component: "checkout", Kind: "deploy", Summary: "Version 42 added a retry."},
		Onset:      "2026-11-20T10:00:00Z",
		Impact:     "12% of orders failed.",
		Confidence: "high",
		RuledOut:   []string{"the database", "the cache"},
		NextSteps:  []string{"Roll back checkout."},
	}, investigationID)
	want := "Checkout errors\n\n" +
		"Root cause: checkout (deploy) - Version 42 added a retry.\n" +
		"Onset: 2026-11-20T10:00:00Z\nImpact: 12% of orders failed.\nConfidence: high\n" +
		"Ruled out: the database; the cache\nNext steps: Roll back checkout.\n\n" +
		"Errors rose.\n\nhttps://o.example/o/1\n"
	if stdout.String() != want {
		t.Errorf("printed %q\nwant %q", stdout.String(), want)
	}
	stdout.Reset()
	printFinding(&output{out: &stdout, err: io.Discard}, findingSummary{Title: "Quiet", Markdown: "Nothing.", URL: "https://o.example/o/2", NoIncident: true}, investigationID)
	if want := "Quiet\n\nNo incident found.\n\nNothing.\n\nhttps://o.example/o/2\n"; stdout.String() != want {
		t.Errorf("printed %q\nwant %q", stdout.String(), want)
	}
}

func TestPrintFindingListsTheOtherCausesUnderTheRootCause(t *testing.T) {
	var f findingSummary
	raw := `{"root_cause":{"component":"cartFailure","kind":"config_or_flag","summary":"The flag broke cart."},"causes":[
		{"component":{"name":"cartFailure","namespace":null},"mechanism":"The flag broke cart.","knock_on_of":null,"band":"high","stated":true},
		{"component":{"name":"checkout","namespace":"shop"},"mechanism":"Checkout failed on cart calls.","knock_on_of":0,"band":"medium","stated":true},
		{"component":{"name":"payments","namespace":null},"mechanism":"Maybe slow.","knock_on_of":null,"band":"low","stated":false}]}`
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Root cause: cartFailure (config_or_flag) - The flag broke cart.",
		"Also caused by: shop/checkout (knock-on, medium) - Checkout failed on cart calls.",
		"Unconfirmed cause: payments (low) - Maybe slow.",
	}
	if got := f.header(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("header %q\nwant %q", got, want)
	}
}

func TestPrintFindingListsTheItemsNotFoundInWhatTheAgentRead(t *testing.T) {
	var stdout bytes.Buffer
	printFinding(&output{out: &stdout, err: io.Discard}, findingSummary{
		Title:      "Checkout latency",
		Markdown:   "p99 rose to 523 ms.",
		URL:        "https://o.example/o/1",
		Unverified: []findingUnverified{{Kind: "figure", Text: "523 ms"}, {Kind: "candidate", Text: "c2 ad"}, {Kind: "component", Text: "cartservice"}},
	}, investigationID)
	want := "Checkout latency\n\nNot found in what the agent read: 523 ms; cartservice\nTriage candidates left open: c2 ad\n\np99 rose to 523 ms.\n\nhttps://o.example/o/1\n"
	if stdout.String() != want {
		t.Errorf("printed %q\nwant %q", stdout.String(), want)
	}
}
