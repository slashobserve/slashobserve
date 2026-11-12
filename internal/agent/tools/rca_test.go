package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type failingFreezer struct{}

func (failingFreezer) Freeze(context.Context, auth.Principal, *spec.Dashboard, string, time.Time, time.Time) (observations.Frozen, error) {
	return observations.Frozen{}, errors.New("the datasource is down")
}

func (r *observationRig) published(t *testing.T, by auth.Principal, id string) rcaPublishedOut {
	t.Helper()
	res := r.call(t, by, namePublishRCA, fmt.Sprintf(`{"observation_id":%q}`, id))
	out, ok := res.Content.(rcaPublishedOut)
	if res.IsError || !ok || out.Code != "ok" {
		t.Fatalf("publish_rca: %#v", res.Content)
	}
	return out
}

func TestPublishRCAGivesRevisionOneThenTwo(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	r.written(t, editorCaller, nameAddBlock, textBlockInput(id, "Errors began at 09:12"))
	r.written(t, editorCaller, nameAddBlock, panelBlockInput(id, evidencePanel("Errors", "sum(rate(errors[5m]))")))

	first := r.published(t, editorCaller, id)
	if first.Revision != 1 || first.Blocks != 2 || first.Snapshots != 1 || first.ObservationID != id || !strings.HasSuffix(first.ObservationURL, "/o/"+id) {
		t.Fatalf("first = %#v", first)
	}
	r.written(t, editorCaller, nameAddBlock, textBlockInput(id, "Only checkout fails"))
	second := r.published(t, editorCaller, id)
	if second.Revision != 2 || second.Blocks != 3 || second.Snapshots != 1 {
		t.Fatalf("second = %#v", second)
	}
}

func TestPublishRCARefusesWhatThePersonMayNotDo(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	r.written(t, editorCaller, nameAddBlock, textBlockInput(id, "a note"))
	personal := r.written(t, editorCaller, nameCreateObservation, `{"title":"Private"}`).ObservationID

	tests := []struct {
		name  string
		by    auth.Principal
		input string
		code  string
	}{
		{"a commenter", commenterCaller, fmt.Sprintf(`{"observation_id":%q}`, id), codeForbiddenRole},
		{"a viewer", viewerCaller, fmt.Sprintf(`{"observation_id":%q}`, id), codeForbiddenRole},
		{"an observation in My Dashboards", editorCaller, fmt.Sprintf(`{"observation_id":%q}`, personal), codePersonalFolder},
		{"an unknown observation", editorCaller, `{"observation_id":"11111111-1111-4111-8111-111111111111"}`, codeNotFound},
		{"an id that is not a UUID", editorCaller, `{"observation_id":"checkout"}`, codeInvalidRequest},
		{"a missing id", editorCaller, `{}`, codeInvalidRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if code := errorCode(t, r.call(t, tc.by, namePublishRCA, tc.input)); code != tc.code {
				t.Errorf("code = %q, want %q", code, tc.code)
			}
		})
	}
}

func TestPublishRCANamesThePanelBlockThatFailed(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	r.written(t, editorCaller, nameAddBlock, textBlockInput(id, "a note"))
	panel := r.written(t, editorCaller, nameAddBlock, panelBlockInput(id, evidencePanel("Errors", "up")))
	r.registry.deps.Observations = observations.New(r.db.DB, folders.NewAccess(r.db.DB), failingFreezer{}, nil, slog.New(slog.DiscardHandler))

	res := r.call(t, editorCaller, namePublishRCA, fmt.Sprintf(`{"observation_id":%q}`, id))
	body, _ := res.Content.(errorBody)
	if errorCode(t, res) != observations.CodePanelFailed || !strings.Contains(body.Message, panel.BlockID) || !strings.Contains(body.Message, "position 1") {
		t.Errorf("a failing panel: %#v", res.Content)
	}
	if strings.Contains(body.Message, "the datasource is down") {
		t.Errorf("the message carries the upstream error: %s", body.Message)
	}
}

func TestPublishRCAIsAViewerToolAndTellsThatPeopleMakeLinks(t *testing.T) {
	tool, ok := toolsByName(t)[namePublishRCA]
	if !ok {
		t.Fatal("publish_rca is not registered")
	}
	if !tool.MCPOnly || tool.MinRole != auth.RoleViewer || tool.ReadOnly {
		t.Errorf("publish_rca = MCPOnly %v, MinRole %s, ReadOnly %v", tool.MCPOnly, tool.MinRole, tool.ReadOnly)
	}
	if tool.OfferedIn(ModeCreate) || tool.OfferedIn(ModeInvestigate) {
		t.Error("publish_rca is offered to the built-in agent")
	}
	if len(tool.Description) > agentToolTextCap {
		t.Errorf("description is %d bytes", len(tool.Description))
	}
	if !strings.Contains(tool.Description, "a person creates links in the web app") {
		t.Errorf("description does not say who makes links: %s", tool.Description)
	}
}

func TestPublishRCANeedsObservations(t *testing.T) {
	ctx := auth.WithPrincipal(t.Context(), editorCaller)
	res, err := New(Deps{}).Call(ctx, namePublishRCA, []byte(`{"observation_id":"11111111-1111-4111-8111-111111111111"}`))
	if err != nil || errorCode(t, res) != FailureToolFailed {
		t.Errorf("res = %#v, err = %v", res.Content, err)
	}
}
