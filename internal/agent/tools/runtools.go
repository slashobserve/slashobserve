package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
)

const (
	codeOutOfScope = "out_of_scope"

	MaxCreatedIDs = 256
)

const (
	reasonClaims    = "a run token works a run and never claims one; only a claimer calls the claim tools"
	reasonComments  = "the server answers and resolves threads after a run"
	reasonRuns      = "a run never starts, retries or cancels a run, because the new run would escape the cost and call limits of this one"
	reasonVersions  = "only the finished build becomes a version, one per request"
	reasonDraft     = "a run's cost is computed once by runsession, and draft_query makes model calls outside it"
	reasonShare     = "only the requester shares a finished answer into a thread, and inside a run the answer is not finished"
	reasonApply     = "it posts in a thread and starts a run"
	reasonProposals = "merging writes a dashboard version outside the build"
	reasonRunAnswer = "a later turn of a chat could publish an earlier private ask's answer, so only the requester saves an answer"
	reasonApp       = "it draws a chart in an MCP Apps host, which a run does not have"
	reasonContext   = "what an MCP source returns is outside text a build must not act on, so only an investigation or a chat question reads it"
	reasonHealth    = "it checks every dashboard of the workspace, far outside what one run is about"
)

var leftOutOfRuns = map[string]string{
	nameClaimRun:                reasonClaims,
	nameReportRun:               reasonClaims,
	nameFinishRun:               reasonClaims,
	nameGetRunBrief:             reasonClaims,
	nameReleaseRun:              reasonClaims,
	nameCreateComment:           reasonComments,
	nameReplyToComment:          reasonComments,
	nameEditComment:             reasonComments,
	nameResolveComment:          reasonComments,
	nameReopenComment:           reasonComments,
	nameShareAnswer:             reasonShare,
	nameApplySuggestedChange:    reasonApply,
	nameStartRun:                reasonRuns,
	nameRetryRun:                reasonRuns,
	nameCancelRun:               reasonRuns,
	nameDraftQuery:              reasonDraft,
	nameRestoreDashboardVersion: reasonVersions,
	nameMergeProposal:           reasonProposals,
	nameMakeDashboard:           reasonVersions,
	nameDuplicateDashboard:      reasonVersions,
	nameSaveRunAnswer:           reasonRunAnswer,
	nameRunDashboardHealth:      reasonHealth,
	nameShowPanel:               reasonApp,
}

var contextReads = map[string]string{
	nameListMCPSources: reasonContext,
	nameCallMCPTool:    reasonContext,
	nameFindMCPTools:   reasonContext,
}

var creatingWrites = map[string]bool{
	nameCreateObservation: true,
	nameAddBlock:          true,
	nameCreateFolder:      true,
	nameSaveSearch:        true,
	nameCreateAnnotation:  true,
}

type TargetKind int

const (
	TargetNew TargetKind = iota
	TargetDashboard
	TargetObservation
	TargetOwned
	TargetRun
	TargetPerson
	TargetEveryDashboard
)

type Target struct {
	Kind  TargetKind
	ID    string
	Field string
}

type WriteScope struct {
	Created   []string
	Committed string
}

type writeScopeKey struct{}

func WithWriteScope(ctx context.Context, scope WriteScope) context.Context {
	return context.WithValue(ctx, writeScopeKey{}, scope)
}

func WriteScopeFrom(ctx context.Context) WriteScope {
	scope, _ := ctx.Value(writeScopeKey{}).(WriteScope)
	return scope
}

func normalizeID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func sameID(a, b string) bool {
	return a != "" && normalizeID(a) == normalizeID(b)
}

func (run Run) Chat() bool {
	return run.ChatID != "" && !run.ThreadAsk && !run.Investigation
}

func (run Run) privateAsk() bool {
	return run.Mode == ModeInvestigate && run.ChatID != "" && !run.ThreadAsk && !run.Investigation && (run.DashboardID != "" || len(run.Observations) > 0)
}

func (run Run) subjectIs(id string) bool {
	return sameID(run.DashboardID, id) || slices.ContainsFunc(run.Observations, func(o string) bool { return sameID(o, id) })
}

func (scope WriteScope) created(id string) bool {
	return slices.ContainsFunc(scope.Created, func(c string) bool { return sameID(c, id) })
}

func fieldTarget(kind TargetKind, field string) func(json.RawMessage) []Target {
	return func(input json.RawMessage) []Target {
		return []Target{{Kind: kind, ID: stringField(input, field), Field: field}}
	}
}

func stringField(input json.RawMessage, field string) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(input, &fields) != nil {
		return ""
	}
	var id string
	if json.Unmarshal(fields[field], &id) != nil {
		return ""
	}
	return normalizeID(id)
}

func newObject(json.RawMessage) []Target { return []Target{{Kind: TargetNew}} }

func personState(json.RawMessage) []Target { return []Target{{Kind: TargetPerson}} }

func annotationTarget(input json.RawMessage) []Target {
	if id := stringField(input, "dashboard_id"); id != "" {
		return []Target{{Kind: TargetDashboard, ID: id}}
	}
	return []Target{{Kind: TargetEveryDashboard}}
}

func pinObservationTarget(input json.RawMessage) []Target {
	return append(fieldTarget(TargetObservation, "observation_id")(input), fieldTarget(TargetDashboard, "dashboard_id")(input)...)
}

var writeTargets = map[string]func(json.RawMessage) []Target{
	nameCreateObservation:    newObject,
	nameAddBlock:             fieldTarget(TargetObservation, "observation_id"),
	nameUpdateBlock:          fieldTarget(TargetOwned, "block_id"),
	nameArrangeBlocks:        fieldTarget(TargetObservation, "observation_id"),
	nameKeepBlock:            fieldTarget(TargetOwned, "block_id"),
	nameDiscardBlock:         fieldTarget(TargetOwned, "block_id"),
	nameResolveObservation:   fieldTarget(TargetObservation, "observation_id"),
	nameReopenObservation:    fieldTarget(TargetObservation, "observation_id"),
	nameUpdateObservation:    fieldTarget(TargetObservation, "observation_id"),
	nameMoveObservation:      fieldTarget(TargetObservation, "observation_id"),
	namePublishRCA:           fieldTarget(TargetObservation, "observation_id"),
	namePinObservation:       pinObservationTarget,
	nameMoveDashboard:        fieldTarget(TargetDashboard, "dashboard_id"),
	nameCreateAnnotation:     annotationTarget,
	nameUpdateAnnotation:     fieldTarget(TargetOwned, "id"),
	namePinDashboard:         personState,
	nameUnpinDashboard:       personState,
	nameSetHomeDashboard:     personState,
	nameClearHomeDashboard:   personState,
	nameCreateFolder:         newObject,
	nameRenameFolder:         fieldTarget(TargetOwned, "folder_id"),
	nameSaveSearch:           newObject,
	nameUpdateSavedSearch:    fieldTarget(TargetOwned, "id"),
	nameDiscardProposal:      fieldTarget(TargetOwned, "proposal_id"),
	nameSetInboxItemDone:     personState,
	nameRequestCommentAccess: personState,
	nameRenameRun:            fieldTarget(TargetRun, "id"),
	namePinRun:               fieldTarget(TargetRun, "id"),
	nameUnpinRun:             fieldTarget(TargetRun, "id"),
	nameArchiveRun:           fieldTarget(TargetRun, "id"),
}

var personStateWrites = map[string]bool{
	namePinDashboard:         true,
	nameUnpinDashboard:       true,
	nameSetHomeDashboard:     true,
	nameClearHomeDashboard:   true,
	nameSetInboxItemDone:     true,
	nameRequestCommentAccess: true,
	nameRenameRun:            true,
	namePinRun:               true,
	nameUnpinRun:             true,
	nameArchiveRun:           true,
}

var webActions = map[string]string{
	nameCreateObservation:    "New observation",
	nameAddBlock:             "Add a block on the observation page",
	nameUpdateBlock:          "editing the block on the observation page",
	nameArrangeBlocks:        "dragging blocks on the observation page",
	nameKeepBlock:            "Keep on the block",
	nameDiscardBlock:         "Discard on the block",
	nameResolveObservation:   "Resolve on the observation page",
	nameReopenObservation:    "Reopen on the observation page",
	nameUpdateObservation:    "editing the observation's title or range",
	nameMoveObservation:      "Move on the observation page",
	namePublishRCA:           "Publish RCA on the observation page",
	namePinObservation:       "Pin on the observation page",
	nameMoveDashboard:        "Move on the dashboard's menu",
	nameCreateAnnotation:     "adding an annotation on the dashboard",
	nameUpdateAnnotation:     "editing the annotation on the dashboard",
	namePinDashboard:         "the pin on the dashboard",
	nameUnpinDashboard:       "the pin on the dashboard",
	nameSetHomeDashboard:     "Set as home on the dashboard's menu",
	nameClearHomeDashboard:   "Clear home in the settings",
	nameCreateFolder:         "New folder",
	nameRenameFolder:         "Rename on the folder's menu",
	nameSaveSearch:           "Save search on the search page",
	nameUpdateSavedSearch:    "editing the saved search",
	nameDiscardProposal:      "Discard on the proposal",
	nameSetInboxItemDone:     "Done in the inbox",
	nameRequestCommentAccess: "Request access on the page",
	nameRenameRun:            "Rename on the chat",
	namePinRun:               "Pin on the chat",
	nameUnpinRun:             "Unpin on the chat",
	nameArchiveRun:           "Archive on the chat",
}

func scopeRefusal(tool, reason string) string {
	action := webActions[tool]
	if action == "" {
		return fmt.Sprintf("%s cannot run here: %s", tool, reason)
	}
	return fmt.Sprintf("%s cannot run here: %s. A person does this with %s", tool, reason, action)
}

func writeScopeViolation(tool string, run Run, scope WriteScope, targets []Target, inChat func(runID string) bool) string {
	if run.Mode == ModeSuggest {
		return scopeRefusal(tool, "a suggest run proposes one panel and changes nothing else")
	}
	person := personStateWrites[tool]
	switch {
	case creatingWrites[tool] && len(scope.Created) >= MaxCreatedIDs:
		return scopeRefusal(tool, fmt.Sprintf("this run already created %d objects, the most it may track", MaxCreatedIDs))
	case person && !run.Chat():
		return scopeRefusal(tool, "it changes the requester's own pins, inbox or run list, which only a chat run may do")
	case !person && run.privateAsk():
		return scopeRefusal(tool, "a private ask writes nothing shared; ask in a thread on the page, or do it on the page")
	}
	for _, target := range targets {
		if reason := targetViolation(run, scope, target, inChat); reason != "" {
			return scopeRefusal(tool, reason)
		}
	}
	return ""
}

func targetViolation(run Run, scope WriteScope, target Target, inChat func(runID string) bool) string {
	if target.ID == "" && target.Field != "" {
		return fmt.Sprintf("%s %s is empty, and inside a run a write names what it changes", missingTarget+":", target.Field)
	}
	switch target.Kind {
	case TargetEveryDashboard:
		return "without dashboard_id it would land on every dashboard"
	case TargetDashboard, TargetObservation:
		return objectViolation(run, scope, target.ID)
	case TargetOwned:
		return ownedViolation(scope, target.ID)
	case TargetRun:
		return runViolation(run, target.ID, inChat)
	}
	return ""
}

const missingTarget = "it names no target"

func objectViolation(run Run, scope WriteScope, id string) string {
	if id == "" {
		return missingTarget
	}
	if run.subjectIs(id) || scope.created(id) || sameID(scope.Committed, id) {
		return ""
	}
	return "a run may change only what it is about, what it created and the dashboard it built"
}

func ownedViolation(scope WriteScope, id string) string {
	if id == "" {
		return missingTarget
	}
	if scope.created(id) {
		return ""
	}
	return "a run may change only what it created"
}

func runViolation(run Run, id string, inChat func(runID string) bool) string {
	if id == "" {
		return missingTarget
	}
	if sameID(run.RequestID, id) || (run.ChatID != "" && inChat != nil && inChat(id)) {
		return ""
	}
	return "a run may change only runs of its own chat"
}

func (r *Registry) admitted(ctx context.Context, tool Tool, run Run, inRun bool, input json.RawMessage) (Result, bool) {
	if inRun && tool.OfferedInMode(run) && !tool.InTask(run) {
		return r.notInTask(tool, run), false
	}
	if res, ok := offered(tool, run, inRun); !ok {
		return res, false
	}
	return r.withinWriteScope(ctx, tool, run, inRun, input)
}

func (r *Registry) withinWriteScope(ctx context.Context, tool Tool, run Run, inRun bool, input json.RawMessage) (Result, bool) {
	name := tool.Name
	if !inRun || tool.MCPOnly || Phase(name) != PhaseWrite {
		return Result{}, true
	}
	targetsOf, declared := writeTargets[name]
	if !declared {
		res := toolError(codeOutOfScope, name+" has no declared write scope")
		res.Step = "Refused " + name + ": " + codeOutOfScope
		return res, false
	}
	message := writeScopeViolation(name, run, WriteScopeFrom(ctx), targetsOf(input), r.runInChat(ctx, run))
	if message == "" {
		return Result{}, true
	}
	res := toolError(codeOutOfScope, message)
	res.Step = "Refused " + name + ": " + codeOutOfScope
	return res, false
}

func (r *Registry) runInChat(ctx context.Context, run Run) func(runID string) bool {
	return func(runID string) bool {
		id, err := db.ParseUUID(runID)
		if err != nil || r.deps.Runs == nil {
			return false
		}
		principal, _ := auth.FromContext(ctx)
		raw, err := r.deps.Runs.Run(ctx, id, principal, 1)
		if err != nil {
			return false
		}
		var found struct {
			ChatID *string `json:"chat_id"`
		}
		return json.Unmarshal(raw, &found) == nil && found.ChatID != nil && sameID(*found.ChatID, run.ChatID)
	}
}
