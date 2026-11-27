package revisions

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

type BuildState string

const (
	BuildNone         BuildState = "none"
	BuildBuilding     BuildState = "building"
	BuildStopped      BuildState = "stopped"
	BuildStoppedEmpty BuildState = "stopped_empty"
	BuildStale        BuildState = "stale"
	BuildKept         BuildState = "kept"
	BuildMoved        BuildState = "moved"
	BuildCommitted    BuildState = "committed"
	BuildProposed     BuildState = "proposed"
)

var BuildStates = lifecycle.Derived{Name: "build", States: []lifecycle.DerivedState{
	{Name: string(BuildNone), When: "The run holds no build and none of the states below applies; a stopped build deleted after its 7 days also reads `none`."},
	{Name: string(BuildBuilding), When: "The run has not finished and the agent may still change the build."},
	{Name: string(BuildStopped), When: "The run failed or was canceled and left a build with at least one panel; its requester can keep or continue it until `build.expires_at`."},
	{Name: string(BuildStoppedEmpty), When: "The run failed or was canceled and left a build with no panels, which cannot be kept."},
	{Name: string(BuildStale), When: "A stopped build whose dashboard has a newer version than the one it started from: it can no longer be kept, and Continue moves it onto the newer version."},
	{Name: string(BuildKept), When: "The requester saved the stopped build as a version (`build_saved_at` is set)."},
	{Name: string(BuildMoved), When: "Another run continues this one and took the build (`continued_by` names it)."},
	{Name: string(BuildCommitted), When: "The run finished and its build became the version in `result_version`."},
	{Name: string(BuildProposed), When: "The run finished and its build became a proposal."},
}}

type buildFacts struct {
	run    State
	result ResultKind
	held   bool
	panels int
	stale  bool
	kept   bool
	moved  bool
}

func buildStateOf(f buildFacts) BuildState {
	stopped := f.run == StateFailed || f.run == StateCanceled
	switch {
	case f.kept:
		return BuildKept
	case f.held && !f.run.Terminal():
		return BuildBuilding
	case f.held && stopped:
		return stoppedBuildState(f)
	case f.moved:
		return BuildMoved
	case f.result == ResultProposal:
		return BuildProposed
	case f.result == ResultVersion:
		return BuildCommitted
	}
	return BuildNone
}

func stoppedBuildState(f buildFacts) BuildState {
	switch {
	case f.panels == 0:
		return BuildStoppedEmpty
	case f.stale:
		return BuildStale
	}
	return BuildStopped
}

func buildFactsOf(row dbsqlc.ListAgentRequestBuildStateRow) buildFacts {
	return buildFacts{
		run:    State(row.RunState),
		result: ResultKind(row.ResultKind),
		held:   row.Saves.Valid,
		panels: int(row.Panels.Int32),
		stale:  row.BaseVersion.Valid && row.LatestVersion > row.BaseVersion.Int32,
		kept:   row.BuildSavedAt.Valid,
		moved:  row.ContinuedBy.Valid,
	}
}

func buildExpiry(row dbsqlc.ListAgentRequestBuildStateRow) *string {
	if !row.Saves.Valid || !row.FinishedAt.Valid || !State(row.RunState).Terminal() {
		return nil
	}
	return httpapi.FormatOptionalTime(new(row.FinishedAt.Time.Add(BuildRetentionDays * 24 * time.Hour)))
}

func (s buildState) derived() BuildState {
	if s.state == "" {
		return BuildNone
	}
	return s.state
}

const (
	reasonRunNotFinished = "run_not_finished"
	reasonNoPanels       = "no_panels"
	reasonNoStoppedBuild = "no_stopped_build"
)

type buildRefusalJSON struct {
	Error          string     `json:"error"`
	Code           string     `json:"code"`
	Message        string     `json:"message"`
	State          BuildState `json:"state"`
	AllowedActions []string   `json:"allowed_actions"`
	Reason         string     `json:"reason"`
}

func buildRefusalFor(err error, state BuildState, allowed []Action) (buildRefusalJSON, bool) {
	var reason string
	switch {
	case errors.Is(err, ErrBuildRunning):
		reason = reasonRunNotFinished
	case errors.Is(err, ErrBuildEmpty):
		reason = reasonNoPanels
	case errors.Is(err, ErrBuildNotSavable):
		reason = reasonNoStoppedBuild
	default:
		return buildRefusalJSON{}, false
	}
	reply, _ := ReplyFor(err)
	return buildRefusalJSON{
		Error:          reply.Message,
		Code:           reply.Code,
		Message:        reply.Message,
		State:          state,
		AllowedActions: actionStrings(allowed),
		Reason:         reason,
	}, true
}

func (h *ObserveHandler) failBuildSave(c *gin.Context, row dbsqlc.AgentRequest, err error, idAttr any) {
	if body, ok := h.buildRefusal(c.Request.Context(), row, httpapi.Caller(c), err); ok {
		c.AbortWithStatusJSON(http.StatusConflict, body)
		return
	}
	h.failBuild(c, "save stopped agent request build", err, idAttr)
}

func (h *ObserveHandler) buildRefusal(ctx context.Context, row dbsqlc.AgentRequest, caller auth.Principal, err error) (buildRefusalJSON, bool) {
	if _, refused := buildRefusalFor(err, BuildNone, nil); !refused || !row.ID.Valid || !isRequester(row, caller) {
		return buildRefusalJSON{}, false
	}
	current, readErr := h.service.db.Queries().GetAgentRequestByID(ctx, row.ID)
	if readErr != nil {
		return buildRefusalJSON{}, false
	}
	states, readErr := h.service.buildStates(ctx, []pgtype.UUID{row.ID})
	if readErr != nil {
		return buildRefusalJSON{}, false
	}
	facts, readErr := h.service.factsFor(ctx, current, states[row.ID], caller)
	if readErr != nil {
		return buildRefusalJSON{}, false
	}
	return buildRefusalFor(err, states[row.ID].derived(), allowedActions(facts))
}
