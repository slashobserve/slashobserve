package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/proposals"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	nameListProposals   = "list_proposals"
	nameGetProposal     = "get_proposal"
	nameMergeProposal   = "merge_proposal"
	nameDiscardProposal = "discard_proposal"

	codeProposeMode     = "propose_mode"
	codeProposalChanged = "proposal_changed"

	defaultListProposals = 20
	maxMergeChoices      = 400

	proposeModeMessage = "This connection is in propose mode, so it cannot merge; ask a person who can edit to merge it."

	descListProposals   = "List the proposals on a dashboard, newest first: open ones by default, or state merged, discarded or all. A proposal is a change someone who cannot edit, or whose connection is in propose mode, saved for an Editor to merge. Pass next_cursor as cursor for the next page."
	descGetProposal     = "Read a proposal: its summary, the proposed spec, its changes against the version it was based on, the conflicts a merge would hit on the latest version, and the allowed_actions you have now. A spec over 128 KiB returns too_large."
	descMergeProposal   = "Merge an open proposal into its dashboard as a new version; only someone who can edit may, and not a connection in propose mode. Send the revision you read with get_proposal, and a choice per conflict, ours for the dashboard's current unit or theirs for the proposal's. The merge runs the queries of the panels it changes before it saves."
	descDiscardProposal = "Close an open proposal without saving its change. Its author may, and so may an Editor whose connection is not in propose mode."

	propProposalID = `"proposal_id":{"type":"string","format":"uuid","description":"The proposal, from list_proposals or a save that answered proposed."}`

	schemaListProposals   = `{"type":"object","properties":{` + propDashID + `,"state":{"type":"string","enum":["open","merged","discarded","all"],"description":"Which proposals to list. Default open."},"cursor":{"type":"string","description":"next_cursor from the previous page."}},"required":["dashboard_id"],"additionalProperties":false}`
	schemaGetProposal     = `{"type":"object","properties":{` + propProposalID + `},"required":["proposal_id"],"additionalProperties":false}`
	schemaMergeProposal   = `{"type":"object","properties":{` + propProposalID + `,"revision":{"type":"integer","minimum":1,"description":"The proposal's revision from get_proposal; a newer one answers proposal_changed."},"choices":{"type":"array","maxItems":400,"items":{"type":"object","additionalProperties":false,"required":["kind","id","take"],"properties":{"kind":{"type":"string","enum":["panel","variable","setting"]},"id":{"type":"string","description":"The panel id, variable name or setting name from the conflict."},"take":{"type":"string","enum":["ours","theirs"],"description":"ours keeps the dashboard's current unit, theirs takes the proposal's."}}},"description":"One choice per conflict get_proposal listed."}},"required":["proposal_id","revision"],"additionalProperties":false}`
	schemaDiscardProposal = `{"type":"object","properties":{` + propProposalID + `},"required":["proposal_id"],"additionalProperties":false}`
)

var errProposalsNotConfigured = errors.New("proposals are not configured")

func init() { registerEntries((*Registry).proposalEntries) }

func (r *Registry) proposalEntries() []entry {
	return []entry{
		{Tool{Name: nameListProposals, Description: descListProposals, InputSchema: json.RawMessage(schemaListProposals), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.listProposals},
		{Tool{Name: nameGetProposal, Description: descGetProposal, InputSchema: json.RawMessage(schemaGetProposal), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.getProposal},
		{Tool{Name: nameMergeProposal, Description: descMergeProposal, InputSchema: json.RawMessage(schemaMergeProposal), MCPOnly: true, MinRole: auth.RoleViewer}, r.mergeProposal},
		{Tool{Name: nameDiscardProposal, Description: descDiscardProposal, InputSchema: json.RawMessage(schemaDiscardProposal), MCPOnly: true, MinRole: auth.RoleViewer}, r.discardProposal},
	}
}

type proposalListOut struct {
	Proposals  []proposals.SummaryJSON `json:"proposals"`
	NextCursor *string                 `json:"next_cursor"`
}

type proposalChangedOut struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Revision int    `json:"revision"`
}

type invalidTransitionOut struct {
	Code           string   `json:"code"`
	Message        string   `json:"message"`
	State          string   `json:"state"`
	AllowedActions []string `json:"allowed_actions"`
}

type mergeChoiceInput struct {
	Kind spec.UnitKind `json:"kind"`
	ID   string        `json:"id"`
	Take spec.Side     `json:"take"`
}

func parseProposalID(raw string) (pgtype.UUID, Result, bool) {
	id, err := db.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, invalid("proposal_id must be a UUID from list_proposals or a save that answered proposed"), false
	}
	return id, Result{}, true
}

func (r *Registry) listProposals(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		DashboardID string `json:"dashboard_id"`
		State       string `json:"state"`
		Cursor      string `json:"cursor"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseDashboardID(in.DashboardID)
	if !ok {
		return res, nil
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return res, nil
	}
	if r.deps.Proposals == nil {
		return Result{}, errProposalsNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	limit := defaultListProposals
	for {
		page, err := r.deps.Proposals.List(ctx, principal, proposals.ListQuery{DashboardID: id, State: in.State, Limit: limit, Cursor: in.Cursor})
		if err != nil {
			return r.proposalFailure(err)
		}
		items := make([]proposals.SummaryJSON, len(page.Items))
		for i, item := range page.Items {
			items[i] = proposals.NewSummaryJSON(item)
		}
		kept, cut := fit(items, listBudget)
		if cut && limit > 1 {
			limit = max(len(kept), 1)
			continue
		}
		out := proposalListOut{Proposals: items}
		if page.NextCursor != "" {
			out.NextCursor = &page.NextCursor
		}
		return success("Listed "+plural(len(items), "proposal", "proposals"), out), nil
	}
}

func (r *Registry) getProposal(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		ProposalID string `json:"proposal_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseProposalID(in.ProposalID)
	if !ok {
		return res, nil
	}
	if r.deps.Proposals == nil {
		return Result{}, errProposalsNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	proposal, err := r.deps.Proposals.Get(ctx, principal, id)
	if err != nil {
		return r.proposalFailure(err)
	}
	out := proposals.NewProposalJSON(proposal)
	if size := jsonSize(out); size > MaxModelSpecBytes {
		return toolError(codeTooLarge, fmt.Sprintf("the proposal is %d bytes, over the %d byte tool result limit", size, MaxModelSpecBytes)), nil
	}
	return success("Read proposal revision "+strconv.Itoa(out.Revision), out), nil
}

func (r *Registry) mergeProposal(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		ProposalID string             `json:"proposal_id"`
		Revision   *int               `json:"revision"`
		Choices    []mergeChoiceInput `json:"choices"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseProposalID(in.ProposalID)
	if !ok {
		return res, nil
	}
	if in.Revision == nil {
		return invalid("revision is required: the revision from get_proposal"), nil
	}
	revision, res, ok := boundedInt(in.Revision, 0, 1, math.MaxInt32, "revision")
	if !ok {
		return res, nil
	}
	if len(in.Choices) > maxMergeChoices {
		return invalid("choices takes at most %d items", maxMergeChoices), nil
	}
	choices := make([]spec.Choice, len(in.Choices))
	for i, c := range in.Choices {
		choices[i] = spec.Choice(c)
	}
	if r.deps.Proposals == nil {
		return Result{}, errProposalsNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	merged, err := r.deps.Proposals.Merge(ctx, principal, id, revision, choices)
	if err != nil {
		return r.proposalFailure(err)
	}
	step := "Merged proposal; the dashboard already had its change"
	if merged.MergedVersion != nil {
		step = "Merged proposal as version " + strconv.Itoa(*merged.MergedVersion)
	}
	return success(step, proposals.NewSummaryJSON(merged.Item)), nil
}

func (r *Registry) discardProposal(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		ProposalID string `json:"proposal_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseProposalID(in.ProposalID)
	if !ok {
		return res, nil
	}
	if r.deps.Proposals == nil {
		return Result{}, errProposalsNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	discarded, err := r.deps.Proposals.Discard(ctx, principal, id)
	if err != nil {
		return r.proposalFailure(err)
	}
	return success("Discarded proposal", proposals.NewSummaryJSON(discarded.Item)), nil
}

func (r *Registry) proposalFailure(err error) (Result, error) {
	var refusal *lifecycle.Refusal
	var changed *proposals.ChangedError
	var invalidInput *dashboards.InvalidInputError
	switch {
	case errors.As(err, &refusal):
		allowed := refusal.Allowed
		if allowed == nil {
			allowed = []string{}
		}
		return failure("", invalidTransitionOut{
			Code:           codeInvalidTransition,
			Message:        fmt.Sprintf("%s is not allowed while the %s is %s%s", refusal.Action, refusal.Machine, refusal.State, allowedNow(refusal.Allowed)),
			State:          refusal.State,
			AllowedActions: allowed,
		}), nil
	case errors.Is(err, proposals.ErrNotFound):
		return toolError(codeNotFound, "proposal not found"), nil
	case errors.Is(err, dashboards.ErrNotFound), errors.Is(err, folders.ErrNoAccess):
		return toolError(codeNotFound, "dashboard not found"), nil
	case errors.Is(err, proposals.ErrProposeMode):
		return toolError(codeProposeMode, proposeModeMessage), nil
	case errors.Is(err, proposals.ErrForbidden):
		return toolError(codeForbiddenRole, forbidden.RoleMessage), nil
	case errors.As(err, &changed):
		return failure("", proposalChangedOut{
			Code:     codeProposalChanged,
			Message:  fmt.Sprintf("the proposal has a newer revision; read it with %s and merge that revision", nameGetProposal),
			Revision: changed.Revision,
		}), nil
	case errors.Is(err, dashboards.ErrInvalidCursor):
		return invalid("cursor is not valid"), nil
	case errors.As(err, &invalidInput):
		return invalid("%s", invalidInput.Message), nil
	}
	return saveFailure(err)
}
