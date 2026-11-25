package brief

import (
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var statuses = []datasource.Status{{UID: "prom", Type: "prometheus", Healthy: true}}

func renderCases(t *testing.T) map[string]struct {
	task  Task
	views []View
} {
	t.Helper()
	build, view := buildOf(t)
	current := &Revision{DashboardID: "d1", Number: 3, Spec: view.Spec}
	return map[string]struct {
		task  Task
		views []View
	}{
		"create":      {Task{Mode: tools.ModeCreate, Prompt: "make a dashboard"}, []View{{}, {Build: build}}},
		"revise":      {Task{Mode: tools.ModeRevise, Source: revisions.SourcePrompt, Prompt: "change a", PanelID: "a"}, []View{{Current: current}, {Current: current, Build: build}}},
		"suggest":     {Task{Mode: tools.ModeSuggest, Prompt: "change a", PanelID: "a"}, []View{{Current: current}}},
		"investigate": {Task{Mode: tools.ModeInvestigate, Prompt: "why is a slow", PanelID: "a"}, []View{{}, {Current: current}}},
	}
}

func TestRenderUserMessageIsTheJoinedContextAndPromptForEveryModeAndReason(t *testing.T) {
	for name, tc := range renderCases(t) {
		for _, reason := range append([]Reason{Fresh}, reasons...) {
			for i, v := range tc.views {
				got, err := Render(statuses, tc.task, v, reason, "")
				if err != nil {
					t.Fatalf("%s/%q/%d: %v", name, reason, i, err)
				}
				want := llm.Request{Prompt: tc.task.Prompt, Context: got.Context}.UserMessage()
				if got.User != want {
					t.Fatalf("%s/%q/%d: user message\n%s\nwant\n%s", name, reason, i, got.User, want)
				}
				if !strings.HasPrefix(got.Context, "Configured datasources:\n- prom (prometheus): healthy\n") || !strings.HasSuffix(got.User, "\n<request>\n"+tc.task.Prompt+"\n</request>") {
					t.Fatalf("%s/%q/%d: unexpected message %q", name, reason, i, got.User)
				}
			}
		}
	}
}

func TestRenderPicksTheSystemTextOfTheMode(t *testing.T) {
	cases := renderCases(t)
	build := prompts.Build([]spec.DatasourceType{spec.Prometheus})
	for name, want := range map[string]string{"create": build, "revise": build, "suggest": build, "investigate": prompts.Investigate()} {
		tc := cases[name]
		got, err := Render(statuses, tc.task, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		if got.System != want {
			t.Fatalf("%s: system text is not the one of its mode", name)
		}
	}
}

func TestRenderAppendsTheDigestToEveryModeButSuggest(t *testing.T) {
	const digest = "<discovered>\n- prom metrics: up\n</discovered>"
	for name, tc := range renderCases(t) {
		got, err := Render(statuses, tc.task, tc.views[0], Fresh, digest)
		if err != nil {
			t.Fatal(err)
		}
		appended := strings.HasSuffix(got.Context, "\n\n"+digest)
		if appended == (name == "suggest") {
			t.Fatalf("%s: digest appended = %v", name, appended)
		}
	}
}

func TestRenderNamesThePersonsTimeZoneToEveryModeButSuggest(t *testing.T) {
	const note = "Time zone: the person reads times in Asia/Kolkata, labelled IST. Tool results give UTC; write every time in the answer in Asia/Kolkata with its label, such as 14:05 IST, not in UTC."
	for name, tc := range renderCases(t) {
		task := tc.task
		task.TimeZone = "Asia/Kolkata"
		got, err := Render(statuses, task, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		if named := strings.Contains(got.Context, note); named == (name == "suggest") {
			t.Fatalf("%s: zone named = %v in\n%s", name, named, got.Context)
		}
		without, err := Render(statuses, tc.task, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(without.Context, "Time zone:") {
			t.Fatalf("%s: a run with no zone named one", name)
		}
	}
}

func TestRenderShowsTheBuildAsTheViewWhenOneExists(t *testing.T) {
	build, _ := buildOf(t)
	current := &Revision{DashboardID: "d1", Number: 3, Spec: &spec.Dashboard{Title: "old"}}
	got, err := Render(statuses, Task{Mode: tools.ModeRevise, Prompt: "p"}, View{Current: current, Build: build}, Restart, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Context, "You are revising dashboard d1, currently version 3. This is your build of the revision") || strings.Contains(got.Context, `"title":"old"`) {
		t.Fatalf("context does not show the build:\n%s", got.Context)
	}
}

func TestParseReasonKnowsEveryReasonAndNothingElse(t *testing.T) {
	for _, s := range []string{"", "restart", "output_limit", "pending", "conflict", "continue", "renewed", "call_timeout", "no_finding", "no_answer"} {
		if got, ok := ParseReason(s); !ok || string(got) != s {
			t.Fatalf("ParseReason(%q) = %q, %v", s, got, ok)
		}
	}
	for _, s := range []string{"fresh", "Restart", "unknown", " "} {
		if _, ok := ParseReason(s); ok {
			t.Fatalf("ParseReason(%q) accepted", s)
		}
	}
}

func TestRenderTellsAnAsOfRunItsNow(t *testing.T) {
	const note = "Now is 2026-11-20T10:30:00Z for this run: every relative time, such as now-1h, resolves to it, and the data may end there."
	at := time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)
	for name, tc := range renderCases(t) {
		task := tc.task
		task.AsOf = &at
		got, err := Render(statuses, task, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Context, note) {
			t.Fatalf("%s: no as_of note in\n%s", name, got.Context)
		}
		without, err := Render(statuses, tc.task, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(without.Context, "for this run:") {
			t.Fatalf("%s: a live run was given a now", name)
		}
	}
}

func TestRenderGivesTheCurrentTimeInThePersonsZone(t *testing.T) {
	at := time.Date(2026, 11, 25, 5, 29, 0, 0, time.UTC)
	task := Task{Mode: tools.ModeInvestigate, Prompt: "why is a slow", TimeZone: "Asia/Kolkata", AsOf: &at}
	got, err := Render(statuses, task, View{}, Fresh, "")
	if err != nil {
		t.Fatal(err)
	}
	const want = "It is now 10:59 IST on 2026-11-25, which is 2026-11-25 05:29 UTC; take your window from that time and state the window you queried."
	if !strings.Contains(got.Context, want) {
		t.Fatalf("context lacks %q:\n%s", want, got.Context)
	}
}

func TestRenderGivesTheCurrentTimeInUTCWhenNoZoneIsKnownOrTheZoneIsUTC(t *testing.T) {
	at := time.Date(2026, 11, 25, 5, 29, 0, 0, time.UTC)
	const want = "It is now 2026-11-25 05:29 UTC; take your window from that time and state the window you queried."
	for name, tc := range renderCases(t) {
		task := tc.task
		task.AsOf = &at
		got, err := Render(statuses, task, tc.views[0], Fresh, "")
		if err != nil {
			t.Fatal(err)
		}
		if has := strings.Contains(got.Context, want); has == (name == "suggest") {
			t.Fatalf("%s: UTC time named = %v in\n%s", name, has, got.Context)
		}
		if strings.Contains(got.Context, "Time zone:") {
			t.Fatalf("%s: a run with no zone named one", name)
		}
	}

	utc := Task{Mode: tools.ModeInvestigate, Prompt: "why is a slow", TimeZone: "UTC", AsOf: &at}
	got, err := Render(statuses, utc, View{}, Fresh, "")
	if err != nil {
		t.Fatal(err)
	}
	const inUTC = "It is now 05:29 UTC on 2026-11-25; take your window from that time and state the window you queried."
	if !strings.Contains(got.Context, inUTC) || strings.Contains(got.Context, "which is") {
		t.Fatalf("context for a UTC zone:\n%s", got.Context)
	}
}

func TestRenderCallsTheDefaultInvestigationWindowADefault(t *testing.T) {
	task := Task{Mode: tools.ModeInvestigate, Prompt: "why is a slow"}
	got, err := Render(statuses, task, View{}, Fresh, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Window: the last hour (a default the question may need to exceed)"; !strings.Contains(got.Context, want) {
		t.Fatalf("context lacks %q:\n%s", want, got.Context)
	}
}
