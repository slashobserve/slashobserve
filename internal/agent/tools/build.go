package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxChangedPanels = 50
	maxListedIDs     = 5

	defaultPanelW = 12
	defaultPanelH = 8
	maxPanelW     = spec.GridColumns
	maxPanelH     = 100

	noBuildYet            = "this run has no build yet; call create_dashboard first with the title, time, variables and the first panel"
	agentOnlyBuildFields  = "finish, planned_panels and row are only for the built-in agent"
	importInBuildRun      = "this run is building a dashboard; continue with update_panels"
	hasBuildAlready       = "this run already has a build; add panels with update_panels"
	rowNeedsPanel         = "name the row in row with its first panel"
	rowNeedsNewPanel      = "row places the new panel in upsert; move an existing panel with layout"
	rowNeedsFirstPanel    = "row places the first panel of a new dashboard; send exactly one panel in spec"
	serverPlacesPanel     = "the server places a new panel; move it in a later save"
	allPanelsBuiltMessage = "Every planned panel is built. Set finish: true on a final save, or answer with one short sentence and no tool call."
	loweredPlanFormat     = "You lowered planned_panels from %d to %d. Keep it at the number of panels the request needs; if the request needs more than you have built, raise it and add the next panel, else set finish: true."
)

type buildFields struct {
	Finish        bool `json:"finish"`
	PlannedPanels *int `json:"planned_panels"`
}

type buildOut struct {
	Build         buildStateOut `json:"build"`
	ChangedPanels []panelOut    `json:"changed_panels"`
	Message       string        `json:"message"`
}

type buildStateOut struct {
	Saves         int `json:"saves"`
	Panels        int `json:"panels"`
	PlannedPanels int `json:"planned_panels,omitempty"`
}

type addedPanel struct {
	Title string
	Row   string
}

type panelChanges struct {
	shown  []panelOut
	count  int
	added  *addedPanel
	edited int
	edits  Edits
}

type buildSave struct {
	principal     auth.Principal
	run           Run
	requestID     pgtype.UUID
	dashboardID   pgtype.UUID
	raw           []byte
	summary       string
	planned       *int
	plannedBefore int
	finish        bool
	changes       panelChanges
	panelIDs      map[string]string
}

func inBuildRun(run Run, scoped bool) bool {
	return scoped && run.Build
}

type rowRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func agentOnlyFields(finish bool, planned *int, row *rowRef, inBuild bool) (Result, bool) {
	if inBuild || (!finish && planned == nil && row == nil) {
		return Result{}, true
	}
	return invalid("%s", agentOnlyBuildFields), false
}

func buildRequestID(run Run) (pgtype.UUID, error) {
	id, err := requestIDOf(run, true)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if !id.Valid {
		return pgtype.UUID{}, fmt.Errorf("%w: a build run needs a request id", errInvalidRun)
	}
	return id, nil
}

func (r *Registry) storedBuild(ctx context.Context, run Run) (dashboards.Build, bool, error) {
	id, err := buildRequestID(run)
	if err != nil {
		return dashboards.Build{}, false, err
	}
	return r.deps.Dashboards.Build(ctx, id)
}

func (r *Registry) buildBase(ctx context.Context, run Run) (base *spec.Dashboard, plannedBefore int, err error) {
	build, found, err := r.storedBuild(ctx, run)
	if err != nil {
		return nil, 0, err
	}
	if found {
		return build.Spec, build.PlannedPanels, nil
	}
	if run.Mode != ModeRevise {
		return nil, 0, nil
	}
	id, err := validateRun(run)
	if err != nil {
		return nil, 0, err
	}
	base, err = r.deps.Dashboards.VersionSpec(ctx, id, run.ParentVersion)
	return base, 0, err
}

func (r *Registry) saveToBuild(ctx context.Context, in buildSave) (Result, error) {
	save := dashboards.BuildSave{
		RequestID:     in.requestID,
		Attempt:       in.run.Attempt,
		DashboardID:   in.dashboardID,
		Spec:          in.raw,
		Summary:       in.summary,
		PlannedPanels: in.planned,
	}
	if in.dashboardID.Valid {
		save.BaseVersion = in.run.ParentVersion
	}
	build, err := r.deps.Dashboards.SaveBuild(ctx, in.principal, save)
	var invalidSpec *dashboards.InvalidSpecError
	if errors.As(err, &invalidSpec) && in.panelIDs != nil {
		return buildRejected(invalidSpec.Issues, in.panelIDs), nil
	}
	if err != nil {
		return saveFailure(err)
	}
	progress := build.Progress()
	step, message := buildMessage(progress, in.changes, in.plannedBefore)
	if in.finish {
		return r.commitBuild(ctx, in, step)
	}
	res := success(step, buildOut{
		Build:         buildStateOut{Saves: build.Saves, Panels: progress.Panels, PlannedPanels: progress.PlannedPanels},
		ChangedPanels: in.changes.shown,
		Message:       message,
	})
	res.Edits = in.changes.edits
	return res, nil
}

func (r *Registry) commitBuild(ctx context.Context, in buildSave, saveStep string) (Result, error) {
	dash, ver, replayed, err := r.deps.Dashboards.CommitBuild(ctx, in.principal, in.requestID, in.run.Attempt)
	if err != nil {
		return saveFailure(err)
	}
	return success(finishStep(saveStep, in.changes, ver.Number, versionShape(ver.Spec)), r.savedOut(dash, ver, &replayed)), nil
}

func finishStep(saveStep string, changes panelChanges, version int, shape string) string {
	if changes.added == nil && changes.edited == 0 {
		return fmt.Sprintf("Saved version %d (%s)", version, shape)
	}
	return fmt.Sprintf("%s, then saved version %d (%s)", saveStep, version, shape)
}

func versionShape(d *spec.Dashboard) string {
	rows, panels := 0, 0
	for _, p := range d.Panels {
		if p.Type == spec.Row {
			rows++
			panels += len(p.Panels)
			continue
		}
		panels++
	}
	if rows == 0 {
		return plural(panels, "panel", "panels")
	}
	return plural(rows, "row", "rows") + ", " + plural(panels, "panel", "panels")
}

func changesOf(prev, next *spec.Dashboard) panelChanges {
	known := map[string]bool{}
	if prev != nil {
		for _, e := range prev.AllPanels() {
			known[e.Panel.ID] = true
		}
	}
	edited, _ := dashboards.ChangedSince(prev, next)
	changes := panelChanges{shown: []panelOut{}, edits: editsOf(prev, next)}
	for _, e := range next.AllPanels() {
		isNew := !known[e.Panel.ID]
		if !isNew && !slices.Contains(edited, e.Panel.ID) {
			continue
		}
		if len(changes.shown) < maxChangedPanels {
			changes.shown = append(changes.shown, panelOut{ID: e.Panel.ID, Title: e.Panel.Title, Type: e.Panel.Type})
		}
		if e.Panel.Type == spec.Row {
			continue
		}
		changes.count++
		switch {
		case !isNew:
			changes.edited++
		case changes.added == nil:
			changes.added = &addedPanel{Title: e.Panel.Title}
			if e.Row != nil {
				changes.added.Row = e.Row.Title
			}
		}
	}
	return changes
}

func editsOf(prev, next *spec.Dashboard) Edits {
	if prev == nil {
		prev = &spec.Dashboard{}
	}
	was := map[string]*spec.Panel{}
	for _, e := range prev.AllPanels() {
		was[e.Panel.ID] = e.Panel
	}
	var edits Edits
	kept := map[string]bool{}
	for _, e := range next.AllPanels() {
		kept[e.Panel.ID] = true
		old, ok := was[e.Panel.ID]
		if !ok {
			edits.Added = append(edits.Added, e.Panel.ID)
		}
		if !ok || !sameJSON(ownFields(old), ownFields(e.Panel)) {
			edits.Panels = append(edits.Panels, e.Panel.ID)
		}
	}
	for _, e := range prev.AllPanels() {
		if !kept[e.Panel.ID] {
			edits.Deleted = append(edits.Deleted, e.Panel.ID)
		}
	}
	edits.Settings = !sameJSON(withoutPanels(prev), withoutPanels(next))
	return edits
}

func ownFields(p *spec.Panel) *spec.Panel {
	own := *p
	own.Panels = nil
	return &own
}

func withoutPanels(d *spec.Dashboard) *spec.Dashboard {
	head := *d
	head.Panels = nil
	return &head
}

func sameJSON(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

func buildMessage(p dashboards.BuildProgress, c panelChanges, plannedBefore int) (step, message string) {
	planned := p.PlannedPanels > 0 && p.Panels <= p.PlannedPanels
	switch {
	case c.added != nil:
		step = fmt.Sprintf("Saved panel %d", p.Panels)
		if planned {
			step += fmt.Sprintf(" of %d", p.PlannedPanels)
		}
		step += ": " + c.added.Title
		if c.added.Row != "" {
			step += " (row " + c.added.Row + ")"
		}
	case c.edited > 0:
		step = fmt.Sprintf("Changed %s in the build (%d in all)", plural(c.edited, "panel", "panels"), p.Panels)
	default:
		step = fmt.Sprintf("Saved the build (%s)", plural(p.Panels, "panel", "panels"))
	}
	if plannedBefore > 0 && p.PlannedPanels != plannedBefore {
		step += fmt.Sprintf(" (planned %d -> %d)", plannedBefore, p.PlannedPanels)
	}
	switch {
	case plannedBefore > 0 && p.PlannedPanels < plannedBefore:
		return step, fmt.Sprintf(loweredPlanFormat, plannedBefore, p.PlannedPanels)
	case p.PlannedPanels > 0 && p.Panels >= p.PlannedPanels:
		return step, allPanelsBuiltMessage
	case p.PlannedPanels > 0:
		return step, fmt.Sprintf("Panel %d of %d saved to the build. Next: add one panel with update_panels, alone in upsert, with row naming its row, and check its queries in the same call. Set finish: true on your last save.", p.Panels, p.PlannedPanels)
	}
	return step, fmt.Sprintf("The build holds %s. Add the next one with update_panels, one panel per save, and set finish: true on your last save, or answer with one short sentence and no tool call.", plural(p.Panels, "panel", "panels"))
}

func (r *Registry) readBuildPanels(ctx context.Context, run Run, scoped bool, in getPanelsInput) (Result, bool, error) {
	if !inBuildRun(run, scoped) || in.Version != nil {
		return Result{}, false, nil
	}
	if in.DashboardID != "" {
		if run.Mode == ModeCreate {
			return Result{}, false, nil
		}
		id, res, ok := parseDashboardID(in.DashboardID)
		if !ok {
			return res, true, nil
		}
		if runID, err := db.ParseUUID(run.DashboardID); err != nil || runID != id {
			return Result{}, false, nil
		}
	}
	build, found, err := r.storedBuild(ctx, run)
	if err != nil {
		return Result{}, false, err
	}
	if !found {
		if run.Mode == ModeCreate {
			return invalid("%s", noBuildYet), true, nil
		}
		return Result{}, false, nil
	}
	dashboardID := ""
	if build.DashboardID.Valid {
		dashboardID = build.DashboardID.String()
	}
	out, err := readPanels(dashboardID, build.BaseVersion, build.Spec, in.IDs)
	if err != nil {
		return Result{}, true, err
	}
	out.Build = true
	return success("Read "+plural(len(out.Panels), "panel", "panels")+" of the build", out), true, nil
}

func (r *Registry) hasBuild(ctx context.Context, run Run) (bool, error) {
	_, found, err := r.storedBuild(ctx, run)
	return found, err
}

func (r *Registry) buildCreate(ctx context.Context, principal auth.Principal, run Run, in createInput) (Result, error) {
	requestID, err := buildRequestID(run)
	if err != nil {
		return Result{}, err
	}
	prev, plannedBefore, err := r.buildBase(ctx, run)
	if err != nil {
		return saveFailure(err)
	}
	if prev != nil {
		return invalid("%s", hasBuildAlready), nil
	}
	raw := in.Spec
	if in.Row != nil {
		placed, res, err := placeFirstPanel(in.Spec, in.Row)
		if placed == nil {
			return res, err
		}
		raw = placed
	}
	save := buildSave{
		principal: principal, run: run, requestID: requestID, plannedBefore: plannedBefore,
		raw: raw, summary: summaryOr(in.Summary, defaultCreateSummary),
		planned: in.PlannedPanels, finish: in.Finish,
	}
	if parsed, err := spec.Parse(raw); err == nil {
		save.changes = changesOf(nil, parsed)
		save.panelIDs = panelIDsOf(parsed)
		if res, ok := onePanelPerSave(save.changes); !ok {
			return res, nil
		}
	}
	return r.saveToBuild(ctx, save)
}

func placeFirstPanel(raw json.RawMessage, row *rowRef) (json.RawMessage, Result, error) {
	var doc map[string]json.RawMessage
	var panels []json.RawMessage
	if json.Unmarshal(raw, &doc) != nil {
		return raw, Result{}, nil
	}
	if json.Unmarshal(doc["panels"], &panels) != nil || len(panels) != 1 {
		return nil, invalid("%s", rowNeedsFirstPanel), nil
	}
	empty := &spec.Dashboard{}
	np, res, ok := newPanelIn(empty, spec.PanelPatch{Upsert: panels}, row)
	if !ok {
		return nil, res, nil
	}
	if np == nil {
		return raw, Result{}, nil
	}
	doc["panels"], _ = json.Marshal(placeNewPanel(empty, *np, row).Upsert)
	placed, err := json.Marshal(doc)
	return placed, Result{}, err
}

func (r *Registry) buildUpdate(ctx context.Context, principal auth.Principal, run Run, target saveTarget, in saveInput) (Result, error) {
	requestID, err := buildRequestID(run)
	if err != nil {
		return Result{}, err
	}
	base, plannedBefore, err := r.buildBase(ctx, run)
	if err != nil {
		return saveFailure(err)
	}
	if base == nil {
		return invalid("%s", noBuildYet), nil
	}
	save := buildSave{
		principal: principal, run: run, requestID: requestID, dashboardID: target.id, plannedBefore: plannedBefore,
		raw: in.Spec, summary: summaryOr(in.Summary, defaultUpdateSummary),
		planned: in.PlannedPanels, finish: in.Finish,
	}
	if parsed, err := spec.Parse(in.Spec); err == nil {
		if issues := importOnlyIssues(parsed, base); len(issues) > 0 {
			return importOnlyFailure(issues), nil
		}
		save.changes = changesOf(base, parsed)
		save.panelIDs = panelIDsOf(parsed)
		if res, ok := onePanelPerSave(save.changes); !ok {
			return res, nil
		}
	}
	return r.saveToBuild(ctx, save)
}

func (r *Registry) buildPatch(ctx context.Context, principal auth.Principal, run Run, target saveTarget, in updatePanelsInput) (Result, error) {
	requestID, err := buildRequestID(run)
	if err != nil {
		return Result{}, err
	}
	base, plannedBefore, err := r.buildBase(ctx, run)
	if err != nil {
		return saveFailure(err)
	}
	if base == nil {
		return invalid("%s", noBuildYet), nil
	}
	merged, res, err := mergePatch(base, in.PanelPatch, in.Row)
	if merged == nil {
		return res, err
	}
	if issues := importOnlyIssues(merged, base); len(issues) > 0 {
		return buildRejected(issues, panelIDsOf(merged)), nil
	}
	changes := changesOf(base, merged)
	if res, ok := onePanelPerSave(changes); !ok {
		return res, nil
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return Result{}, err
	}
	return r.saveToBuild(ctx, buildSave{
		principal: principal, run: run, requestID: requestID, dashboardID: target.id,
		raw: raw, summary: in.Summary, planned: in.PlannedPanels, plannedBefore: plannedBefore, finish: in.Finish,
		changes: changes, panelIDs: panelIDsOf(merged),
	})
}

func onePanelPerSave(c panelChanges) (Result, bool) {
	if c.count <= 1 {
		return Result{}, true
	}
	var ids []string
	for _, p := range c.shown {
		if p.Type != spec.Row {
			ids = append(ids, p.ID)
		}
	}
	return tooManyPanels(c.count, ids), false
}

func tooManyPanels(n int, ids []string) Result {
	ids = ids[:min(len(ids), maxListedIDs)]
	res := invalid("one panel per save: this save adds or changes %d panels (%s); save them one at a time", n, strings.Join(ids, ", "))
	res.Refusal = RefusalOnePanel
	return res
}

type upsertHead struct {
	ID   string         `json:"id"`
	Type spec.PanelType `json:"type"`
}

type newPanel struct {
	head upsertHead
	body map[string]json.RawMessage
}

func newPanelIn(base *spec.Dashboard, p spec.PanelPatch, row *rowRef) (*newPanel, Result, bool) {
	if len(p.Upsert) > 1 {
		return nil, tooManyPanels(len(p.Upsert), upsertIDs(p.Upsert)), false
	}
	var panel newPanel
	isNew := len(p.Upsert) == 1 &&
		json.Unmarshal(p.Upsert[0], &panel.head) == nil && json.Unmarshal(p.Upsert[0], &panel.body) == nil &&
		!panelExists(base, panel.head.ID)
	switch {
	case isNew && panel.head.Type == spec.Row:
		return nil, invalid("%s", rowNeedsPanel), false
	case !isNew && row != nil:
		return nil, invalid("%s", rowNeedsNewPanel), false
	case !isNew:
		return nil, Result{}, true
	}
	if res, ok := placementAllowed(base, p.Layout, panel.head.ID, row); !ok {
		return nil, res, false
	}
	return &panel, Result{}, true
}

func placeNewPanel(staged *spec.Dashboard, np newPanel, row *rowRef) spec.PanelPatch {
	rowID := ""
	if row != nil {
		rowID = row.ID
	}
	w, h := panelSize(np.body)
	pl := place(staged, rowID, w, h)
	panel := withGridPos(np.body, pl.Panel)
	var placed spec.PanelPatch
	switch {
	case pl.Row != nil:
		placed.Upsert = []json.RawMessage{newRowUpsert(row, *pl.Row, panel)}
	default:
		placed.Upsert = []json.RawMessage{panel}
		if rowID != "" {
			placed.Layout = append(placed.Layout, spec.PanelPlacement{ID: np.head.ID, Row: &rowID})
		}
	}
	placed.Layout = append(placed.Layout, pl.Shifts...)
	return placed
}

func mergePatch(base *spec.Dashboard, p spec.PanelPatch, row *rowRef) (*spec.Dashboard, Result, error) {
	np, res, ok := newPanelIn(base, p, row)
	if !ok {
		return nil, res, nil
	}
	if np == nil {
		return applyBuildPatch(base, p)
	}
	rest := p
	rest.Upsert = nil
	staged := base
	if changesAnything(rest) {
		var err error
		if staged, res, err = applyBuildPatch(base, rest); staged == nil {
			return nil, res, err
		}
	}
	return applyBuildPatch(staged, placeNewPanel(staged, *np, row))
}

func applyBuildPatch(d *spec.Dashboard, p spec.PanelPatch) (*spec.Dashboard, Result, error) {
	merged, err := spec.ApplyPanelPatch(d, p)
	var patchErr *spec.PatchError
	if errors.As(err, &patchErr) {
		return nil, buildRejected(patchErr.Issues, patchErr.PanelIDs), nil
	}
	if err != nil {
		return nil, Result{}, err
	}
	return merged, Result{}, nil
}

func changesAnything(p spec.PanelPatch) bool {
	d := p.Dashboard
	return len(p.Delete) > 0 || len(p.Layout) > 0 || len(p.Variables) > 0 ||
		d != nil && (len(d.Time) > 0 || len(d.Title) > 0 || len(d.Tags) > 0 || len(d.Links) > 0)
}

func placementAllowed(base *spec.Dashboard, layout []spec.PanelPlacement, panelID string, row *rowRef) (Result, bool) {
	if slices.ContainsFunc(layout, func(l spec.PanelPlacement) bool { return l.ID == panelID }) {
		return invalid("%s", serverPlacesPanel), false
	}
	if row == nil {
		return Result{}, true
	}
	if e, found := base.FindPanel(row.ID); found && (e.Row != nil || e.Panel.Type != spec.Row) {
		return invalid("panel %q is not a row", row.ID), false
	}
	return Result{}, true
}

func panelExists(d *spec.Dashboard, id string) bool {
	_, found := d.FindPanel(id)
	return found
}

func upsertIDs(items []json.RawMessage) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		var head upsertHead
		_ = json.Unmarshal(item, &head)
		ids = append(ids, head.ID)
	}
	return ids
}

func panelSize(body map[string]json.RawMessage) (w, h int) {
	var pos struct {
		W *float64 `json:"w"`
		H *float64 `json:"h"`
	}
	if json.Unmarshal(body["gridPos"], &pos) != nil {
		return defaultPanelW, defaultPanelH
	}
	w, wOK := wholeWithin(pos.W, maxPanelW)
	h, hOK := wholeWithin(pos.H, maxPanelH)
	if !wOK || !hOK {
		return defaultPanelW, defaultPanelH
	}
	return w, h
}

func wholeWithin(v *float64, limit int) (int, bool) {
	if v == nil || *v != math.Trunc(*v) || *v < 1 || *v > float64(limit) {
		return 0, false
	}
	return int(*v), true
}

func withGridPos(body map[string]json.RawMessage, pos spec.GridPos) json.RawMessage {
	placed := maps.Clone(body)
	placed["gridPos"], _ = json.Marshal(pos)
	raw, _ := json.Marshal(placed)
	return raw
}

func newRowUpsert(row *rowRef, pos spec.GridPos, panel json.RawMessage) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"id":      row.ID,
		"type":    spec.Row,
		"title":   row.Title,
		"gridPos": pos,
		"panels":  []json.RawMessage{panel},
	})
	return raw
}
