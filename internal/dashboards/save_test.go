package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func twoPanels(q0, q1 string) []byte {
	return specDef{panels: []panelDef{{id: "p0", query: q0}, {id: "p1", query: q1}}}.json()
}

func (r *rig) staleDashboard(t *testing.T) pgtype.UUID {
	t.Helper()
	id := r.createSpec(t, specDef{panels: []panelDef{{id: "p0", query: "m0"}, {id: "p1", query: "m1"}}})
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, id, 1, twoPanels("m0", "x1"), "another panel"); err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) latestNumber(t *testing.T, id pgtype.UUID) int {
	t.Helper()
	n, err := r.service.LatestVersion(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func queryOf(t *testing.T, d *spec.Dashboard, id string) string {
	t.Helper()
	found, ok := d.FindPanel(id)
	if !ok {
		t.Fatalf("panel %s is missing", id)
	}
	return found.Panel.Queries[0].Query
}

func TestAStaleSaveWithoutMergeIsAConflictWithNoConflictList(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.staleDashboard(t)

	_, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: id, Parent: 1, Spec: twoPanels("y0", "m1"), Summary: "mine"})
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.Latest != 2 || conflict.Conflicts != nil {
		t.Fatalf("err = %v, want a conflict at version 2 with no conflict list", err)
	}
	if got := r.latestNumber(t, id); got != 2 {
		t.Fatalf("latest = %d, want 2", got)
	}
}

func TestAStaleSaveWithMergeLandsOnTopOfTheLatestWhenOtherPanelsChanged(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.staleDashboard(t)

	res, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: id, Parent: 1, Spec: twoPanels("y0", "m1"), Summary: "mine", Merge: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Replayed || res.Version.Number != 3 || res.Version.ParentVersion == nil || *res.Version.ParentVersion != 2 {
		t.Fatalf("version %d, parent %v, replayed %t, want version 3 on parent 2", res.Version.Number, res.Version.ParentVersion, res.Replayed)
	}
	if res.MergedFrom == nil || *res.MergedFrom != (MergedFrom{BaseVersion: 1, OntoVersion: 2}) {
		t.Fatalf("merged_from = %+v, want 1 onto 2", res.MergedFrom)
	}
	if res.Version.Summary != "mine" || res.Dashboard.LatestVersion != 3 {
		t.Fatalf("summary %q, dashboard latest %d", res.Version.Summary, res.Dashboard.LatestVersion)
	}
	if got, want := queryOf(t, res.Version.Spec, "p0"), "y0"; got != want {
		t.Fatalf("p0 query = %q, want %q", got, want)
	}
	if got, want := queryOf(t, res.Version.Spec, "p1"), "x1"; got != want {
		t.Fatalf("p1 query = %q, want %q", got, want)
	}
	stored, err := r.service.VersionSpec(t.Context(), id, 3)
	if err != nil || queryOf(t, stored, "p0") != "y0" || queryOf(t, stored, "p1") != "x1" {
		t.Fatalf("stored version 3 does not carry both changes: %v", err)
	}
}

func TestAStaleSaveWithMergeThatChangedTheSamePanelAnswersTheConflictAndSavesNothing(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.staleDashboard(t)

	_, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: id, Parent: 1, Spec: twoPanels("m0", "y1"), Summary: "mine", Merge: true})
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.Latest != 2 {
		t.Fatalf("err = %v, want a conflict at version 2", err)
	}
	if len(conflict.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v, want one both_changed on panel p1", conflict.Conflicts)
	}
	if c := conflict.Conflicts[0]; c.Kind != spec.UnitPanel || c.ID != "p1" || c.Reason != spec.ReasonBothChanged {
		t.Fatalf("conflict = %+v, want both_changed on panel p1", c)
	}
	if got := r.latestNumber(t, id); got != 2 {
		t.Fatalf("latest = %d, want 2", got)
	}
}

func TestAChoiceResolvesAConflictAndAnUnknownChoiceIsInvalidInput(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.staleDashboard(t)

	res, err := r.service.Save(t.Context(), testUser, SaveInput{
		DashboardID: id, Parent: 1, Spec: twoPanels("m0", "y1"), Summary: "mine", Merge: true,
		Choices: []spec.Choice{{Kind: spec.UnitPanel, ID: "p1", Take: spec.SideTheirs}},
	})
	if err != nil || res.Version.Number != 3 || queryOf(t, res.Version.Spec, "p1") != "y1" {
		t.Fatalf("version %d, err %v, want version 3 with the caller's p1", res.Version.Number, err)
	}

	_, err = r.service.Save(t.Context(), testUser, SaveInput{
		DashboardID: id, Parent: 2, Spec: twoPanels("m0", "z1"), Summary: "again", Merge: true,
		Choices: []spec.Choice{{Kind: spec.UnitPanel, ID: "nope", Take: spec.SideOurs}},
	})
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want invalid input", err)
	}
}

func TestAMergeNeverReadsAParentThatIsNotOlderThanTheLatest(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.staleDashboard(t)
	for _, parent := range []int{0, 7} {
		_, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: id, Parent: parent, Spec: twoPanels("y0", "m1"), Summary: "mine", Merge: true})
		var conflict *ConflictError
		if !errors.As(err, &conflict) || conflict.Latest != 2 || conflict.Conflicts != nil {
			t.Errorf("parent %d: err = %v, want a plain conflict at version 2", parent, err)
		}
	}
}

func TestAMergedSaveRunsThePanelQueriesItBrought(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.staleDashboard(t)
	p.setFail("bad0", true)

	_, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: id, Parent: 1, Spec: twoPanels("bad0", "m1"), Summary: "mine", Merge: true})
	invalid := wantInvalid(t, err)
	if len(invalid.Issues) == 0 {
		t.Fatal("no issues on the failing panel")
	}
	if got := r.latestNumber(t, id); got != 2 {
		t.Fatalf("latest = %d, want 2", got)
	}
}

func TestAPartialMergedSaveChecksOnlyWhatTheMergeBrought(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(5)})
	newer := numberedPanels(5)
	newer[3].query = "newer3"
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, id, 1, specDef{panels: newer}.json(), "newer"); err != nil {
		t.Fatal(err)
	}
	mine := numberedPanels(5)
	mine[0].query = "mine0"
	p.reset()

	res, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: id, Parent: 1, Spec: specDef{panels: mine}.json(), Summary: "mine", Merge: true, Partial: true})
	if err != nil || res.Version.Number != 3 {
		t.Fatalf("version %d, err %v, want version 3", res.Version.Number, err)
	}
	if ran := p.texts(); !slices.Equal(ran, []string{"mine0"}) {
		t.Fatalf("queries run = %v, want only the one the merge brought", ran)
	}
}

func TestAStaleSaveWhoseChangeIsAlreadyInTheLatestAnswersTheLatestAsAReplay(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: []panelDef{{id: "p0", query: "m0"}, {id: "p1", query: "m1"}}})
	if _, _, _, err := r.service.AddVersion(t.Context(), otherUser, id, 1, twoPanels("same", "x1"), "both"); err != nil {
		t.Fatal(err)
	}

	res, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: id, Parent: 1, Spec: twoPanels("same", "m1"), Summary: "mine", Merge: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Replayed || res.Version.Number != 2 || res.MergedFrom == nil || *res.MergedFrom != (MergedFrom{BaseVersion: 1, OntoVersion: 2}) {
		t.Fatalf("version %d, replayed %t, merged_from %+v, want the latest as a replay merged 1 onto 2", res.Version.Number, res.Replayed, res.MergedFrom)
	}
	if got := r.latestNumber(t, id); got != 2 {
		t.Fatalf("latest = %d, want 2", got)
	}
}

func TestARequestIdSaveRepeatedAfterSomeoneElseSavedReturnsTheFirstVersion(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t)
	request := r.queueRevision(t, dash)

	first, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: dash, Parent: 1, Spec: panelsSpec("t", "rps", "p99"), Summary: "first", RequestID: request, Merge: true})
	if err != nil || first.Replayed || first.Version.Number != 2 {
		t.Fatalf("first save: version %d, replayed %t, err %v", first.Version.Number, first.Replayed, err)
	}
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, dash, 2, panelsSpec("t", "rps"), "someone else"); err != nil {
		t.Fatal(err)
	}
	again, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: dash, Parent: 1, Spec: panelsSpec("t", "rps", "p99"), Summary: "first", RequestID: request, Merge: true})
	if err != nil || !again.Replayed || again.Version.Number != 2 || again.MergedFrom != nil {
		t.Fatalf("repeat: version %d, replayed %t, merged_from %+v, err %v", again.Version.Number, again.Replayed, again.MergedFrom, err)
	}
}

func TestInTxRunsInTheSavesTransaction(t *testing.T) {
	r := newRig(t, false)
	dash := r.createDashboard(t)
	boom := errors.New("boom")

	var seen int
	_, err := r.service.Save(t.Context(), testUser, SaveInput{
		DashboardID: dash, Parent: 1, Spec: panelsSpec("t", "rps", "p99"), Summary: "refused",
		InTx: func(_ *dbsqlc.Queries, version int) error {
			seen = version
			return boom
		},
	})
	if !errors.Is(err, boom) || seen != 2 {
		t.Fatalf("err = %v, version seen %d, want the hook's error at version 2", err, seen)
	}
	if got := r.latestNumber(t, dash); got != 1 {
		t.Fatalf("latest = %d, want the failed hook to leave no version", got)
	}

	res, err := r.service.Save(t.Context(), testUser, SaveInput{
		DashboardID: dash, Parent: 1, Spec: panelsSpec("t", "rps", "p99"), Summary: "kept",
		InTx: func(*dbsqlc.Queries, int) error { return nil },
	})
	if err != nil || res.Version.Number != 2 {
		t.Fatalf("version %d, err %v, want version 2", res.Version.Number, err)
	}
}

func TestACallerWhoCanOnlyCommentGetsAProposalOnlyWhenAskedAndWired(t *testing.T) {
	r := newRig(t, false)
	dash := r.createDashboard(t)
	var seen []ProposeInput
	r.service.SetProposer(func(_ context.Context, by auth.Principal, in ProposeInput) (ProposalRef, error) {
		seen = append(seen, in)
		return ProposalRef{ID: "0b0b0b0b-0b0b-4b0b-8b0b-0b0b0b0b0b0b", Revision: 1, Opened: true}, nil
	})
	save := func(by auth.Principal, propose bool) (SaveResult, error) {
		return r.service.Save(t.Context(), by, SaveInput{DashboardID: dash, Parent: 1, Spec: panelsSpec("t", "rps", "p99"), Summary: "idea", ProposeIfNeeded: propose})
	}

	res, err := save(commenterNoGrant, true)
	if err != nil || res.Proposal == nil || !res.Proposal.Opened || res.Version.Number != 0 {
		t.Fatalf("proposal %+v, version %d, err %v, want a proposal and no version", res.Proposal, res.Version.Number, err)
	}
	if len(seen) != 1 || seen[0].BaseVersion != 1 || seen[0].Summary != "idea" || seen[0].Spec == nil || seen[0].DashboardID != dash {
		t.Fatalf("proposer saw %+v", seen)
	}
	if res.Dashboard.ID != dash.String() || res.Dashboard.Abilities.Edit || !res.Dashboard.Abilities.Comment {
		t.Fatalf("dashboard %s with abilities %+v", res.Dashboard.ID, res.Dashboard.Abilities)
	}
	if got := r.latestNumber(t, dash); got != 1 {
		t.Fatalf("latest = %d, want no new version", got)
	}

	if _, err := save(commenterNoGrant, false); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("without propose_if_needed: err = %v, want forbidden", err)
	}
	if _, err := save(viewerNoGrant, true); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a viewer with propose_if_needed: err = %v, want forbidden", err)
	}
	if len(seen) != 1 {
		t.Errorf("the proposer ran %d times, want once", len(seen))
	}

	r.service.SetProposer(nil)
	if _, err := save(commenterNoGrant, true); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("with no proposer: err = %v, want forbidden", err)
	}
}

func saveBody(spec []byte, parent int, extra string) string {
	return `{"spec":` + string(spec) + `,"summary":"mine","parent_version":` + strconv.Itoa(parent) + extra + `}`
}

func TestTheVersionsRouteMergesOnlyWhenAskedTo(t *testing.T) {
	r, _ := newProbeRig(t)
	b := boards{rig: r}
	id := r.staleDashboard(t)
	path := "/dashboards/" + id.String() + "/versions"

	rec := b.serve(testUser, http.MethodPost, path, saveBody(twoPanels("y0", "m1"), 1, ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("a stale save without merge = %d %s, want 409", rec.Code, rec.Body)
	}
	got := decodeObject(t, rec.Body.Bytes())
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, []string{"code", "error", "latest_version"}) || got["code"] != "version_conflict" || got["latest_version"] != float64(2) {
		t.Fatalf("409 body = %s, want exactly error, code and latest_version", rec.Body)
	}

	rec = b.serve(testUser, http.MethodPost, path, saveBody(twoPanels("y0", "m1"), 1, `,"merge":"yes"`))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `merge must be \"auto\"`) {
		t.Fatalf("merge yes = %d %s, want 400", rec.Code, rec.Body)
	}

	rec = b.serve(testUser, http.MethodPost, path, saveBody(twoPanels("m0", "y1"), 1, `,"merge":"auto"`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("an overlap with merge auto = %d %s, want 409", rec.Code, rec.Body)
	}
	conflicts, _ := decodeObject(t, rec.Body.Bytes())["conflicts"].([]any)
	if len(conflicts) != 1 {
		t.Fatalf("409 body = %s, want one conflict", rec.Body)
	}
	first, _ := conflicts[0].(map[string]any)
	if first["kind"] != "panel" || first["id"] != "p1" || first["reason"] != "both_changed" {
		t.Fatalf("conflict = %v", first)
	}

	rec = b.serve(testUser, http.MethodPost, path, saveBody(twoPanels("y0", "m1"), 1, `,"merge":"auto"`))
	if rec.Code != http.StatusCreated || rec.Header().Get("Location") != versionLocation(id.String(), 3) {
		t.Fatalf("merge auto = %d %s, location %q, want 201 at version 3", rec.Code, rec.Body, rec.Header().Get("Location"))
	}
	merged, _ := decodeObject(t, rec.Body.Bytes())["merged_from"].(map[string]any)
	if merged["base_version"] != float64(1) || merged["onto_version"] != float64(2) {
		t.Fatalf("merged_from = %v, want 1 onto 2", merged)
	}
}

func TestTheVersionsRouteAnswersAViewerBeforeReadingTheBodyAndACommenterAfter(t *testing.T) {
	r, _ := newProbeRig(t)
	b := boards{rig: r}
	id := r.staleDashboard(t)
	path := "/dashboards/" + id.String() + "/versions"

	if rec := b.serve(viewerNoGrant, http.MethodPost, path, `{`); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), roleErrorCode) {
		t.Errorf("a viewer with a malformed body = %d %s, want 403 %s", rec.Code, rec.Body, roleErrorCode)
	}
	if rec := b.serve(commenterNoGrant, http.MethodPost, path, `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("a commenter with a malformed body = %d %s, want 400", rec.Code, rec.Body)
	}
	if rec := b.serve(commenterNoGrant, http.MethodPost, path, saveBody(twoPanels("y0", "x1"), 2, "")); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), roleErrorCode) {
		t.Errorf("a commenter with a valid body = %d %s, want 403 %s", rec.Code, rec.Body, roleErrorCode)
	}
}

func TestAnEditorInProposeModeNeverWritesAVersionDirectly(t *testing.T) {
	r := newRig(t, false)
	dash := r.createDashboard(t)
	proposed := 0
	r.service.SetProposer(func(context.Context, auth.Principal, ProposeInput) (ProposalRef, error) {
		proposed++
		return ProposalRef{ID: "0b0b0b0b-0b0b-4b0b-8b0b-0b0b0b0b0b0b", Revision: 1, Opened: true}, nil
	})
	ctx := grantmode.With(t.Context(), grantmode.Propose)
	save := func(propose bool) (SaveResult, error) {
		return r.service.Save(ctx, testUser, SaveInput{DashboardID: dash, Parent: 1, Spec: panelsSpec("t", "rps", "p99"), Summary: "idea", ProposeIfNeeded: propose})
	}

	if _, err := save(false); !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("without propose_if_needed: err = %v, want forbidden", err)
	}
	if got := r.latestNumber(t, dash); got != 1 || proposed != 0 {
		t.Fatalf("latest = %d, proposer ran %d times, want no version and no proposal", got, proposed)
	}
	res, err := save(true)
	if err != nil || res.Proposal == nil || res.Version.Number != 0 || proposed != 1 {
		t.Fatalf("proposal %+v, version %d, err %v, want a proposal and no version", res.Proposal, res.Version.Number, err)
	}
	if got := r.latestNumber(t, dash); got != 1 {
		t.Fatalf("latest = %d, want 1", got)
	}
	if _, err := r.service.Save(t.Context(), testUser, SaveInput{DashboardID: dash, Parent: 1, Spec: panelsSpec("t", "rps", "p99"), Summary: "direct"}); err != nil {
		t.Fatalf("the same editor outside propose mode: %v", err)
	}
}

func TestCheckProposalRunsTheQueriesTheProposalChanges(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: []panelDef{{id: "p0", query: "m0"}, {id: "p1", query: "m1"}}})
	parse := func(q0, q1 string) *spec.Dashboard {
		parsed, err := spec.Parse(twoPanels(q0, q1))
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	p.setFail("y1", true)

	if err := r.service.CheckProposal(t.Context(), testUser, id, 1, parse("m0", "x1"), false); err != nil {
		t.Fatalf("a proposal whose queries run: %v", err)
	}
	_ = wantInvalid(t, r.service.CheckProposal(t.Context(), testUser, id, 1, parse("m0", "y1"), false))
	_ = wantInvalid(t, r.service.CheckProposal(t.Context(), testUser, id, 1, parse("m0", "y1"), true))
	if err := r.service.CheckProposal(t.Context(), testUser, id, 9, parse("m0", "x1"), false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an unknown base: err = %v, want not found", err)
	}
	if got := r.latestNumber(t, id); got != 1 {
		t.Fatalf("latest = %d, want the check to save nothing", got)
	}
}

func TestAVersionNamesTheProposalItWasMergedFromOrNull(t *testing.T) {
	meta := VersionMeta{Number: 2, Author: Author{Kind: "user", ID: "u", Name: "Ada"}, MergedProposalID: "0b0b0b0b-0b0b-4b0b-8b0b-0b0b0b0b0b0b"}
	raw, err := json.Marshal(newVersionMetaJSON(meta))
	if err != nil || !strings.Contains(string(raw), `"merged_proposal_id":"0b0b0b0b-0b0b-4b0b-8b0b-0b0b0b0b0b0b"`) {
		t.Fatalf("merged version = %s, %v", raw, err)
	}
	meta.MergedProposalID = ""
	raw, err = json.Marshal(newVersionMetaJSON(meta))
	if err != nil || !strings.Contains(string(raw), `"merged_proposal_id":null`) {
		t.Fatalf("direct version = %s, %v", raw, err)
	}
}

func TestAProposeModeSaveThatRemovesPanelsOrVariablesIsRefusedAndWritesNothing(t *testing.T) {
	const variable = `{"name":"inst","type":"query","datasource":"prom","query":"label_values(up, instance)"}`
	r, _ := newProbeRig(t)
	dash := r.createSpec(t, specDef{variables: variable, panels: numberedPanels(3)})
	proposed := 0
	r.service.SetProposer(func(context.Context, auth.Principal, ProposeInput) (ProposalRef, error) {
		proposed++
		return ProposalRef{ID: "0b0b0b0b-0b0b-4b0b-8b0b-0b0b0b0b0b0b", Revision: 1, Opened: true}, nil
	})
	propose := grantmode.With(t.Context(), grantmode.Propose)
	save := func(ctx context.Context, by auth.Principal, def specDef) (SaveResult, error) {
		return r.service.Save(ctx, by, SaveInput{DashboardID: dash, Parent: 1, Spec: def.json(), Summary: "idea", ProposeIfNeeded: true})
	}

	for _, tc := range []struct {
		name    string
		def     specDef
		removed []spec.UnitChange
		message string
	}{
		{"a panel", specDef{variables: variable, panels: numberedPanels(2)}, []spec.UnitChange{{Kind: spec.UnitPanel, ID: "p2", Change: spec.ChangeRemoved}},
			`This connection is in propose mode, and a proposal cannot remove panels or variables; ask your person to remove panel "p2"`},
		{"a variable", specDef{panels: numberedPanels(3)}, []spec.UnitChange{{Kind: spec.UnitVariable, ID: "inst", Change: spec.ChangeRemoved}},
			`This connection is in propose mode, and a proposal cannot remove panels or variables; ask your person to remove variable "inst"`},
		{"a panel and a variable", specDef{panels: numberedPanels(1)}, []spec.UnitChange{
			{Kind: spec.UnitPanel, ID: "p1", Change: spec.ChangeRemoved},
			{Kind: spec.UnitPanel, ID: "p2", Change: spec.ChangeRemoved},
			{Kind: spec.UnitVariable, ID: "inst", Change: spec.ChangeRemoved},
		}, `This connection is in propose mode, and a proposal cannot remove panels or variables; ask your person to remove panel "p1", panel "p2", variable "inst"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := save(propose, testUser, tc.def)
			var refused *ProposalRequiredError
			if !errors.As(err, &refused) || !slices.Equal(refused.Removed, tc.removed) || refused.Error() != tc.message {
				t.Fatalf("err = %v (%+v), want proposal_required removing %+v with %q", err, refused, tc.removed, tc.message)
			}
		})
	}
	if got := r.latestNumber(t, dash); got != 1 || proposed != 0 {
		t.Fatalf("latest = %d, proposer ran %d times, want no version and no proposal", got, proposed)
	}

	changed := specDef{variables: variable, panels: []panelDef{{id: "p0", query: "changed"}, {id: "p1", query: "m1"}, {id: "p2", query: "m2"}}}
	if res, err := save(propose, testUser, changed); err != nil || res.Proposal == nil || proposed != 1 {
		t.Fatalf("a propose-mode change that removes nothing: proposal %+v, err %v, proposer calls %d", res.Proposal, err, proposed)
	}

	removing := specDef{panels: numberedPanels(2)}
	if res, err := save(t.Context(), commenterNoGrant, removing); err != nil || res.Proposal == nil || proposed != 2 {
		t.Fatalf("a commenter outside propose mode removing units: proposal %+v, err %v, proposer calls %d", res.Proposal, err, proposed)
	}
}

func TestAProposalRequiredFailureAnswers409WithTheRemovedUnits(t *testing.T) {
	r, _ := newProbeRig(t)
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	r.service.Fail(c, "save", &ProposalRequiredError{Removed: []spec.UnitChange{{Kind: spec.UnitPanel, ID: "p1", Change: spec.ChangeRemoved}}})

	var body struct {
		Error   string            `json:"error"`
		Code    string            `json:"code"`
		Removed []spec.UnitChange `json:"removed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict || body.Code != "proposal_required" || !strings.HasSuffix(body.Error, `ask your person to remove panel "p1"`) || len(body.Removed) != 1 || body.Removed[0].ID != "p1" {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
}
