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

	maxPanelW     = spec.GridColumns
	maxPanelH     = 100
	maxPreferredY = 1 << 20

	maxLayoutBytes  = 1536
	maxLayoutPanels = 24
	maxLayoutGaps   = 8

	noBuildYet            = "this run has no build yet; call create_dashboard first with the title, time, variables and the first panel"
	agentOnlyBuildFields  = "finish, planned_panels and row are only for the built-in agent"
	importInBuildRun      = "this run is building a dashboard; continue with update_panels"
	hasBuildAlready       = "this run already has a build; add panels with update_panels"
	buildHasNoID          = "dashboard_id must be a UUID from list_dashboards; your build has no id until you finish, so read its panels with get_panels without dashboard_id"
	rowNeedsPanel         = "name the row in row with its first panel"
	rowNeedsFirstPanel    = "row places the first panel of a new dashboard; send exactly one panel in spec"
	rowsStayInPlace       = "rows keep their place in a build; move a panel with row, or resize it with its w and h"
	newPanelRowOnce       = "name the new panel's row once, in row"
	newRowNeedsItsPanel   = "a new row in update_dashboard needs its first panel and nothing else; add rows with update_panels"
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
	Layout        *layoutOut    `json:"layout,omitempty"`
	Message       string        `json:"message"`
}

type layoutOut struct {
	Rows      []string      `json:"rows"`
	Row       *rowLayoutOut `json:"row,omitempty"`
	Open      string        `json:"open,omitempty"`
	Gaps      []string      `json:"gaps,omitempty"`
	Truncated bool          `json:"truncated,omitempty"`
}

type rowLayoutOut struct {
	ID     string   `json:"id"`
	Panels []string `json:"panels"`
}

type buildStateOut struct {
	Saves         int `json:"saves"`
	Panels        int `json:"panels"`
	PlannedPanels int `json:"planned_panels,omitempty"`
}

type addedPanel struct {
	ID    string
	Title string
	Row   string
}

type panelChanges struct {
	shown  []panelOut
	count  int
	added  *addedPanel
	edited int
	moved  []string
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
	holes := gaps(build.Spec)
	step, message := buildMessage(progress, in.changes, in.plannedBefore, len(holes))
	if in.finish {
		return r.commitBuild(ctx, in, step)
	}
	res := success(step, buildOut{
		Build:         buildStateOut{Saves: build.Saves, Panels: progress.Panels, PlannedPanels: progress.PlannedPanels},
		ChangedPanels: in.changes.shown,
		Layout:        layoutOf(build.Spec, in.changes.focus(), holes),
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
			changes.added = &addedPanel{ID: e.Panel.ID, Title: e.Panel.Title}
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

func buildMessage(p dashboards.BuildProgress, c panelChanges, plannedBefore, gapCount int) (step, message string) {
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
	case len(c.moved) > 0:
		step = fmt.Sprintf("Moved %s in the build (%d in all)", plural(len(c.moved), "panel", "panels"), p.Panels)
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
		return step, allPanelsBuiltMessage + gapSentence(gapCount)
	case p.PlannedPanels > 0:
		return step, fmt.Sprintf("Panel %d of %d saved to the build. Next: add one panel with update_panels, alone in upsert, with row naming its row, and check its queries in the same call. Set finish: true on your last save.", p.Panels, p.PlannedPanels)
	}
	return step, fmt.Sprintf("The build holds %s. Add the next one with update_panels, one panel per save, and set finish: true on your last save, or answer with one short sentence and no tool call.", plural(p.Panels, "panel", "panels"))
}

func gapSentence(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" The layout has %s, listed in layout.gaps; you can close one by resending a panel beside it with a larger w or h.", plural(n, "gap", "gaps"))
}

func (c panelChanges) focus() string {
	switch {
	case c.added != nil:
		return c.added.ID
	case len(c.moved) > 0:
		return c.moved[len(c.moved)-1]
	}
	for _, p := range c.shown {
		if p.Type != spec.Row {
			return p.ID
		}
	}
	return ""
}

func layoutOf(d *spec.Dashboard, focus string, holes []gap) *layoutOut {
	out := &layoutOut{Rows: sectionSummaries(d)}
	if e, found := d.FindPanel(focus); found {
		section := sectionOf(e)
		out.Row = &rowLayoutOut{ID: section, Panels: sectionPanels(d, section)}
		if o := newWorkingLayout(d).openLine(section); o.W > 0 {
			out.Open = fmt.Sprintf("%s %s", sectionName(section), positionText(spec.GridPos{X: o.X, Y: o.Y, W: o.W, H: o.H}))
		}
	}
	for _, g := range holes[:min(len(holes), maxLayoutGaps)] {
		out.Gaps = append(out.Gaps, fmt.Sprintf("%s %s", sectionName(g.Row), positionText(g.Pos)))
	}
	out.fit()
	return out
}

func sectionName(rowID string) string {
	if rowID == "" {
		return "(top level)"
	}
	return rowID
}

func positionText(g spec.GridPos) string {
	return fmt.Sprintf("x%d y%d w%d h%d", g.X, g.Y, g.W, g.H)
}

func sectionSummaries(d *spec.Dashboard) []string {
	rows := []string{}
	loose := 0
	for _, p := range d.Panels {
		if p.Type != spec.Row {
			loose++
		}
	}
	if loose > 0 {
		rows = append(rows, fmt.Sprintf("%s %d", sectionName(""), loose))
	}
	for _, p := range d.Panels {
		if p.Type == spec.Row {
			rows = append(rows, fmt.Sprintf("%s %q %d", p.ID, p.Title, len(p.Panels)))
		}
	}
	return rows
}

func sectionPanels(d *spec.Dashboard, rowID string) []string {
	var members []spec.Panel
	for _, p := range d.Panels {
		switch {
		case rowID == "" && p.Type != spec.Row:
			members = append(members, p)
		case rowID != "" && p.ID == rowID:
			members = p.Panels
		}
	}
	panels := []string{}
	for _, p := range members[:min(len(members), maxLayoutPanels)] {
		panels = append(panels, p.ID+" "+positionText(p.GridPos))
	}
	return panels
}

func (l *layoutOut) fit() {
	for layoutSize(l) > maxLayoutBytes {
		l.Truncated = true
		switch {
		case len(l.Rows) > 0:
			l.Rows = l.Rows[:len(l.Rows)-1]
		case len(l.Gaps) > 0:
			l.Gaps = l.Gaps[:len(l.Gaps)-1]
		case l.Row != nil && len(l.Row.Panels) > 0:
			l.Row.Panels = l.Row.Panels[:len(l.Row.Panels)-1]
		default:
			return
		}
	}
}

func layoutSize(l *layoutOut) int {
	raw, _ := json.Marshal(l)
	return len(raw)
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
	raw, res, err := placeFirstPanel(in.Spec, in.Row)
	if raw == nil {
		return res, err
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
		if row != nil {
			return nil, invalid("%s", rowNeedsFirstPanel), nil
		}
		return raw, Result{}, nil
	}
	np, ok := decodeNewPanel(panels[0])
	if !ok {
		return raw, Result{}, nil
	}
	if np.head.Type == spec.Row {
		if row != nil {
			return nil, invalid("%s", rowNeedsPanel), nil
		}
		if np, row, ok = onlyChildOfRow(np); !ok {
			return raw, Result{}, nil
		}
	}
	placed := placement(&spec.Dashboard{}, []slot{np.slot(row, nil)}, np, row)
	doc["panels"], _ = json.Marshal(placed.patch.Upsert)
	out, err := json.Marshal(doc)
	return out, Result{}, err
}

func onlyChildOfRow(row *newPanel) (*newPanel, *rowRef, bool) {
	var children []json.RawMessage
	var title string
	if json.Unmarshal(row.body["panels"], &children) != nil || len(children) != 1 {
		return nil, nil, false
	}
	_ = json.Unmarshal(row.body["title"], &title)
	child, ok := decodeNewPanel(children[0])
	if !ok || child.head.Type == spec.Row {
		return nil, nil, false
	}
	return child, &rowRef{ID: row.head.ID, Title: title}, true
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
	raw, parsed, res, ok, err := stageUpdate(base, in.Spec)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return res, nil
	}
	save := buildSave{
		principal: principal, run: run, requestID: requestID, dashboardID: target.id, plannedBefore: plannedBefore,
		raw: raw, summary: summaryOr(in.Summary, defaultUpdateSummary),
		planned: in.PlannedPanels, finish: in.Finish,
	}
	if parsed != nil {
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

func stageUpdate(base *spec.Dashboard, raw json.RawMessage) (json.RawMessage, *spec.Dashboard, Result, bool, error) {
	kept, added, row, res, ok := keepBuildPositions(base, raw)
	if !ok {
		return nil, nil, res, false, nil
	}
	parsed, err := spec.Parse(kept)
	if err != nil {
		return kept, nil, Result{}, true, nil
	}
	if added == nil {
		return kept, parsed, Result{}, true, nil
	}
	placed := placement(parsed, []slot{added.slot(row, nil)}, added, row)
	merged, res, err := applyBuildPatch(parsed, placed.patch)
	if merged == nil {
		return nil, nil, res, false, err
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return nil, nil, Result{}, false, err
	}
	return out, merged, Result{}, true, nil
}

type positionKeeper struct {
	base  *spec.Dashboard
	added []*newPanel
	row   *rowRef
}

func (k *positionKeeper) keep(panel map[string]json.RawMessage) (known bool) {
	var id string
	_ = json.Unmarshal(panel["id"], &id)
	e, found := k.base.FindPanel(id)
	if found {
		panel["gridPos"], _ = json.Marshal(e.Panel.GridPos)
	}
	return found
}

func (k *positionKeeper) collect(panel map[string]json.RawMessage, row *rowRef) bool {
	raw, _ := json.Marshal(panel)
	np, ok := decodeNewPanel(raw)
	if ok {
		k.added = append(k.added, np)
		k.row = row
	}
	return ok
}

func keepBuildPositions(base *spec.Dashboard, raw json.RawMessage) (json.RawMessage, *newPanel, *rowRef, Result, bool) {
	var doc map[string]json.RawMessage
	var panels []map[string]json.RawMessage
	if json.Unmarshal(raw, &doc) != nil || json.Unmarshal(doc["panels"], &panels) != nil {
		return raw, nil, nil, Result{}, true
	}
	k := &positionKeeper{base: base}
	kept := make([]map[string]json.RawMessage, 0, len(panels))
	for _, p := range panels {
		stays, res, ok := k.visit(p)
		if !ok {
			return nil, nil, nil, res, false
		}
		if stays {
			kept = append(kept, p)
		}
	}
	if len(k.added) > 1 {
		ids := make([]string, len(k.added))
		for i, np := range k.added {
			ids[i] = np.head.ID
		}
		return nil, nil, nil, tooManyPanels(len(ids), ids), false
	}
	doc["panels"], _ = json.Marshal(kept)
	out, _ := json.Marshal(doc)
	if len(k.added) == 0 {
		return out, nil, nil, Result{}, true
	}
	return out, k.added[0], k.row, Result{}, true
}

func (k *positionKeeper) visit(p map[string]json.RawMessage) (stays bool, res Result, ok bool) {
	known := k.keep(p)
	if !isRowBody(p) {
		return known || !k.collect(p, nil), Result{}, true
	}
	row, children, split := splitRow(p)
	switch {
	case !split:
		return true, Result{}, true
	case !known:
		res, ok := k.collectNewRow(row, children)
		return false, res, ok
	}
	remaining := make([]map[string]json.RawMessage, 0, len(children))
	for _, c := range children {
		if !k.keep(c) && !isRowBody(c) && k.collect(c, row) {
			continue
		}
		remaining = append(remaining, c)
	}
	p["panels"], _ = json.Marshal(remaining)
	return true, Result{}, true
}

func (k *positionKeeper) collectNewRow(row *rowRef, children []map[string]json.RawMessage) (Result, bool) {
	if len(children) != 1 || isRowBody(children[0]) || k.keep(children[0]) || !k.collect(children[0], row) {
		return invalid("%s", newRowNeedsItsPanel), false
	}
	return Result{}, true
}

func isRowBody(panel map[string]json.RawMessage) bool {
	var panelType spec.PanelType
	_ = json.Unmarshal(panel["type"], &panelType)
	return panelType == spec.Row
}

func splitRow(row map[string]json.RawMessage) (*rowRef, []map[string]json.RawMessage, bool) {
	var ref rowRef
	var children []map[string]json.RawMessage
	_ = json.Unmarshal(row["id"], &ref.ID)
	_ = json.Unmarshal(row["title"], &ref.Title)
	if raw, ok := row["panels"]; ok && json.Unmarshal(raw, &children) != nil {
		return nil, nil, false
	}
	return &ref, children, true
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
	merged, moved, res, err := mergePatch(base, in.PanelPatch, in.Row)
	if merged == nil {
		return res, err
	}
	if issues := importOnlyIssues(merged, base); len(issues) > 0 {
		return buildRejected(issues, panelIDsOf(merged)), nil
	}
	changes := changesOf(base, merged)
	changes.moved = moved
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

func decodeNewPanel(raw json.RawMessage) (*newPanel, bool) {
	var np newPanel
	if json.Unmarshal(raw, &np.head) != nil || json.Unmarshal(raw, &np.body) != nil {
		return nil, false
	}
	return &np, true
}

func (np *newPanel) slot(row *rowRef, prefer *spec.GridPos) slot {
	s := slot{ID: np.head.ID, Type: np.head.Type, Prefer: prefer}
	if row != nil {
		s.Row, s.RowTitle = row.ID, row.Title
	}
	given := givenSize(np.body["gridPos"])
	s.W, s.H = given.W, given.H
	return s
}

type buildPlan struct {
	rest     spec.PanelPatch
	layout   []spec.PanelPlacement
	row      *rowRef
	added    *newPanel
	resent   *newPanel
	resentAt spec.PanelEntry
}

func buildPatchPlan(base *spec.Dashboard, p spec.PanelPatch, row *rowRef) (buildPlan, Result, bool) {
	if len(p.Upsert) > 1 {
		return buildPlan{}, tooManyPanels(len(p.Upsert), upsertIDs(p.Upsert)), false
	}
	if row != nil {
		if e, found := base.FindPanel(row.ID); found && (e.Row != nil || e.Panel.Type != spec.Row) {
			return buildPlan{}, invalid("panel %q is not a row", row.ID), false
		}
	}
	plan := buildPlan{rest: p, layout: p.Layout, row: row}
	plan.rest.Layout = nil
	if len(p.Upsert) == 0 {
		return plan, Result{}, true
	}
	np, ok := decodeNewPanel(p.Upsert[0])
	if !ok || np.head.ID == "" {
		return plan, Result{}, true
	}
	e, exists := base.FindPanel(np.head.ID)
	switch {
	case !exists && np.head.Type == spec.Row:
		return buildPlan{}, invalid("%s", rowNeedsPanel), false
	case !exists:
		plan.added = np
		plan.rest.Upsert = nil
	case e.Panel.Type == spec.Row && row != nil:
		return buildPlan{}, invalid("%s", rowsStayInPlace), false
	default:
		plan.resent, plan.resentAt = np, e
		plan.rest.Upsert = []json.RawMessage{withGridPos(np.body, e.Panel.GridPos)}
	}
	return plan, Result{}, true
}

func (b buildPlan) rowID() string {
	return rowIDOf(b.row)
}

func (b buildPlan) rowTitle(id string) string {
	if b.row != nil && b.row.ID == id {
		return b.row.Title
	}
	return ""
}

func sectionOf(e spec.PanelEntry) string {
	if e.Row == nil {
		return ""
	}
	return e.Row.ID
}

func (b buildPlan) slots(staged *spec.Dashboard) ([]slot, Result, bool) {
	var slots []slot
	named := map[string]bool{}
	var wanted *spec.GridPos
	for _, l := range b.layout {
		if named[l.ID] {
			return nil, invalid("panel %q appears more than once in layout", l.ID), false
		}
		named[l.ID] = true
		if b.added != nil && l.ID == b.added.head.ID {
			if l.Row != nil && *l.Row != b.rowID() {
				return nil, invalid("%s", newPanelRowOnce), false
			}
			wanted = preferredSpot(l.GridPos)
			continue
		}
		s, res, ok := b.layoutSlot(staged, l)
		if !ok {
			return nil, res, false
		}
		slots = append(slots, s)
	}
	if b.resent != nil && !named[b.resent.head.ID] {
		if s, ok := b.resentSlot(); ok {
			slots = append(slots, s)
		}
	}
	if b.added != nil {
		slots = append(slots, b.added.slot(b.row, wanted))
	}
	return slots, Result{}, true
}

func (b buildPlan) layoutSlot(staged *spec.Dashboard, l spec.PanelPlacement) (slot, Result, bool) {
	e, found := staged.FindPanel(l.ID)
	switch {
	case !found:
		return slot{}, invalid("no panel with id %q to place", l.ID), false
	case e.Panel.Type == spec.Row:
		return slot{}, invalid("%s", rowsStayInPlace), false
	}
	section := sectionOf(e)
	target := section
	if l.Row != nil {
		target = *l.Row
		if res, ok := b.rowTarget(staged, target); !ok {
			return slot{}, res, false
		}
	}
	stored := e.Panel.GridPos
	w, h := givenSize(l.GridPos).or(stored)
	prefer := preferredSpot(l.GridPos)
	if prefer == nil && target == section {
		prefer = &spec.GridPos{X: stored.X, Y: stored.Y}
	}
	return slot{ID: l.ID, Type: e.Panel.Type, Row: target, RowTitle: b.rowTitle(target), W: w, H: h, Prefer: prefer}, Result{}, true
}

func (b buildPlan) rowTarget(staged *spec.Dashboard, id string) (Result, bool) {
	if id == "" {
		return Result{}, true
	}
	e, found := staged.FindPanel(id)
	switch {
	case found && (e.Row != nil || e.Panel.Type != spec.Row):
		return invalid("panel %q is not a row", id), false
	case !found && b.rowID() != id:
		return invalid("no row with id %q to place the panel in; name a new row in row", id), false
	}
	return Result{}, true
}

func (b buildPlan) resentSlot() (slot, bool) {
	stored := b.resentAt.Panel.GridPos
	if b.resentAt.Panel.Type == spec.Row {
		return slot{}, false
	}
	section := sectionOf(b.resentAt)
	w, h := givenSize(b.resent.body["gridPos"]).or(stored)
	switch {
	case b.row != nil && b.row.ID != section:
		return slot{ID: b.resent.head.ID, Type: b.resentAt.Panel.Type, Row: b.row.ID, RowTitle: b.row.Title, W: w, H: h}, true
	case w != stored.W || h != stored.H:
		return slot{ID: b.resent.head.ID, Type: b.resentAt.Panel.Type, Row: section, W: w, H: h, Prefer: &spec.GridPos{X: stored.X, Y: stored.Y}}, true
	}
	return slot{}, false
}

type placedPatch struct {
	patch spec.PanelPatch
	moved []string
}

func placement(staged *spec.Dashboard, slots []slot, added *newPanel, row *rowRef) placedPatch {
	plan := place(staged, slots)
	var placed placedPatch
	slotted := map[string]bool{}
	carried := false
	created := map[string]bool{}
	for _, s := range slots {
		slotted[s.ID] = true
		pos, isNew := plan.NewRows[s.Row]
		if !isNew || created[s.Row] {
			continue
		}
		created[s.Row] = true
		var panel json.RawMessage
		if added != nil && s.ID == added.head.ID {
			panel = withGridPos(added.body, plan.Positions[s.ID])
			carried = true
		}
		placed.patch.Upsert = append(placed.patch.Upsert, rowUpsert(s.Row, s.RowTitle, pos, panel))
	}
	if added != nil && !carried {
		placed.patch.Upsert = append(placed.patch.Upsert, withGridPos(added.body, plan.Positions[added.head.ID]))
		if rowID := rowIDOf(row); rowID != "" {
			placed.patch.Layout = append(placed.patch.Layout, spec.PanelPlacement{ID: added.head.ID, Row: &rowID})
		}
	}
	placed.patch.Layout = append(placed.patch.Layout, plan.Moves...)
	for _, m := range plan.Moves {
		if slotted[m.ID] {
			placed.moved = append(placed.moved, m.ID)
		}
	}
	return placed
}

func rowIDOf(row *rowRef) string {
	if row == nil {
		return ""
	}
	return row.ID
}

func mergePatch(base *spec.Dashboard, p spec.PanelPatch, row *rowRef) (*spec.Dashboard, []string, Result, error) {
	plan, res, ok := buildPatchPlan(base, p, row)
	if !ok {
		return nil, nil, res, nil
	}
	staged := base
	restChanges := changesAnything(plan.rest)
	if restChanges {
		var err error
		if staged, res, err = applyBuildPatch(base, plan.rest); staged == nil {
			return nil, nil, res, err
		}
	}
	slots, res, ok := plan.slots(staged)
	if !ok {
		return nil, nil, res, nil
	}
	if len(slots) == 0 && restChanges {
		return staged, nil, Result{}, nil
	}
	if len(slots) == 0 {
		merged, res, err := applyBuildPatch(base, plan.rest)
		return merged, nil, res, err
	}
	placed := placement(staged, slots, plan.added, row)
	merged, res, err := applyBuildPatch(staged, placed.patch)
	return merged, placed.moved, res, err
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
	return len(p.Upsert) > 0 || len(p.Delete) > 0 || len(p.Variables) > 0 ||
		d != nil && (len(d.Time) > 0 || len(d.Title) > 0 || len(d.Tags) > 0 || len(d.Links) > 0)
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

func preferredSpot(gridPos json.RawMessage) *spec.GridPos {
	var pos struct {
		X *float64 `json:"x"`
		Y *float64 `json:"y"`
	}
	if json.Unmarshal(gridPos, &pos) != nil {
		return nil
	}
	x, xOK := wholeWithin(pos.X, 0, spec.GridColumns)
	y, yOK := wholeWithin(pos.Y, 0, maxPreferredY)
	if !xOK || !yOK {
		return nil
	}
	return &spec.GridPos{X: x, Y: y}
}

func wholeWithin(v *float64, lowest, highest int) (int, bool) {
	if v == nil || *v != math.Trunc(*v) || *v < float64(lowest) || *v > float64(highest) {
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

func rowUpsert(id, title string, pos spec.GridPos, panel json.RawMessage) json.RawMessage {
	row := map[string]any{"id": id, "type": spec.Row, "title": title, "gridPos": pos}
	if panel != nil {
		row["panels"] = []json.RawMessage{panel}
	}
	raw, _ := json.Marshal(row)
	return raw
}
