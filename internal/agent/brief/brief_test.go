package brief

import (
	"strings"
	"testing"

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
