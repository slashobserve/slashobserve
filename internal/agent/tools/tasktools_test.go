package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
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

func TestEachTaskTypeOffersItsOwnTools(t *testing.T) {
	want := map[route.Task]string{
		route.Measure:   "call_mcp_tool,find_mcp_tools,list_label_values,list_labels,list_mcp_sources,list_metrics,list_more_tools,list_services,measure_aggregate,measure_window,note_hypotheses,query_instant,query_range,save_finding",
		route.Logs:      "call_mcp_tool,find_mcp_tools,list_label_values,list_labels,list_mcp_sources,list_metrics,list_more_tools,list_services,log_context,log_facets,log_patterns,measure_aggregate,note_hypotheses,query_range,sample_rows,save_finding",
		route.Traces:    "call_mcp_tool,compare_traces,find_mcp_tools,get_trace,list_label_values,list_labels,list_mcp_sources,list_metrics,list_more_tools,list_services,logs_for_trace,note_hypotheses,save_finding,trace_error_origins,trace_slow_path,trace_stats",
		route.Incident:  "call_mcp_tool,find_mcp_tools,get_trace,list_annotations,list_label_values,list_labels,list_mcp_sources,list_metrics,list_more_tools,list_services,log_patterns,measure_window,note_hypotheses,query_range,save_finding,trace_error_origins,trace_slow_path,trace_stats",
		route.Inventory: "call_mcp_tool,find_mcp_tools,list_dashboards,list_datasources,list_label_values,list_labels,list_mcp_sources,list_metrics,list_more_tools,list_observations,list_services,note_hypotheses,save_finding,search_workspace",
		route.Explain:   "call_mcp_tool,find_mcp_tools,get_dashboard,get_observation,get_panels,get_spec_guide,list_dashboards,list_label_values,list_labels,list_mcp_sources,list_metrics,list_more_tools,list_services,note_hypotheses,query_range,save_finding,search_workspace",
	}
	r := New(Deps{LedgerTool: true})
	for _, task := range route.Tasks {
		got := offeredNames(r, Run{Mode: ModeInvestigate, Investigation: true, Task: task})
		slices.Sort(got)
		if strings.Join(got, ",") != want[task] {
			t.Errorf("%s offers %s", task, strings.Join(got, ","))
		}
		if len(got) < 8 || len(got) > 18 {
			t.Errorf("%s offers %d tools", task, len(got))
		}
	}
}

func TestEveryToolATaskPromptNamesIsInThatTasksSet(t *testing.T) {
	r := New(Deps{LedgerTool: true})
	words := regexp.MustCompile(`[a-z]+(?:_[a-z]+)+`)
	for _, task := range route.Tasks {
		run := Run{Mode: ModeInvestigate, Investigation: true, Task: task, DashboardID: "d", Observations: []string{"o"}}
		offered := offeredNames(r, run)
		for _, word := range words.FindAllString(prompts.InvestigateBody(prompts.Variant{Task: task, Ledger: true}), -1) {
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
