package tools

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/auth"
)

func offeredNames(r *Registry, run Run) []string {
	var names []string
	for _, tool := range r.List() {
		if tool.OfferedInRun(run) {
			names = append(names, tool.Name)
		}
	}
	return names
}

const taskToolsFixture = "../../../spec/examples/agent/route/task-tools.json"

var updateTaskTools = flag.Bool("update", false, "rewrite spec/examples/agent/route/task-tools.json")

type taskToolsCase struct {
	Task    route.Task `json:"task"`
	Subject struct {
		Dashboard   bool `json:"dashboard"`
		Observation bool `json:"observation"`
	} `json:"subject"`
	Want []string `json:"want"`
}

type taskToolsCases struct {
	All   []string        `json:"all"`
	Cases []taskToolsCase `json:"cases"`
}

func (c taskToolsCase) run() Run {
	run := Run{Mode: ModeInvestigate, Investigation: true, Task: c.Task}
	if c.Subject.Dashboard {
		run.DashboardID = "d"
	}
	if c.Subject.Observation {
		run.Observations = []string{"o"}
	}
	return run
}

func currentTaskTools(r *Registry) taskToolsCases {
	var out taskToolsCases
	for _, tool := range r.List() {
		if tool.OfferedInMode(Run{Mode: ModeInvestigate, Investigation: true}) {
			out.All = append(out.All, tool.Name)
		}
	}
	for _, task := range append([]route.Task{route.Full}, route.Tasks...) {
		for _, subject := range []bool{false, true} {
			c := taskToolsCase{Task: task}
			c.Subject.Dashboard, c.Subject.Observation = subject, subject
			c.Want = offeredNames(r, c.run())
			out.Cases = append(out.Cases, c)
		}
	}
	return out
}

func TestEachTaskTypeOffersItsOwnTools(t *testing.T) {
	r := New(Deps{LedgerTool: true})
	if *updateTaskTools {
		body, err := json.MarshalIndent(currentTaskTools(r), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(taskToolsFixture, append(body, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(taskToolsFixture)
	if err != nil {
		t.Fatal(err)
	}
	var want taskToolsCases
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	got := currentTaskTools(r)
	if !slices.Equal(got.All, want.All) {
		t.Errorf("an investigate run offers %s", strings.Join(got.All, ","))
	}
	if len(want.Cases) != len(got.Cases) {
		t.Fatalf("the fixture has %d cases, want %d", len(want.Cases), len(got.Cases))
	}
	for _, c := range want.Cases {
		names := offeredNames(r, c.run())
		if !slices.Equal(names, c.Want) {
			t.Errorf("%s (subject %v) offers %s", c.Task.Name(), c.Subject.Dashboard, strings.Join(names, ","))
		}
		if c.Task != route.Full && !c.Subject.Dashboard && (len(names) < 8 || len(names) > 18) {
			t.Errorf("%s offers %d tools", c.Task.Name(), len(names))
		}
	}
}

func TestEveryToolATaskPromptNamesIsInThatTasksSet(t *testing.T) {
	r := New(Deps{LedgerTool: true})
	words := regexp.MustCompile(`[a-z]+(?:_[a-z]+)+`)
	for _, task := range route.Tasks {
		run := Run{Mode: ModeInvestigate, Investigation: true, Task: task, DashboardID: "d", Observations: []string{"o"}}
		offered := offeredNames(r, run)
		text := runText(t, "investigate.md") + runText(t, "playbook/"+string(task)+".md") + runText(t, "ledger.md")
		for _, word := range words.FindAllString(text, -1) {
			if _, isTool := r.entries[word]; isTool && !slices.Contains(offered, word) {
				t.Errorf("the %s prompt names %s, which the task does not offer", task, word)
			}
		}
	}
}

func TestATaskTypeNeverWidensWhatItsModeOffers(t *testing.T) {
	r := New(Deps{LedgerTool: true})
	for _, task := range route.Tasks {
		for _, base := range []Run{{Mode: ModeInvestigate, Investigation: true}, privateChat(), threadAsk()} {
			narrowed := base
			narrowed.Task = task
			full := offeredNames(r, base)
			for _, name := range offeredNames(r, narrowed) {
				if name != nameListMoreTools && !slices.Contains(full, name) {
					t.Errorf("%s offers %s, which its mode does not", task, name)
				}
			}
			for _, name := range TaskTools(task) {
				if _, ok := r.entries[name]; !ok {
					t.Errorf("%s names %s, which is not a tool", task, name)
				}
			}
		}
		for _, mode := range []Mode{ModeCreate, ModeRevise, ModeSuggest} {
			run := Run{Mode: mode, Task: task, ChatID: "chat"}
			untouched := run
			untouched.Task = route.Full
			if a, b := offeredNames(r, run), offeredNames(r, untouched); !slices.Equal(a, b) {
				t.Errorf("%s narrows a %s run", task, mode)
			}
		}
	}
}

func TestTheFullTaskAndAWidenedRunOfferEveryInvestigationTool(t *testing.T) {
	r := New(Deps{LedgerTool: true})
	full := offeredNames(r, Run{Mode: ModeInvestigate, Investigation: true})
	if slices.Contains(full, nameListMoreTools) {
		t.Error("a run with every tool still offers list_more_tools")
	}
	for _, task := range route.Tasks {
		widened := offeredNames(r, Run{Mode: ModeInvestigate, Investigation: true, Task: task, AllTools: true})
		if got := slices.DeleteFunc(widened, func(n string) bool { return n == nameListMoreTools }); !slices.Equal(got, full) {
			t.Errorf("%s widened offers %d tools, the full set is %d", task, len(got), len(full))
		}
	}
}

func inRunCtx(run Run) context.Context {
	return auth.WithPrincipal(WithRun(context.Background(), run), auth.Principal{Kind: auth.KindUser, ID: "u1", Role: auth.RoleAdmin})
}

func TestAToolOutsideTheTaskIsUnknownAndNamesTheTasksTools(t *testing.T) {
	r := New(Deps{})
	run := Run{Mode: ModeInvestigate, Investigation: true, Task: route.Measure}
	res, err := r.Call(inRunCtx(run), nameLogPatterns, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	eb, ok := res.Content.(errorBody)
	if !res.IsError || !ok || eb.Code != codeUnknownTool {
		t.Fatalf("res = %+v", res)
	}
	for _, want := range []string{`unknown tool "log_patterns"`, "This run is a measure question and offers ", "measure_window", "call list_more_tools"} {
		if !strings.Contains(eb.Message, want) {
			t.Errorf("message lacks %q: %s", want, eb.Message)
		}
	}
	if res.Step != "Refused log_patterns: not a tool of the measure task type" {
		t.Errorf("step = %q", res.Step)
	}
}

func TestListMoreToolsWidensTheRun(t *testing.T) {
	r := New(Deps{})
	run := Run{Mode: ModeInvestigate, Investigation: true, Task: route.Measure}
	res, err := r.Call(inRunCtx(run), nameListMoreTools, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	out, ok := res.Content.(moreToolsOut)
	if res.IsError || !ok || !res.AllTools || res.Step != stepAllTools {
		t.Fatalf("res = %+v", res)
	}
	if out.Task != "measure" || !slices.Contains(out.Added, nameLogPatterns) || slices.Contains(out.Added, nameMeasure) {
		t.Fatalf("out = %+v", out)
	}
	run.AllTools = true
	if res, _ := r.Call(inRunCtx(run), nameLogPatterns, json.RawMessage(`{}`)); res.IsError {
		if eb, _ := res.Content.(errorBody); eb.Code == codeUnknownTool {
			t.Fatalf("a widened run still refuses log_patterns: %+v", res)
		}
	}
}

func runText(t *testing.T, name string) string {
	t.Helper()
	root, err := os.OpenRoot("../prompts")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	text, err := root.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}
