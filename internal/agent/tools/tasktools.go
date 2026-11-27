package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/auth"
)

const (
	nameListMoreTools = "list_more_tools"

	descListMoreTools = "Offer every investigation tool for the rest of this run, when the tools you were given cannot answer the question. It reads nothing and returns the names of the tools it adds; call them in your next call."

	schemaListMoreTools = `{"type":"object","properties":{},"additionalProperties":false}`

	stepAllTools = "Offered every investigation tool for the rest of the run"
)

var taskBasics = []string{
	nameListServices, nameListLabels, nameListLabelValues, nameSaveFinding, nameNoteHypotheses,
	nameListMCPSources, nameFindMCPTools, nameCallMCPTool, nameListMoreTools,
}

var taskTools = map[route.Task][]string{
	route.Measure:   {nameListMetrics, nameMeasure, nameAggregate, nameQueryRange, nameQueryInstant},
	route.Logs:      {nameQueryRange, nameAggregate, nameLogPatterns, nameLogFacets, nameLogContext, nameSampleRows},
	route.Traces:    {nameTraceStats, nameTraceErrorOrigins, nameTraceSlowPath, nameGetTrace, nameCompareTraces, nameLogsForTrace},
	route.Incident:  {nameListMetrics, nameListAnnotations, nameMeasure, nameQueryRange, nameTraceErrorOrigins, nameTraceSlowPath, nameLogPatterns, nameGetTrace},
	route.Inventory: {nameListDatasources, nameListMetrics, nameListDashboards, nameListObservations, nameSearchWorkspace},
	route.Explain:   {nameGetDashboard, nameGetPanels, nameGetObservation, nameListDashboards, nameSearchWorkspace, nameGetSpecGuide, nameQueryRange},
}

func subjectTools(run Run) []string {
	var names []string
	if run.DashboardID != "" {
		names = append(names, nameGetPanels)
	}
	if len(run.Observations) > 0 {
		names = append(names, nameGetObservation)
	}
	return names
}

func (run Run) narrowed() bool {
	return run.Mode == ModeInvestigate && run.Task != route.Full && !run.AllTools
}

func (t Tool) InTask(run Run) bool {
	if t.Name == nameListMoreTools {
		return run.Task != route.Full
	}
	if !run.narrowed() {
		return true
	}
	return slices.Contains(taskBasics, t.Name) || slices.Contains(taskTools[run.Task], t.Name) || slices.Contains(subjectTools(run), t.Name)
}

func TaskTools(task route.Task) []string {
	if task == route.Full {
		return nil
	}
	return slices.Concat(taskBasics, taskTools[task])
}

func (r *Registry) taskToolNames(run Run) []string {
	var names []string
	for _, t := range r.List() {
		if t.OfferedInRun(run) {
			names = append(names, t.Name)
		}
	}
	return names
}

func (r *Registry) notInTask(t Tool, run Run) Result {
	res := toolError(codeUnknownTool, fmt.Sprintf("unknown tool %q.\nThis run is a %s question and offers %s; call %s to offer every investigation tool for the rest of the run.",
		t.Name, run.Task.Name(), strings.Join(r.taskToolNames(run), ", "), nameListMoreTools))
	res.Step = "Refused " + t.Name + ": not a tool of the " + run.Task.Name() + " task type"
	return res
}

func init() { registerEntries((*Registry).taskEntries) }

func (r *Registry) taskEntries() []entry {
	return []entry{
		{Tool{Name: nameListMoreTools, Description: descListMoreTools, InputSchema: json.RawMessage(schemaListMoreTools), AgentOnly: true, ReadOnly: true, OnlyIn: ModeInvestigate, MinRole: auth.RoleViewer}, r.listMoreTools},
	}
}

type moreToolsOut struct {
	Task  string   `json:"task"`
	Added []string `json:"added"`
}

func (r *Registry) listMoreTools(ctx context.Context, _ json.RawMessage) (Result, error) {
	run, _ := RunFrom(ctx)
	out := moreToolsOut{Task: run.Task.Name(), Added: []string{}}
	if run.narrowed() {
		offered := r.taskToolNames(run)
		widened := run
		widened.AllTools = true
		for _, name := range r.taskToolNames(widened) {
			if !slices.Contains(offered, name) {
				out.Added = append(out.Added, name)
			}
		}
	}
	res := success(stepAllTools, out)
	res.AllTools = true
	return res, nil
}
