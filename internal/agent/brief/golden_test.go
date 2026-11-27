package brief

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	briefGoldenDir  = "../../../spec/examples/agent/brief"
	promptGoldenDir = "../../../spec/examples/agent/prompts"
)

var updateBriefGoldens = flag.Bool("update", false, "rewrite spec/examples/agent/brief")

type goldenDiscovery struct {
	Datasource string   `json:"datasource"`
	Of         string   `json:"of"`
	Names      []string `json:"names"`
}

type goldenState struct {
	Task   route.Task             `json:"task"`
	Loop   bool                   `json:"loop"`
	Facts  bool                   `json:"facts"`
	Ledger []revisions.Hypothesis `json:"ledger"`
	Triage string                 `json:"triage"`
	Digest []goldenDiscovery      `json:"digest"`
}

type goldenText struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type briefGolden struct {
	Name   string         `json:"name"`
	Brief  tools.RunBrief `json:"brief"`
	Guide  string         `json:"guide"`
	Reason Reason         `json:"reason"`
	Now    time.Time      `json:"now"`
	State  goldenState    `json:"state"`
	Want   struct {
		System  goldenText `json:"system"`
		Context string     `json:"context"`
		User    string     `json:"user"`
	} `json:"want"`
}

type briefInput struct {
	name     string
	statuses []datasource.Status
	everyone []string
	task     Task
	view     View
	draft    json.RawMessage
	reason   Reason
	state    goldenState
}

func mustDashboard(t *testing.T, raw string) *spec.Dashboard {
	t.Helper()
	d, err := spec.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

const goldenSpec = `{"specVersion":"0","title":"Checkout <live> & co","tags":["shop","p-l"],"time":{"from":"now-6h","to":"now"},"refresh":"30s",` +
	`"variables":[{"name":"service","type":"query","datasource":"prom","query":"label_values(up, service)"},{"name":"env","type":"custom","options":["prod","staging"],"multi":true,"default":["prod"]}],` +
	`"panels":[{"id":"p1","type":"stat","title":"Requests \"now\" été","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"prom","queries":[{"query":"sum(rate(http_requests_total{service=\"$service\"}[5m]))"}],"unit":"reqps","decimals":2,"min":2.5e-7},` +
	`{"id":"p2","type":"timeseries","title":"Latency p99","gridPos":{"x":6,"y":0,"w":18,"h":8},"datasource":"prom","queries":[{"ref":"A","query":"histogram_quantile(0.99, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))","legend":"p99"}],"unit":"s","min":0.000001},` +
	`{"id":"r1","type":"row","title":"Details","gridPos":{"x":0,"y":8,"w":24,"h":1},"panels":[{"id":"p3","type":"table","title":"Errors by route","gridPos":{"x":0,"y":9,"w":12,"h":6},"datasource":"prom","queries":[{"query":"topk(5, sum by (route) (rate(errors_total[5m])))","instant":true}]}]}]}`

func largeSpec(t *testing.T) *spec.Dashboard {
	t.Helper()
	panels := make([]string, 0, 64)
	for i := range 60 {
		panels = append(panels, fmt.Sprintf(`{"id":"s%d","type":"stat","title":"Service %d <errors> — café","description":"%s","gridPos":{"x":%d,"y":%d,"w":6,"h":4},"datasource":"prom","queries":[{"query":"sum(rate(http_requests_total{service=\"svc-%d\",code=~\"5..\"}[5m]))"}]}`,
			i, i, strings.Repeat("A long description of the panel. ", 5), (i%4)*6, (i/4)*4, i))
	}
	panels = append(panels, `{"id":"row-a","type":"row","title":"Row A","gridPos":{"x":0,"y":60,"w":24,"h":1},"panels":[{"id":"inner_1","type":"timeseries","title":"Inner","gridPos":{"x":0,"y":61,"w":24,"h":8},"datasource":"prom","queries":[{"query":"up"}]}]}`)
	return mustDashboard(t, `{"specVersion":"0","title":"Big","time":{"from":"now-1h","to":"now"},"panels":[`+strings.Join(panels, ",")+`]}`)
}

func timeAt(s string) *time.Time {
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &at
}

var (
	promOnly = []datasource.Status{{UID: "prom", Type: "prometheus", Name: "Prometheus", Healthy: true}}
	allKinds = []datasource.Status{
		{UID: "tempo", Type: "tempo", Healthy: true},
		{UID: "prom", Type: "prometheus", Healthy: true},
		{UID: "ch", Type: "clickhouse", Spans: "otel", Healthy: true},
		{UID: "loki", Type: "loki", Healthy: false, Message: "connection refused"},
		{UID: "jaeger", Type: "jaeger", Healthy: true},
		{UID: "es", Type: "elasticsearch", Index: "logs-*", TimeField: "@timestamp", Healthy: true},
	}
	builtinMix = []datasource.Status{
		{UID: "loki", Type: "loki", Healthy: true},
		{UID: "slashobserve-audit", Type: "tempo", Source: datasource.SourceBuiltin, Healthy: true},
	}
)

func briefInputs(t *testing.T) []briefInput {
	t.Helper()
	current := &Revision{
		DashboardID: "d1", Number: 4, Spec: mustDashboard(t, goldenSpec),
		Unverified: dashboards.Unverified{
			Panels:    []dashboards.UnverifiedPanel{{PanelID: "p2", Issues: []spec.Issue{{Path: "/panels/1/queries", Code: "query_empty", Message: "query returned\nno data"}}}},
			Variables: []dashboards.UnverifiedVariable{{Name: "service", Issues: []spec.Issue{{Code: "variable_failed", Message: "boom"}}}},
		},
		Conversion: []tools.Attention{{PanelID: "p3", Title: "Errors by route", Status: "partial", Reason: "Some overrides use matchers\nor properties that are not supported yet."}},
	}
	built := mustDashboard(t, goldenSpec)
	built.Panels = append(built.Panels, spec.Panel{ID: "p9", Type: "stat", Title: "New", GridPos: spec.GridPos{X: 0, Y: 20, W: 6, H: 4}})
	build := &dashboards.Build{BaseVersion: 4, Spec: built, PlannedPanels: 5, Saves: 2}
	createBuild := &dashboards.Build{Spec: mustDashboard(t, goldenSpec), Saves: 3}
	draft := json.RawMessage(`{"id":"p1","type":"stat","title":"Requests <draft>","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":"prom","queries":[{"query":"sum(rate(http_requests_total[1m]))"}]}`)
	suggested, err := withDraft(mustDashboard(t, goldenSpec), "p1", draft)
	if err != nil {
		t.Fatal(err)
	}
	ledger := []revisions.Hypothesis{
		{ID: "pool", Text: "The checkout pool is exhausted", Status: "supported", Closing: "cause", Evidence: []string{"pool_in_use 50 of 50 at 10:05", "waiters 120"}},
		{ID: "errors", Text: "Frontend errors rose", Status: "supported", Closing: "knock_on", KnockOnOf: "pool", Evidence: []string{"only on checkout calls"}},
		{ID: "db", Text: "The database is slow", Status: "open", Next: "read db latency"},
	}
	digest := []goldenDiscovery{
		{Datasource: "prom", Of: "metrics", Names: []string{"http_requests_total", "http_request_duration_seconds_bucket"}},
		{Datasource: "prom", Of: "labels", Names: []string{"service", "route", "code"}},
	}
	anchorText := anchorOf(t, []byte(`{"kind":"point","pos":{"x":0.4,"y":0.3},"series":{"service":"checkout","env":"prod"},"shown":"440 <ms> & more","from":"2026-11-21T05:26:30+05:30","window":{"from":"now-2h","to":"now"}}`))
	labels := make([]string, 16)
	for i := range labels {
		labels[i] = fmt.Sprintf(`"k%02d":"café  <%02d> & value of some length"`, i, i)
	}
	longQuote := anchorOf(t, []byte(`{"kind":"series","pos":{"x":0.1,"y":0.2},"series":{`+strings.Join(labels, ",")+`}}`))
	return []briefInput{
		{
			name: "create-fresh", statuses: promOnly, everyone: []string{"prom"},
			task:  Task{Mode: tools.ModeCreate, Source: revisions.SourcePrompt, Prompt: "make a checkout dashboard", Attempt: 1, TimeZone: "Europe/Berlin", Facts: true},
			state: goldenState{Facts: true, Digest: digest},
		},
		{
			name: "create-build-renewed", statuses: promOnly,
			task:   Task{Mode: tools.ModeCreate, Source: revisions.SourceObserve, Prompt: "make a checkout dashboard", Attempt: 1},
			view:   View{Build: createBuild},
			reason: Renewed,
			state:  goldenState{Digest: digest},
		},
		{
			name: "revise-build-restart", statuses: builtinMix, everyone: []string{"loki"},
			task:   Task{Mode: tools.ModeRevise, Source: revisions.SourceComment, Prompt: "make p2 red", PanelID: "p2", Attempt: 2, TimeZone: "UTC", Withheld: true},
			view:   View{Current: current, Build: build},
			reason: Restart,
		},
		{
			name: "revise-outline", statuses: promOnly,
			task:  Task{Mode: tools.ModeRevise, Source: revisions.SourcePrompt, Prompt: "change s7, s12, row-a and inner_1 (not s99)", PanelID: "s3", Attempt: 1, Facts: true},
			view:  View{Current: &Revision{DashboardID: "big", Number: 12, Spec: largeSpec(t)}},
			state: goldenState{Facts: true},
		},
		{
			name: "revise-missing-panel", statuses: promOnly,
			task: Task{Mode: tools.ModeRevise, Source: revisions.SourcePrompt, Prompt: "fix it", PanelID: "gone", Attempt: 1},
			view: View{Current: current},
		},
		{
			name: "suggest-draft", statuses: promOnly, everyone: []string{"prom"},
			task:  Task{Mode: tools.ModeSuggest, Source: revisions.SourceSuggest, Prompt: "show the last minute", PanelID: "p1", Attempt: 1, TimeZone: "Europe/Berlin", Withheld: true},
			view:  View{Current: &Revision{DashboardID: "d1", Number: 4, Spec: suggested}},
			draft: draft,
			state: goldenState{Digest: digest, Facts: false},
		},
		{
			name: "investigate-triage-ledger-facts", statuses: allKinds, everyone: []string{"prom", "tempo"},
			task: Task{
				Mode: tools.ModeInvestigate, Source: revisions.SourceInvestigate, Prompt: "why did checkout fail at 10:05?", Attempt: 1, Investigation: true,
				From: timeAt("2026-11-25T09:00:00+01:00"), To: timeAt("2026-11-25T10:30:00Z"), TimeZone: "Europe/Paris",
			},
			state: goldenState{
				Task: route.Incident, Loop: true, Facts: true, Ledger: ledger, Digest: digest,
				Triage: "<triage>\nA sweep of the datasources everyone can read, before your first call:\n- checkout errors 12% (was 0.4%) from 10:04\n</triage>",
			},
		},
		{
			name: "investigate-resume-after-compaction", statuses: promOnly,
			task: Task{
				Mode: tools.ModeInvestigate, Source: revisions.SourcePrompt, Prompt: "what is p1 measuring", PanelID: "p1", Attempt: 1,
				TimeZone: "Asia/Kolkata", AsOf: timeAt("2026-11-24T23:59:30Z"),
			},
			view:   View{Current: current},
			reason: OutputLimit,
			state:  goldenState{Task: route.Explain, Digest: digest},
		},
		{
			name: "continue-build", statuses: promOnly,
			task:   Task{Mode: tools.ModeRevise, Source: revisions.SourcePrompt, Prompt: "add a panel for errors", Attempt: 1, Continues: true, ContinueNote: "keep the row"},
			view:   View{Current: current, Build: build},
			reason: ResumeReason(Task{Continues: true, Attempt: 1}),
		},
		{
			name: "continue-investigate", statuses: promOnly,
			task:   Task{Mode: tools.ModeInvestigate, Source: revisions.SourceInvestigate, Prompt: "keep going", Attempt: 1, Continues: true, ContinueNote: "check the db too", Investigation: true},
			reason: Continue,
			state:  goldenState{Task: route.Full, Loop: true},
		},
		{
			name: "thread-ask-anchor", statuses: promOnly,
			task: Task{
				Mode: tools.ModeInvestigate, Source: revisions.SourceComment, Prompt: "@observe why is this so high?", Attempt: 1,
				TimeZone: "America/Sao_Paulo", ThreadContext: "Ana: the checkout <p99> spiked\nBo: since the deploy & rollback",
				Pointer: Pointer{Anchor: anchorText, BlockID: "blk1", ObservationID: "obs1", ThreadAsk: true},
			},
			state: goldenState{Task: route.Incident},
		},
		{
			name: "thread-ask-long-quote", statuses: nil,
			task: Task{
				Mode: tools.ModeInvestigate, Source: revisions.SourceComment, Prompt: "explain this", Attempt: 1, TimeZone: "Not/AZone",
				Pointer: Pointer{Anchor: longQuote, ObservationID: "obs2", ThreadAsk: true},
			},
			state: goldenState{Task: route.Explain},
		},
		{
			name: "thread-ask-page", statuses: promOnly,
			task:  Task{Mode: tools.ModeInvestigate, Source: revisions.SourceComment, Prompt: "summarize this", Attempt: 1, Pointer: Pointer{ObservationID: "obs3", ThreadAsk: true}},
			state: goldenState{Task: route.Explain},
		},
		{
			name: "block-ask", statuses: promOnly,
			task:  Task{Mode: tools.ModeInvestigate, Source: revisions.SourceComment, Prompt: "is this right", Attempt: 1, Pointer: Pointer{BlockID: "blk9", ObservationID: "obs3"}},
			state: goldenState{},
		},
	}
}

func briefGoldens(t *testing.T) map[string][]byte {
	t.Helper()
	guides := promptGuides(t)
	files := map[string][]byte{}
	for _, in := range briefInputs(t) {
		task := in.task
		task.Route, task.Loop, task.Facts, task.Triage = in.state.Task, in.state.Loop, in.state.Facts, in.state.Triage
		if task.Loop {
			task.Hypotheses = Hypotheses(in.state.Ledger)
		}
		var d Digest
		for _, e := range in.state.Digest {
			d.Add(tools.Discovery{Datasource: e.Datasource, Of: e.Of, Names: e.Names})
		}
		msg, err := Render(in.statuses, task, in.view, in.reason, d.Text())
		if err != nil {
			t.Fatalf("%s: %v", in.name, err)
		}
		everyone := func(uid string) bool { return strings.Contains(","+strings.Join(in.everyone, ",")+",", ","+uid+",") }
		structured := Structured(in.statuses, everyone, task, in.view, in.draft)
		g := briefGolden{Name: in.name, Brief: structured, Reason: in.reason, Now: clock(), State: in.state}
		for name, text := range guides {
			if text == structured.Guide.Text {
				g.Guide = name
			}
		}
		if g.Guide == "" {
			t.Fatalf("%s: the guide of its datasources is not one of the prompt goldens", in.name)
		}
		g.Brief.Guide = tools.BriefGuide{Text: "", Schema: json.RawMessage("null")}
		sum := sha256.Sum256([]byte(msg.System))
		g.Want.System = goldenText{Bytes: len(msg.System), SHA256: hex.EncodeToString(sum[:])}
		g.Want.Context, g.Want.User = msg.Context, msg.User
		body, err := json.MarshalIndent(g, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		files[in.name+".json"] = append(body, '\n')
	}
	return files
}

func promptGuides(t *testing.T) map[string]string {
	t.Helper()
	root, err := os.OpenRoot(promptGoldenDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	guides := map[string]string{}
	matches, err := fs.Glob(root.FS(), "guide-*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range matches {
		body, err := root.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		guides[name] = string(body)
	}
	for _, types := range [][]spec.DatasourceType{nil, {spec.Prometheus}} {
		found := false
		for _, text := range guides {
			found = found || text == prompts.GuideFor(types)
		}
		if !found {
			t.Fatalf("the prompt goldens lack the guide of %v; regenerate them first", types)
		}
	}
	return guides
}

func TestBriefGoldensAreCurrent(t *testing.T) {
	files := briefGoldens(t)
	if *updateBriefGoldens {
		if err := os.RemoveAll(briefGoldenDir); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(briefGoldenDir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(briefGoldenDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	if *updateBriefGoldens {
		for name, body := range files {
			if err := root.WriteFile(name, body, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(files) {
		t.Errorf("%s holds %d files, want %d", briefGoldenDir, len(entries), len(files))
	}
	for name, want := range files {
		got, err := root.ReadFile(name)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s is stale; run go test ./internal/agent/brief -run TestBriefGoldensAreCurrent -update", name)
		}
	}
}
