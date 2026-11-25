package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	nameListRuns   = "list_runs"
	nameRenameRun  = "rename_run"
	namePinRun     = "pin_run"
	nameUnpinRun   = "unpin_run"
	nameArchiveRun = "archive_run"

	defaultListRuns = 20
	maxListRuns     = 50

	descListRuns   = "List the runs of the built-in agent you started, which the web app shows as chats, newest first with pinned ones on top, 20 per page by default. Keep runs by state, dashboard_id, observation_id (asked about or written into), pinned or anchored true (only your questions that point at a spot, scope mine only), or by q, a word of the title, the prompt or an answer you may read. scope all lists every person's runs and needs the Admin role. Each has its id, title, kind, intent (ask, apply or suggest), investigation (true for an investigation that writes into an observation, false for an ask), state, result_kind, dashboard, observation, anchor and block_id (the spot your own question points at, null for anything else), pinned_at, created_at and allowed_actions; read one with get_run. Page with next_cursor."
	descRenameRun  = "Give one of your runs a title, 1 to 120 characters on one line, as the Rename action of a chat does; null takes the title back to the start of the prompt. Only the person or key that started the run can rename it: anyone else gets forbidden on a run they can see on a dashboard and not_found on any other. Renaming the first run of a chat with no dashboard also renames the one observation made from that chat; observation_rename in the result is renamed, or kept when you may not edit that observation. Safe to repeat."
	descPinRun     = "Pin one of your runs to the top of list_runs. Only the person or key that started the run can pin it: anyone else gets forbidden on a run they can see on a dashboard and not_found on any other. Safe to repeat."
	descUnpinRun   = "Unpin one of your runs so list_runs puts it back by date. Only the person or key that started the run can unpin it: anyone else gets forbidden on a run they can see on a dashboard and not_found on any other. Safe to repeat."
	descArchiveRun = "Archive one of your runs so it leaves list_runs, as Delete does for a chat in the web app; the run stays readable with get_run and nothing it saved is deleted: an observation it saved stays, and only a person can delete that in the web app or with observe requests archive --delete-observation. Only the person or key that started the run can archive it: anyone else gets forbidden on a run they can see on a dashboard and not_found on any other. Safe to repeat."

	schemaListRuns = `{"type":"object","properties":{` +
		`"scope":{"type":"string","enum":["mine","all"],"description":"mine (default) lists your own runs; all lists everyone's and needs the Admin role."},` +
		`"state":{"type":"array","maxItems":6,"items":{"type":"string","enum":["queued","running","canceling","succeeded","failed","canceled"]},"description":"Only runs in one of these states."},` +
		`"q":{"type":"string","minLength":1,"maxLength":200,"description":"Only runs whose title, prompt or readable answer contains this text, ignoring case."},` +
		`"pinned":{"type":"boolean","description":"true keeps only pinned runs, false only unpinned ones."},` +
		`"dashboard_id":{"type":"string","format":"uuid","description":"Only runs about this dashboard, from list_dashboards."},` +
		`"observation_id":{"type":"string","format":"uuid","description":"Only runs asked about this observation or written into it."},` +
		`"anchored":{"type":"boolean","description":"true keeps only your own runs that point at a spot; it needs scope mine."},` +
		`"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Runs per page. Default 20."},` +
		`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`
	schemaRenameRun = `{"type":"object","properties":{"id":{"type":"string","format":"uuid","description":"The run id, from list_runs."},"title":{"type":["string","null"],"minLength":1,"maxLength":120,"description":"The new title, 1 to 120 characters on one line; null clears it back to the start of the prompt."}},"required":["id","title"],"additionalProperties":false}`
)

var listRunStates = []string{"queued", "running", "canceling", "succeeded", "failed", "canceled"}

func init() { registerEntries((*Registry).runListEntries) }

func (r *Registry) runListEntries() []entry {
	return []entry{
		{Tool{Name: nameListRuns, Description: descListRuns, InputSchema: json.RawMessage(schemaListRuns), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.listRuns},
		{Tool{Name: nameRenameRun, Description: descRenameRun, InputSchema: json.RawMessage(schemaRenameRun), MCPOnly: true, MinRole: auth.RoleViewer}, r.renameRun},
		{Tool{Name: namePinRun, Description: descPinRun, InputSchema: json.RawMessage(schemaRunTarget), MCPOnly: true, MinRole: auth.RoleViewer}, r.pinRun},
		{Tool{Name: nameUnpinRun, Description: descUnpinRun, InputSchema: json.RawMessage(schemaRunTarget), MCPOnly: true, MinRole: auth.RoleViewer}, r.unpinRun},
		{Tool{Name: nameArchiveRun, Description: descArchiveRun, InputSchema: json.RawMessage(schemaRunTarget), MCPOnly: true, MinRole: auth.RoleViewer}, r.archiveRun},
	}
}

type listRunsInput struct {
	Scope         string   `json:"scope"`
	State         []string `json:"state"`
	Q             *string  `json:"q"`
	Pinned        *bool    `json:"pinned"`
	DashboardID   string   `json:"dashboard_id"`
	ObservationID string   `json:"observation_id"`
	Anchored      bool     `json:"anchored"`
	Limit         *int     `json:"limit"`
	Cursor        string   `json:"cursor"`
}

func (in listRunsInput) parse() (revisions.ListFilter, int, Result, bool) {
	filter := revisions.ListFilter{Scope: revisions.ScopeMine, Pinned: in.Pinned}
	switch revisions.ListScope(in.Scope) {
	case "":
	case revisions.ScopeMine, revisions.ScopeAll:
		filter.Scope = revisions.ListScope(in.Scope)
	default:
		return filter, 0, invalid("scope must be mine or all"), false
	}
	for _, state := range in.State {
		if !slices.Contains(listRunStates, state) {
			return filter, 0, invalid("state must be queued, running, canceling, succeeded, failed or canceled"), false
		}
		filter.States = append(filter.States, revisions.State(state))
	}
	if in.Q != nil {
		q, err := revisions.CheckQuery(*in.Q)
		if err != nil {
			return filter, 0, invalid("%s", err.Error()), false
		}
		filter.Query = q
	}
	if in.DashboardID != "" {
		id, res, ok := parseDashboardID(in.DashboardID)
		if !ok {
			return filter, 0, res, false
		}
		filter.DashboardID = id
	}
	if in.ObservationID != "" {
		id, err := db.ParseUUID(in.ObservationID)
		if err != nil {
			return filter, 0, invalid("observation_id must be a UUID"), false
		}
		filter.ObservationID = id
	}
	filter.Anchored = in.Anchored
	limit, res, ok := boundedInt(in.Limit, defaultListRuns, 1, maxListRuns, "limit")
	if !ok {
		return filter, 0, res, false
	}
	res, ok = checkLength("cursor", in.Cursor, maxNameBytes)
	return filter, limit, res, ok
}

type runListAuthorOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type runListItemOut struct {
	ID               string           `json:"id"`
	Title            string           `json:"title"`
	Kind             string           `json:"kind"`
	Intent           string           `json:"intent"`
	Investigation    bool             `json:"investigation"`
	State            string           `json:"state"`
	ResultKind       string           `json:"result_kind"`
	DashboardID      *string          `json:"dashboard_id"`
	DashboardTitle   *string          `json:"dashboard_title"`
	ObservationID    *string          `json:"observation_id"`
	ObservationTitle *string          `json:"observation_title"`
	ChatID           *string          `json:"chat_id"`
	Anchor           *anchor.Anchor   `json:"anchor"`
	BlockID          *string          `json:"block_id"`
	PinnedAt         *string          `json:"pinned_at"`
	CreatedAt        string           `json:"created_at"`
	RequestedBy      runListAuthorOut `json:"requested_by"`
	AllowedActions   []string         `json:"allowed_actions"`
}

type runListCountOut struct {
	Total  int64  `json:"total"`
	People *int64 `json:"people"`
}

type runListOut struct {
	Runs       []runListItemOut `json:"runs"`
	NextCursor *string          `json:"next_cursor"`
	Summary    *runListCountOut `json:"summary"`
}

func (r *Registry) listRuns(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Runs == nil {
		return Result{}, errRunsNotConfigured
	}
	var in listRunsInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	filter, limit, res, ok := in.parse()
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	for {
		raw, err := r.deps.Runs.List(ctx, principal, filter, limit, in.Cursor)
		if errors.Is(err, revisions.ErrInvalidCursor) {
			return invalid("cursor is not valid"), nil
		}
		if err != nil {
			return runFailure(ctx, err)
		}
		var page struct {
			Requests   []runListItemOut `json:"requests"`
			NextCursor *string          `json:"next_cursor"`
			Summary    *runListCountOut `json:"summary"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return Result{}, err
		}
		for i := range page.Requests {
			if page.Requests[i].AllowedActions == nil {
				page.Requests[i].AllowedActions = []string{}
			}
		}
		out := runListOut{Runs: page.Requests, NextCursor: page.NextCursor, Summary: page.Summary}
		if out.Runs == nil {
			out.Runs = []runListItemOut{}
		}
		kept, cut := fit(out.Runs, listBudget-jsonSize(runListOut{Runs: []runListItemOut{}, NextCursor: out.NextCursor, Summary: out.Summary}))
		if !cut || len(out.Runs) == 1 {
			return success("Listed "+plural(len(out.Runs), "run", "runs"), out), nil
		}
		limit = max(len(kept), 1)
	}
}

type renameRunInput struct {
	ID    string          `json:"id"`
	Title json.RawMessage `json:"title"`
}

func (r *Registry) renameRun(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Runs == nil {
		return Result{}, errRunsNotConfigured
	}
	var in renameRunInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, err := db.ParseUUID(in.ID)
	if err != nil {
		return invalid("id must be a UUID"), nil
	}
	var title *string
	clearTitle := bytes.Equal(bytes.TrimSpace(in.Title), []byte("null"))
	switch {
	case len(in.Title) == 0:
		return invalid("title is required; send null to take the title back to the start of the prompt"), nil
	case !clearTitle:
		var raw string
		if json.Unmarshal(in.Title, &raw) != nil {
			return invalid("title must be a string or null"), nil
		}
		cleaned, err := revisions.CheckTitle(raw)
		if err != nil {
			return invalid("%s", err.Error()), nil
		}
		title = &cleaned
	}
	principal, _ := auth.FromContext(ctx)
	raw, err := r.deps.Runs.Update(ctx, id, principal, title, clearTitle, nil)
	if err != nil {
		return runFailure(ctx, err)
	}
	var shared struct {
		Observation revisions.SharedTitle `json:"observation_rename"`
	}
	if err := json.Unmarshal(raw, &shared); err != nil {
		return Result{}, fmt.Errorf("decode renamed run: %w", err)
	}
	return success("Renamed run "+id.String()+observationRenameNote[shared.Observation], raw), nil
}

var observationRenameNote = map[revisions.SharedTitle]string{
	revisions.TitleRenamed: " and the observation made from it",
	revisions.TitleKept:    "; the observation made from it keeps its title because you may not edit it",
}

func (r *Registry) setRunPin(ctx context.Context, input json.RawMessage, pinned bool) (Result, error) {
	if r.deps.Runs == nil {
		return Result{}, errRunsNotConfigured
	}
	id, res, ok := r.runTarget(input)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	raw, err := r.deps.Runs.Update(ctx, id, principal, nil, false, &pinned)
	if err != nil {
		return runFailure(ctx, err)
	}
	step := "Unpinned run "
	if pinned {
		step = "Pinned run "
	}
	return success(step+id.String(), raw), nil
}

func (r *Registry) pinRun(ctx context.Context, input json.RawMessage) (Result, error) {
	return r.setRunPin(ctx, input, true)
}

func (r *Registry) unpinRun(ctx context.Context, input json.RawMessage) (Result, error) {
	return r.setRunPin(ctx, input, false)
}

type archivedRunOut struct {
	ID       string `json:"id"`
	Archived bool   `json:"archived"`
}

func (r *Registry) archiveRun(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Runs == nil {
		return Result{}, errRunsNotConfigured
	}
	id, res, ok := r.runTarget(input)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	if err := r.deps.Runs.Archive(ctx, id, principal); err != nil {
		return runFailure(ctx, err)
	}
	return success("Archived run "+id.String(), archivedRunOut{ID: id.String(), Archived: true}), nil
}
