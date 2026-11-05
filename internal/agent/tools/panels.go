package tools

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxGetPanelIDs  = 20
	maxUpsertPanels = 50
	maxDeletePanels = 50
	maxLayoutMoves  = 200
	panelsBudget    = 60 << 10
	panelsHeadroom  = 2 << 10

	repairLead   = "Fix only the panels in failing_panels and save again."
	repairLayout = "In this run the server sets positions: resend the panel with only the w and h it needs."
	repairQuery  = "The next call may run query_range or query_instant on the failing query to see why."
	repairSchema = "If an issue names a field you do not know, call get_spec_guide for the JSON Schema."
)

var queryIssueCodes = []spec.IssueCode{
	queryproxy.CodeQueryInvalid, queryproxy.CodeQueryEmpty, queryproxy.CodeQueryTimeout, queryproxy.CodeQueryForbidden,
	queryproxy.CodeLimitExceeded, queryproxy.CodeExpressionFailed, queryproxy.CodeVariableFailed, queryproxy.CodeVariableEmpty,
	queryproxy.CodeVariableUnresolved, queryproxy.CodeDatasourceDown, queryproxy.CodeUnknownDatasource,
}

func (r *Registry) panelEntries() []entry {
	return []entry{
		{Tool{Name: nameGetPanels, Description: descGetPanels, InputSchema: json.RawMessage(schemaGetPanels), ReadOnly: true, MinRole: auth.RoleViewer}, r.getPanels},
		{Tool{Name: nameUpdatePanels, Description: descUpdatePanels, InputSchema: json.RawMessage(schemaUpdatePanels), MinRole: auth.RoleEditor}, r.updatePanels},
	}
}

type panelsOut struct {
	DashboardID string          `json:"dashboard_id"`
	Version     int             `json:"version"`
	Panels      []*spec.Panel   `json:"panels"`
	Variables   []spec.Variable `json:"variables"`
	Missing     []string        `json:"missing"`
	TooLarge    []string        `json:"too_large,omitempty"`
	Truncated   bool            `json:"truncated,omitempty"`
	Build       bool            `json:"build,omitempty"`
}

type getPanelsInput struct {
	DashboardID string   `json:"dashboard_id"`
	Version     *int     `json:"version"`
	IDs         []string `json:"ids"`
}

func (r *Registry) getPanels(ctx context.Context, input json.RawMessage) (Result, error) {
	var in getPanelsInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if len(in.IDs) == 0 || len(in.IDs) > maxGetPanelIDs {
		return invalid("ids must hold 1 to %d panel ids", maxGetPanelIDs), nil
	}
	version, res, ok := boundedInt(in.Version, 0, 1, math.MaxInt32, "version")
	if !ok {
		return res, nil
	}
	run, scoped := RunFrom(ctx)
	if res, handled, err := r.readBuildPanels(ctx, run, scoped, in); err != nil || handled {
		return res, err
	}
	id, res, ok := panelsDashboardID(in.DashboardID, run, scoped)
	if !ok {
		return res, nil
	}
	if res, ok, err := r.viewableDashboard(ctx, id, "dashboard or version not found"); err != nil || !ok {
		return res, err
	}
	number, doc, res, ok, err := r.panelsSource(ctx, run, scoped, id, in.Version, version)
	if err != nil || !ok {
		return res, err
	}
	out, err := readPanels(id.String(), number, doc, in.IDs)
	if err != nil {
		return Result{}, err
	}
	return success("Read "+plural(len(out.Panels), "panel", "panels")+" of version "+strconv.Itoa(number), out), nil
}

func panelsDashboardID(given string, run Run, scoped bool) (pgtype.UUID, Result, bool) {
	if given == "" && scoped {
		given = run.DashboardID
	}
	if given == "" {
		return pgtype.UUID{}, invalid("dashboard_id is required"), false
	}
	return parseDashboardID(given)
}

func readPanels(dashboardID string, version int, doc *spec.Dashboard, ids []string) (panelsOut, error) {
	out := panelsOut{DashboardID: dashboardID, Version: version, Panels: []*spec.Panel{}, Missing: []string{}}
	out.Variables, out.Truncated = fit(nonNilVariables(doc.Variables), listBudget)
	var found []*spec.Panel
	var foundIDs []string
	asked := map[string]bool{}
	for _, want := range ids {
		if asked[want] {
			continue
		}
		asked[want] = true
		entry, ok := doc.FindPanel(want)
		if !ok {
			out.Missing = append(out.Missing, want)
			continue
		}
		panel := *entry.Panel
		panel.Panels = nil
		found = append(found, &panel)
		foundIDs = append(foundIDs, want)
	}
	variables, err := json.Marshal(out.Variables)
	if err != nil {
		return panelsOut{}, err
	}
	var fitting []*spec.Panel
	var fittingIDs []string
	for i, panel := range found {
		raw, err := json.Marshal(panel)
		if err != nil {
			return panelsOut{}, err
		}
		if len(raw)+len(variables)+panelsHeadroom > MaxModelSpecBytes {
			out.TooLarge = append(out.TooLarge, foundIDs[i])
			continue
		}
		fitting = append(fitting, panel)
		fittingIDs = append(fittingIDs, foundIDs[i])
	}
	if len(fitting) == 0 {
		return out, nil
	}
	first, err := json.Marshal(fitting[0])
	if err != nil {
		return panelsOut{}, err
	}
	others, cut := fit(fitting[1:], max(panelsBudget-len(variables)-len(first)-1, 0))
	out.Panels = append(out.Panels, fitting[0])
	out.Panels = append(out.Panels, others...)
	if cut {
		out.Truncated = true
		out.Missing = append(out.Missing, fittingIDs[1+len(others):]...)
	}
	return out, nil
}

func nonNilVariables(v []spec.Variable) []spec.Variable {
	if v == nil {
		return []spec.Variable{}
	}
	return v
}

func (r *Registry) panelsSource(ctx context.Context, run Run, scoped bool, id pgtype.UUID, requested *int, version int) (int, *spec.Dashboard, Result, bool, error) {
	if runID, _ := db.ParseUUID(run.DashboardID); scoped && run.Mode == ModeSuggest && requested == nil && run.Base != nil && runID == id {
		return run.ParentVersion, run.Base, Result{}, true, nil
	}
	var (
		ver dashboards.Version
		err error
	)
	if requested == nil {
		_, ver, err = r.deps.Dashboards.Get(ctx, id)
	} else {
		_, ver, err = r.deps.Dashboards.GetVersion(ctx, id, version)
	}
	if errors.Is(err, dashboards.ErrNotFound) {
		return 0, nil, toolError(codeNotFound, "dashboard or version not found"), false, nil
	}
	if err != nil {
		return 0, nil, Result{}, false, err
	}
	return ver.Number, ver.Spec, Result{}, true, nil
}

type updatePanelsInput struct {
	DashboardID   string `json:"dashboard_id"`
	ParentVersion *int   `json:"parent_version"`
	Summary       string `json:"summary"`
	buildFields
	Row *rowRef `json:"row"`
	spec.PanelPatch
}

type patchRejectedOut struct {
	invalidSpecOut
	FailingPanels []string `json:"failing_panels"`
	Message       string   `json:"message,omitempty"`
}

func (r *Registry) updatePanels(ctx context.Context, input json.RawMessage) (Result, error) {
	principal, run, scoped, err := r.saveRun(ctx)
	if err != nil {
		return Result{}, err
	}
	var in updatePanelsInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if strings.TrimSpace(in.Summary) == "" {
		return invalid("summary is required"), nil
	}
	if res, ok := patchSizeWithinLimits(in.PanelPatch); !ok {
		return res, nil
	}
	if res, ok := agentOnlyFields(in.Finish, in.PlannedPanels, in.Row, inBuildRun(run, scoped)); !ok {
		return res, nil
	}
	if scoped && run.Mode == ModeSuggest {
		return r.suggestPatch(ctx, principal, run, in.PanelPatch)
	}
	target, res, ok, err := updateTarget(saveInput{DashboardID: in.DashboardID, ParentVersion: in.ParentVersion}, run, scoped)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return res, nil
	}
	if inBuildRun(run, scoped) {
		return r.buildPatch(ctx, principal, run, target, in)
	}
	requestID, err := requestIDOf(run, scoped)
	if err != nil {
		return Result{}, err
	}
	if !scoped {
		if res, ok, err := r.viewableDashboard(ctx, target.id, "dashboard not found"); err != nil || !ok {
			return res, err
		}
	}
	return r.savePatch(ctx, principal, target, requestID, in)
}

func (r *Registry) savePatch(ctx context.Context, principal auth.Principal, target saveTarget, requestID pgtype.UUID, in updatePanelsInput) (Result, error) {
	_, parent, err := r.deps.Dashboards.GetVersion(ctx, target.id, target.parent)
	if err != nil {
		return saveFailure(err)
	}
	if parent.Spec == nil {
		return Result{}, errors.New("parent version has no spec")
	}
	merged, err := spec.ApplyPanelPatch(parent.Spec, in.PanelPatch)
	var patchErr *spec.PatchError
	if errors.As(err, &patchErr) {
		return patchRejected(patchErr.Issues, patchErr.PanelIDs), nil
	}
	if err != nil {
		return Result{}, err
	}
	if issues := importOnlyIssues(merged, parent.Spec); len(issues) > 0 {
		return patchRejected(issues, panelIDsOf(merged)), nil
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return Result{}, err
	}
	dash, ver, replayed, err := r.deps.Dashboards.AddChangedVersion(ctx, principal, target.id, target.parent, raw, in.Summary, requestID)
	var invalidSpec *dashboards.InvalidSpecError
	if errors.As(err, &invalidSpec) {
		return patchRejected(invalidSpec.Issues, panelIDsOf(merged)), nil
	}
	if err != nil {
		return saveFailure(err)
	}
	return success("Saved version "+strconv.Itoa(ver.Number), withAdvice(ctx, r.savedOut(dash, ver, &replayed), ver.Spec)), nil
}

func patchSizeWithinLimits(p spec.PanelPatch) (Result, bool) {
	for _, limit := range []struct {
		field    string
		n, limit int
	}{
		{"upsert", len(p.Upsert), maxUpsertPanels},
		{"delete", len(p.Delete), maxDeletePanels},
		{"layout", len(p.Layout), maxLayoutMoves},
	} {
		if limit.n > limit.limit {
			return invalid("%s holds %d items; send at most %d per call", limit.field, limit.n, limit.limit), false
		}
	}
	return Result{}, true
}

func (r *Registry) suggestPatch(ctx context.Context, principal auth.Principal, run Run, p spec.PanelPatch) (Result, error) {
	if len(p.Upsert) != 1 || len(p.Delete) > 0 || len(p.Layout) > 0 || len(p.Variables) > 0 || p.Dashboard != nil {
		return invalid("this run proposes a change to panel %s; send exactly one upsert with that panel and nothing else", run.PanelID), nil
	}
	var head struct {
		ID   string         `json:"id"`
		Type spec.PanelType `json:"type"`
	}
	if err := json.Unmarshal(p.Upsert[0], &head); err != nil || head.ID != run.PanelID {
		return invalid("this run proposes a change to panel %s; the upserted panel must have that id", run.PanelID), nil
	}
	if res, ok := panelStaysPanel(run, head.Type); !ok {
		return res, nil
	}
	merged, err := spec.ApplyPanelPatch(run.Base, spec.PanelPatch{Upsert: p.Upsert})
	var patchErr *spec.PatchError
	if errors.As(err, &patchErr) {
		return suggestionRejected(patchErr.Issues), nil
	}
	if err != nil {
		return Result{}, err
	}
	entry, ok := merged.FindPanel(run.PanelID)
	if !ok {
		return invalid("this run proposes a change to panel %s; the upserted panel must have that id", run.PanelID), nil
	}
	raw, err := json.Marshal(entry.Panel)
	if err != nil {
		return Result{}, err
	}
	return r.proposePanel(ctx, principal, run, raw)
}

func patchRejected(issues []spec.Issue, panelIDs map[string]string) Result {
	return failure(saveRejectedStep(issues), rejectionOf(issues, panelIDs))
}

func buildRejected(issues []spec.Issue, panelIDs map[string]string) Result {
	out := rejectionOf(issues, panelIDs)
	out.Message = repairMessage(issues)
	return failure(saveRejectedStep(issues), out)
}

func repairMessage(issues []spec.Issue) string {
	parts := []string{repairLead}
	for _, issue := range issues {
		if hint := repairHint(issue.Code); !slices.Contains(parts, hint) {
			parts = append(parts, hint)
		}
	}
	return strings.Join(parts, " ")
}

func repairHint(code spec.IssueCode) string {
	switch {
	case code == spec.CodeGridOverlap || code == spec.CodeGridOverflow:
		return repairLayout
	case slices.Contains(queryIssueCodes, code):
		return repairQuery
	}
	return repairSchema
}

func rejectionOf(issues []spec.Issue, panelIDs map[string]string) patchRejectedOut {
	kept, truncated := fit(slices.Clone(issues), listBudget)
	return patchRejectedOut{
		invalidSpecOut: invalidSpecOut{Code: codeInvalidSpec, Issues: kept, Truncated: truncated},
		FailingPanels:  failingPanels(issues, panelIDs),
	}
}

func failingPanels(issues []spec.Issue, panelIDs map[string]string) []string {
	failing := []string{}
	for _, issue := range issues {
		if id, ok := owningPanelID(issue.Path, panelIDs); ok && !slices.Contains(failing, id) {
			failing = append(failing, id)
		}
	}
	return failing
}

func owningPanelID(path string, panelIDs map[string]string) (string, bool) {
	best, bestLen := "", -1
	for pointer, id := range panelIDs {
		if (path == pointer || strings.HasPrefix(path, pointer+"/")) && len(pointer) > bestLen {
			best, bestLen = id, len(pointer)
		}
	}
	return best, bestLen >= 0
}

func panelIDsOf(d *spec.Dashboard) map[string]string {
	entries := d.AllPanels()
	ids := make(map[string]string, len(entries))
	for _, e := range entries {
		ids[e.Pointer] = e.Panel.ID
	}
	return ids
}
