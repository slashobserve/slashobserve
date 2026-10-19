package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const suggestBaseVersion = 2

func suggestedResult(t *testing.T, id string) tools.Result {
	t.Helper()
	panel, err := json.Marshal(map[string]string{"id": id})
	if err != nil {
		t.Fatal(err)
	}
	return tools.Result{
		Content: map[string]any{"suggested": true, "panel_id": id, "panel": json.RawMessage(panel)},
		Step:    "Proposed a change to panel " + id,
	}
}

func (r *rig) suggestTask(t *testing.T) agent.Task {
	t.Helper()
	task := r.reviseTask()
	task.Suggest = true
	task.PanelID = panelID
	task.BaseVersion = suggestBaseVersion
	return task
}

func (r *rig) versionTwo(t *testing.T) {
	t.Helper()
	older, err := spec.Parse([]byte(strings.Replace(mustMarshal(t, r.dash.spec), `"title":"Checkout service health"`, `"title":"Version two"`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if older.Title == r.dash.spec.Title {
		t.Fatalf("the version 2 fixture has the same title as the latest: %q", older.Title)
	}
	r.dash.byNum = map[int]*spec.Dashboard{suggestBaseVersion: older}
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestSuggestRunLoadsTheRequestedBaseVersion(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_dashboard", `{"spec":{}}`))))
	r.versionTwo(t)
	r.tools.onSave("update_dashboard", suggestedResult(t, panelID))

	out, err := r.agent.Run(context.Background(), r.suggestTask(t))
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 0 || !strings.Contains(string(out.Suggestion), panelID) {
		t.Fatalf("outcome = %+v", out)
	}
	calls := r.tools.recorded()
	if len(calls) != 1 || !calls[0].scoped {
		t.Fatalf("calls = %v", r.tools.names())
	}
	run := calls[0].run
	if run.Mode != tools.ModeSuggest || run.PanelID != panelID || run.ParentVersion != suggestBaseVersion || run.DashboardID != dashID {
		t.Fatalf("run = %+v", run)
	}
	if run.Base == nil || run.Base.Title != "Version two" {
		t.Fatalf("the run was not based on version %d: %+v", suggestBaseVersion, run.Base)
	}
	if got := r.provider.started()[0].Context; !strings.Contains(got, fmt.Sprintf("version %d", suggestBaseVersion)) || !strings.Contains(got, `"title":"Version two"`) {
		t.Fatalf("context = %s", got)
	}
	if r.dash.getCount() != 0 {
		t.Fatal("suggest mode must not load the latest version")
	}
}

func TestSuggestRunPlacesTheDraftIntoTheBase(t *testing.T) {
	r := newRig(t, session(toolTurn(call("c1", "update_dashboard", `{"spec":{}}`))))
	entry, ok := r.dash.spec.FindPanel(panelID)
	if !ok {
		t.Fatalf("fixture has no panel %s", panelID)
	}
	draft := *entry.Panel
	draft.Title = "Drafted title"
	r.tools.onSave("update_dashboard", suggestedResult(t, panelID))
	task := r.suggestTask(t)
	task.Draft = json.RawMessage(mustMarshal(t, draft))

	if _, err := r.agent.Run(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	base := r.tools.recorded()[0].run.Base
	drafted, ok := base.FindPanel(panelID)
	if !ok || drafted.Panel.Title != "Drafted title" {
		t.Fatalf("draft not in the base: %+v", drafted)
	}
}

func TestSuggestRunRejectsBadStartingPoints(t *testing.T) {
	placeholder := func(r *rig) json.RawMessage {
		entry, _ := r.dash.spec.FindPanel(panelID)
		return json.RawMessage(fmt.Sprintf(`{"id":%q,"type":"placeholder","title":"Map","gridPos":%s,"reason":"no map","sourceType":"worldmap"}`, panelID, mustMarshal(t, entry.Panel.GridPos)))
	}
	for name, c := range map[string]struct {
		edit    func(r *rig, task *agent.Task)
		want    error
		message string
	}{
		"panel missing from the base": {
			edit: func(_ *rig, task *agent.Task) { task.PanelID = "gone" },
			want: agent.ErrInvalidTask,
		},
		"draft that does not validate": {
			edit: func(_ *rig, task *agent.Task) {
				task.Draft = json.RawMessage(`{"id":"` + panelID + `","type":"stat","title":"SECRET draft title"}`)
			},
			want: agent.ErrDraftInvalid,
		},
		"draft with another id": {
			edit: func(r *rig, task *agent.Task) {
				entry, _ := r.dash.spec.FindPanel(panelID)
				draft := *entry.Panel
				draft.ID = "other"
				task.Draft = json.RawMessage(mustMarshal(t, draft))
			},
			want: agent.ErrDraftInvalid,
		},
		"panel that is not a data or text panel": {
			edit: func(r *rig, task *agent.Task) { task.Draft = placeholder(r) },
			want: agent.ErrInvalidTask,
		},
		"version that does not exist": {
			edit: func(_ *rig, task *agent.Task) { task.BaseVersion = 9 },
			want: dashboards.ErrNotFound,
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, session(toolTurn(call("c1", "update_dashboard", `{"spec":{}}`))))
			task := r.suggestTask(t)
			c.edit(r, &task)

			_, err := r.agent.Run(context.Background(), task)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if len(r.tools.recorded()) != 0 || r.provider.started() != nil {
				t.Fatal("the model must not run when the starting point is wrong")
			}
			if !errors.Is(err, agent.ErrDraftInvalid) && strings.Contains(strings.ToLower(err.Error()), "draft") {
				t.Fatalf("err mentions a draft that was not sent: %v", err)
			}
		})
	}
}

func TestSuggestRunLogsDraftIssueCodesAndPathsOnly(t *testing.T) {
	r := newRig(t, session())
	task := r.suggestTask(t)
	task.Draft = json.RawMessage(`{"id":"` + panelID + `","type":"stat","title":"SECRET draft title"}`)

	if _, err := r.agent.Run(context.Background(), task); !errors.Is(err, agent.ErrDraftInvalid) {
		t.Fatalf("err = %v", err)
	}
	entry := r.logEntry("agent draft panel is not valid")
	codes, _ := entry["issue_codes"].([]any)
	paths, _ := entry["issue_paths"].([]any)
	if len(codes) == 0 || len(codes) != len(paths) || entry["agent_request_id"] != "req-1" || entry["error_code"] != "draft_invalid" {
		t.Fatalf("log entry = %v", entry)
	}
	if strings.Contains(r.logs.String(), "SECRET") {
		t.Fatalf("the log leaks draft content: %s", r.logs.String())
	}
}

func TestSuggestRunStoresSuggestWordedMessages(t *testing.T) {
	rejected := func(n int) []tools.Result {
		out := make([]tools.Result, n)
		for i := range out {
			out[i] = invalidSpecResult("a_code")
			out[i].Step = "Suggestion rejected: 1 issues"
		}
		return out
	}
	exhausted := make([]scriptedTurn, 4)
	limited := make([]scriptedTurn, 25)
	for i := range exhausted {
		exhausted[i] = toolTurn(call(fmt.Sprint("c", i), "update_dashboard", `{}`))
	}
	for i := range limited {
		limited[i] = toolTurn(call(fmt.Sprint("c", i), "get_spec_guide", `{}`))
	}
	for name, c := range map[string]struct {
		turns []scriptedTurn
		save  []tools.Result
		want  string
	}{
		"repair rounds used up": {
			turns: exhausted,
			save:  rejected(4),
			want:  "the proposed panel still failed validation after 3 repair rounds (last issues: a_code)",
		},
		"turn limit": {
			turns: limited,
			want:  "the agent stopped without proposing a panel",
		},
		"no proposal": {
			turns: []scriptedTurn{stopTurn(llm.StopEndTurn)},
			want:  agent.ErrNoSuggestion.Error(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, session(c.turns...))
			if c.save != nil {
				r.tools.onSave("update_dashboard", c.save...)
			}
			req := revisionRequest()
			req.Source = revisions.SourceSuggest
			req.BaseVersion = suggestBaseVersion

			_, err := agent.NewReviser(r.agent, nil).Revise(context.Background(), req)
			if err == nil || err.Error() != c.want {
				t.Fatalf("stored error = %v, want %q", err, c.want)
			}
		})
	}
}
