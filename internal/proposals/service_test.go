package proposals

import (
	"errors"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var (
	steady  = panel{id: "a", query: "up"}
	second  = panel{id: "b", query: "up"}
	changed = panel{id: "a", query: "rate(up[1m])"}
)

func TestACommenterOpensAProposalAndItWritesNoVersion(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)

	ref := r.propose(t, commenter, id, 1, changed, second)
	if !ref.Opened || ref.Revision != 1 {
		t.Fatalf("ref = %+v, want an opened proposal at revision 1", ref)
	}
	if got := r.latest(t, id); got != 1 {
		t.Fatalf("latest = %d, want 1: a proposal saves no version", got)
	}
	if got := r.activity(t); !slices.Equal(got, []string{"proposal.opened"}) {
		t.Fatalf("activity = %v", got)
	}
	data := r.activityData(t, "proposal.opened")
	if data["title"] != "proposed change" || data["revision"] != float64(1) || data["base_version"] != float64(1) {
		t.Fatalf("event data = %v", data)
	}

	p, err := r.service.Get(t.Context(), commenter, proposalID(t, ref))
	if err != nil {
		t.Fatal(err)
	}
	if p.State != StateOpen || p.Revision != 1 || p.BaseVersion != 1 || p.Behind || p.LatestVersion != 1 {
		t.Fatalf("proposal = %+v", p.Item)
	}
	if p.Author.ID != commenterID || p.Author.Name != "Bo" {
		t.Fatalf("author = %+v", p.Author)
	}
	wantActions := []Action{ActionUpdate, ActionDiscard}
	if !slices.Equal(p.AllowedActions, wantActions) {
		t.Fatalf("allowed actions for the author = %v, want %v", p.AllowedActions, wantActions)
	}
	if want := []spec.UnitChange{{Kind: spec.UnitPanel, ID: "a", Change: spec.ChangeChanged}}; !slices.Equal(p.Changes, want) {
		t.Fatalf("changes = %v, want %v", p.Changes, want)
	}
	if len(p.Conflicts) != 0 {
		t.Fatalf("conflicts = %v, want none", p.Conflicts)
	}
}

func TestAViewerCannotPropose(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)

	_, err := r.save(t.Context(), viewer, id, 1, changed)
	if !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden", err)
	}
	if got := r.activity(t); len(got) != 0 {
		t.Fatalf("activity = %v, want none", got)
	}
}

func TestACommenterWithoutProposeIfNeededIsRefused(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)

	_, err := r.dashboards.Save(t.Context(), commenter, dashboards.SaveInput{DashboardID: id, Parent: 1, Spec: specOf("t", changed), Summary: "mine"})
	if !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden", err)
	}
}

func TestAProposalOnAHiddenFolderIsNotFound(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)
	ref := r.propose(t, commenter, id, 1, changed)
	hidden := r.hiddenFolder(t)
	r.moveInto(t, id, hidden)

	_, err := r.service.Get(t.Context(), outsider, proposalID(t, ref))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestASecondSaveByTheSameAuthorAddsARevision(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)
	first := r.propose(t, commenter, id, 1, changed, second)

	again := panel{id: "b", query: "node_load1"}
	next := r.propose(t, commenter, id, 1, changed, again)
	if next.Opened || next.Revision != 2 || next.ID != first.ID {
		t.Fatalf("ref = %+v, want revision 2 of %s", next, first.ID)
	}
	if got := r.activity(t); !slices.Equal(got, []string{"proposal.opened", "proposal.updated"}) {
		t.Fatalf("activity = %v", got)
	}

	p, err := r.service.Get(t.Context(), commenter, proposalID(t, first))
	if err != nil {
		t.Fatal(err)
	}
	if p.Revision != 2 {
		t.Fatalf("revision = %d, want 2", p.Revision)
	}
	for _, want := range []panel{changed, again} {
		found, ok := p.Spec.FindPanel(want.id)
		if !ok || found.Panel.Queries[0].Query != want.query {
			t.Fatalf("panel %s of the head = %+v, want query %q", want.id, found, want.query)
		}
	}
	page, err := r.service.List(t.Context(), commenter, ListQuery{DashboardID: id})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list = %+v, %v, want one open proposal", page, err)
	}
}

func TestAnotherCommenterOpensASeparateProposal(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)
	first := r.propose(t, commenter, id, 1, changed)
	other := r.propose(t, otherCommenter, id, 1, panel{id: "a", query: "node_load5"})
	if !other.Opened || other.ID == first.ID {
		t.Fatalf("ref = %+v, want a new proposal", other)
	}
	page, err := r.service.List(t.Context(), editor, ListQuery{DashboardID: id})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("list = %+v, %v, want two", page, err)
	}
}

func TestConcurrentSavesByOneAuthorFoldIntoOneProposal(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)

	const saves = 12
	var wg sync.WaitGroup
	refs := make([]dashboards.ProposalRef, saves)
	errs := make([]error, saves)
	for i := range refs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := r.save(t.Context(), commenter, id, 1, changed, panel{id: "b", query: "q" + strconv.Itoa(i)})
			errs[i] = err
			if res.Proposal != nil {
				refs[i] = *res.Proposal
			}
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	page, err := r.service.List(t.Context(), commenter, ListQuery{DashboardID: id})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list = %+v, %v, want one proposal", page, err)
	}
	opened := 0
	for _, ref := range refs {
		if ref.Opened {
			opened++
		}
	}
	if opened != 1 || page.Items[0].Revision != saves {
		t.Fatalf("opened = %d, head revision = %d, want 1 and %d", opened, page.Items[0].Revision, saves)
	}
}

func TestConcurrentSavesByTwoAuthorsOpenOneProposalEach(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)

	authors := []auth.Principal{commenter, otherCommenter}
	const savesEach = 4
	var wg sync.WaitGroup
	errs := make([]error, len(authors)*savesEach)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = r.save(t.Context(), authors[i%len(authors)], id, 1, changed, panel{id: "b", query: "q" + strconv.Itoa(i)})
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	page, err := r.service.List(t.Context(), editor, ListQuery{DashboardID: id})
	if err != nil || len(page.Items) != len(authors) {
		t.Fatalf("list = %+v, %v, want one proposal per author", page, err)
	}
	for _, item := range page.Items {
		if item.Revision != savesEach {
			t.Fatalf("proposal %s revision = %d, want %d", item.ID, item.Revision, savesEach)
		}
	}
}

func TestAProposalBehindTheDashboardReportsItsConflicts(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)
	ref := r.propose(t, commenter, id, 1, changed, second)
	r.addVersion(t, id, 1, panel{id: "a", query: "node_load15"}, second)

	p, err := r.service.Get(t.Context(), editor, proposalID(t, ref))
	if err != nil {
		t.Fatal(err)
	}
	if !p.Behind || p.LatestVersion != 2 || p.BaseVersion != 1 {
		t.Fatalf("proposal = %+v", p.Item)
	}
	if len(p.Conflicts) != 1 || p.Conflicts[0].ID != "a" || p.Conflicts[0].Reason != spec.ReasonBothChanged {
		t.Fatalf("conflicts = %v, want panel a changed by both", p.Conflicts)
	}
	if want := []spec.UnitChange{{Kind: spec.UnitPanel, ID: "a", Change: spec.ChangeChanged}}; !slices.Equal(p.Changes, want) {
		t.Fatalf("changes = %v, want %v", p.Changes, want)
	}
	if !slices.Equal(p.AllowedActions, []Action{ActionMerge, ActionDiscard}) {
		t.Fatalf("allowed actions for an editor = %v", p.AllowedActions)
	}
}

func TestAnEditorMergesAProposalIntoTheNextVersion(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)
	ref := r.propose(t, commenter, id, 1, changed, second)
	r.addVersion(t, id, 1, steady, panel{id: "b", query: "node_load1"})

	merged, err := r.service.Merge(t.Context(), editor, proposalID(t, ref), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if merged.State != StateMerged || merged.MergedVersion == nil || *merged.MergedVersion != 3 || merged.ClosedBy == nil || merged.ClosedBy.ID != editorID {
		t.Fatalf("merged = %+v", merged.Item)
	}
	if len(merged.AllowedActions) != 0 {
		t.Fatalf("allowed actions = %v, want none once merged", merged.AllowedActions)
	}
	if got := r.latest(t, id); got != 3 {
		t.Fatalf("latest = %d, want 3", got)
	}
	_, latest, err := r.dashboards.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if latest.MergedProposalID != ref.ID {
		t.Fatalf("merged_proposal_id = %q, want %q", latest.MergedProposalID, ref.ID)
	}
	for _, want := range []panel{changed, {id: "b", query: "node_load1"}} {
		found, ok := latest.Spec.FindPanel(want.id)
		if !ok || found.Panel.Queries[0].Query != want.query {
			t.Fatalf("panel %s of version 3 = %+v, want query %q", want.id, found, want.query)
		}
	}
	if got := r.activity(t); !slices.Equal(got, []string{"proposal.opened", "proposal.merged"}) {
		t.Fatalf("activity = %v", got)
	}
	if data := r.activityData(t, "proposal.merged"); data["version"] != float64(3) {
		t.Fatalf("merged event data = %v", data)
	}
}

func TestMergeNamesTheRevisionTheCallerRead(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)
	ref := r.propose(t, commenter, id, 1, changed, second)
	r.propose(t, commenter, id, 1, changed, panel{id: "b", query: "node_load1"})

	_, err := r.service.Merge(t.Context(), editor, proposalID(t, ref), 1, nil)
	var stale *ChangedError
	if !errors.As(err, &stale) || stale.Revision != 2 {
		t.Fatalf("err = %v, want a changed error at revision 2", err)
	}
	if got := r.latest(t, id); got != 1 {
		t.Fatalf("latest = %d, want 1", got)
	}
}

func TestMergingAClosedProposalIsAnInvalidTransition(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)
	ref := r.propose(t, commenter, id, 1, changed)
	if _, err := r.service.Merge(t.Context(), editor, proposalID(t, ref), 1, nil); err != nil {
		t.Fatal(err)
	}

	_, err := r.service.Merge(t.Context(), editor, proposalID(t, ref), 1, nil)
	var refusal *lifecycle.Refusal
	if !errors.Is(err, lifecycle.ErrInvalidTransition) || !errors.As(err, &refusal) || refusal.State != string(StateMerged) {
		t.Fatalf("err = %v, want an invalid transition from merged", err)
	}
	if _, err := r.service.Discard(t.Context(), editor, proposalID(t, ref)); !errors.Is(err, lifecycle.ErrInvalidTransition) {
		t.Fatalf("discard err = %v, want an invalid transition", err)
	}
}

func TestMergeNeedsAChoiceForEachConflict(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)
	ref := r.propose(t, commenter, id, 1, changed, second)
	r.addVersion(t, id, 1, panel{id: "a", query: "node_load15"}, second)

	_, err := r.service.Merge(t.Context(), editor, proposalID(t, ref), 1, nil)
	var conflict *dashboards.ConflictError
	if !errors.As(err, &conflict) || conflict.Latest != 2 || len(conflict.Conflicts) != 1 {
		t.Fatalf("err = %v, want a conflict at version 2", err)
	}
	p, err := r.service.Get(t.Context(), editor, proposalID(t, ref))
	if err != nil || p.State != StateOpen {
		t.Fatalf("proposal after a refused merge = %+v, %v, want it open", p.Item, err)
	}

	choice := spec.Choice{Kind: spec.UnitPanel, ID: "a", Take: spec.SideTheirs}
	merged, err := r.service.Merge(t.Context(), editor, proposalID(t, ref), 1, []spec.Choice{choice})
	if err != nil {
		t.Fatal(err)
	}
	if merged.State != StateMerged || merged.MergedVersion == nil || *merged.MergedVersion != 3 {
		t.Fatalf("merged = %+v", merged.Item)
	}
	_, latest, err := r.dashboards.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if found, _ := latest.Spec.FindPanel("a"); found.Panel.Queries[0].Query != changed.query {
		t.Fatalf("panel a = %q, want the proposal's", found.Panel.Queries[0].Query)
	}
}

func TestChoosingOursKeepsTheDashboardsPanel(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)
	ref := r.propose(t, commenter, id, 1, changed, panel{id: "b", query: "node_load1"})
	r.addVersion(t, id, 1, panel{id: "a", query: "node_load15"}, second)

	choice := spec.Choice{Kind: spec.UnitPanel, ID: "a", Take: spec.SideOurs}
	if _, err := r.service.Merge(t.Context(), editor, proposalID(t, ref), 1, []spec.Choice{choice}); err != nil {
		t.Fatal(err)
	}
	_, latest, err := r.dashboards.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := latest.Spec.FindPanel("a")
	b, _ := latest.Spec.FindPanel("b")
	if a.Panel.Queries[0].Query != "node_load15" || b.Panel.Queries[0].Query != "node_load1" {
		t.Fatalf("a = %q, b = %q, want the dashboard's a and the proposal's b", a.Panel.Queries[0].Query, b.Panel.Queries[0].Query)
	}
}

func TestMergeRefusesAChoiceThatNamesNoUnit(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)
	ref := r.propose(t, commenter, id, 1, changed, second)
	r.addVersion(t, id, 1, steady, panel{id: "b", query: "node_load1"})

	choice := spec.Choice{Kind: spec.UnitPanel, ID: "nope", Take: spec.SideTheirs}
	_, err := r.service.Merge(t.Context(), editor, proposalID(t, ref), 1, []spec.Choice{choice})
	var invalid *dashboards.InvalidInputError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want an invalid input error", err)
	}
}

func TestAProposalAlreadyInTheDashboardClosesWithNoVersion(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)
	ref := r.propose(t, commenter, id, 1, changed, second)
	r.addVersion(t, id, 1, changed, second)

	merged, err := r.service.Merge(t.Context(), editor, proposalID(t, ref), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if merged.State != StateMerged || merged.MergedVersion != nil {
		t.Fatalf("merged = %+v, want merged with no version", merged.Item)
	}
	if got := r.latest(t, id); got != 2 {
		t.Fatalf("latest = %d, want 2", got)
	}
	if data := r.activityData(t, "proposal.merged"); data["version"] != nil {
		t.Fatalf("merged event data = %v, want a null version", data)
	}
}

func TestOnlyAnEditorOutsideProposeModeMerges(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)
	ref := r.propose(t, commenter, id, 1, changed)

	if _, err := r.service.Merge(t.Context(), commenter, proposalID(t, ref), 1, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("commenter err = %v, want forbidden", err)
	}
	proposing := grantmode.With(t.Context(), grantmode.Propose)
	if _, err := r.service.Merge(proposing, editor, proposalID(t, ref), 1, nil); !errors.Is(err, ErrProposeMode) {
		t.Fatalf("propose mode err = %v, want the propose mode error", err)
	}
	if got := r.latest(t, id); got != 1 {
		t.Fatalf("latest = %d, want 1", got)
	}
}

func TestAnEditorInProposeModeProposesInsteadOfSaving(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)
	proposing := grantmode.With(t.Context(), grantmode.Propose)

	if _, err := r.dashboards.Save(proposing, editor, dashboards.SaveInput{DashboardID: id, Parent: 1, Spec: specOf("t", changed), Summary: "mine"}); !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden without propose_if_needed", err)
	}
	res, err := r.save(proposing, editor, id, 1, changed)
	if err != nil || res.Proposal == nil || !res.Proposal.Opened {
		t.Fatalf("res = %+v, err = %v, want an opened proposal", res, err)
	}
	if got := r.latest(t, id); got != 1 {
		t.Fatalf("latest = %d, want 1", got)
	}
}

func TestTheAuthorDiscardsButAnotherCommenterCannot(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)
	ref := r.propose(t, commenter, id, 1, changed)

	if _, err := r.service.Discard(t.Context(), otherCommenter, proposalID(t, ref)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other commenter err = %v, want forbidden", err)
	}
	discarded, err := r.service.Discard(t.Context(), commenter, proposalID(t, ref))
	if err != nil {
		t.Fatal(err)
	}
	if discarded.State != StateDiscarded || discarded.ClosedBy == nil || discarded.ClosedBy.ID != commenterID || discarded.MergedVersion != nil {
		t.Fatalf("discarded = %+v", discarded.Item)
	}
	if got := r.activity(t); !slices.Equal(got, []string{"proposal.opened", "proposal.discarded"}) {
		t.Fatalf("activity = %v", got)
	}
	again := r.propose(t, commenter, id, 1, changed)
	if !again.Opened || again.ID == ref.ID {
		t.Fatalf("ref = %+v, want a new proposal once the first is closed", again)
	}
}

func TestAnEditorDiscardsSomeoneElsesProposal(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)
	ref := r.propose(t, commenter, id, 1, changed)

	discarded, err := r.service.Discard(t.Context(), editor, proposalID(t, ref))
	if err != nil || discarded.State != StateDiscarded {
		t.Fatalf("discarded = %+v, err = %v", discarded.Item, err)
	}
}

func TestListFiltersByState(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)
	closed := r.propose(t, commenter, id, 1, changed)
	if _, err := r.service.Discard(t.Context(), commenter, proposalID(t, closed)); err != nil {
		t.Fatal(err)
	}
	open := r.propose(t, otherCommenter, id, 1, changed)

	counts := map[string]int{}
	for _, state := range []string{"", "open", "discarded", "merged", StateFilterAll} {
		page, err := r.service.List(t.Context(), editor, ListQuery{DashboardID: id, State: state})
		if err != nil {
			t.Fatal(err)
		}
		counts[state] = len(page.Items)
	}
	if counts[""] != 1 || counts["open"] != 1 || counts["discarded"] != 1 || counts["merged"] != 0 || counts[StateFilterAll] != 2 {
		t.Fatalf("counts = %v", counts)
	}
	page, err := r.service.List(t.Context(), editor, ListQuery{DashboardID: id, State: StateFilterAll, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" || page.Items[0].ID != open.ID {
		t.Fatalf("first page = %+v, %v, want the newest proposal and a cursor", page, err)
	}
	next, err := r.service.List(t.Context(), editor, ListQuery{DashboardID: id, State: StateFilterAll, Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != closed.ID || next.NextCursor != "" {
		t.Fatalf("second page = %+v, %v", next, err)
	}
	if _, err := r.service.List(t.Context(), editor, ListQuery{DashboardID: id, State: "weird"}); err == nil {
		t.Fatal("an unknown state was accepted")
	}
}

func TestDeletingTheDashboardDeletesItsProposals(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)
	ref := r.propose(t, commenter, id, 1, changed)

	if err := r.dashboards.Delete(t.Context(), editor, id); err != nil {
		t.Fatal(err)
	}
	if _, err := r.service.Get(t.Context(), editor, proposalID(t, ref)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestAProposalOnAMissingBaseVersionIsRefused(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)

	_, err := r.save(t.Context(), commenter, id, 9, changed)
	var invalid *dashboards.InvalidInputError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want an invalid input error", err)
	}
}

func TestAProposalWhoseQueriesFailIsRefusedAndOpensNothing(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady)
	broken := specOf("t", panel{id: "a", query: "up"})
	broken = append(broken[:len(broken)-2], []byte(`,{"id":"x","type":"timeseries","title":"x","gridPos":{"x":0,"y":8,"w":12,"h":8},"datasource":"missing","queries":[{"query":"up"}]}]}`)...)

	_, err := r.dashboards.Save(t.Context(), commenter, dashboards.SaveInput{DashboardID: id, Parent: 1, Spec: broken, Summary: "broken", ProposeIfNeeded: true})
	var invalid *dashboards.InvalidSpecError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want an invalid spec error", err)
	}
	if got := r.activity(t); len(got) != 0 {
		t.Fatalf("activity = %v, want none", got)
	}
}

func TestTwoMergesAtOnceSaveOneVersion(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)
	ref := r.propose(t, commenter, id, 1, changed, second)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = r.service.Merge(t.Context(), editor, proposalID(t, ref), 1, nil)
		}()
	}
	wg.Wait()
	succeeded := 0
	for _, err := range errs {
		var stale *ChangedError
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, lifecycle.ErrInvalidTransition), errors.As(err, &stale):
		default:
			t.Fatalf("unexpected error %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("errors = %v, want exactly one merge to succeed", errs)
	}
	if got := r.latest(t, id); got != 2 {
		t.Fatalf("latest = %d, want 2", got)
	}
	if got := r.activity(t); !slices.Equal(got, []string{"proposal.opened", "proposal.merged"}) {
		t.Fatalf("activity = %v", got)
	}
}

func TestAMergeAndADiscardAtOnceLeaveOneOutcome(t *testing.T) {
	r := newRig(t)
	id := r.createDashboard(t, steady, second)
	ref := r.propose(t, commenter, id, 1, changed, second)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = r.service.Merge(t.Context(), editor, proposalID(t, ref), 1, nil)
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = r.service.Discard(t.Context(), commenter, proposalID(t, ref))
	}()
	wg.Wait()
	p, err := r.service.Get(t.Context(), editor, proposalID(t, ref))
	if err != nil {
		t.Fatal(err)
	}
	switch p.State {
	case StateMerged:
		if errs[0] != nil || r.latest(t, id) != 2 {
			t.Fatalf("merged but errs = %v, latest = %d", errs, r.latest(t, id))
		}
	case StateDiscarded:
		if errs[1] != nil || r.latest(t, id) != 1 {
			t.Fatalf("discarded but errs = %v, latest = %d", errs, r.latest(t, id))
		}
	default:
		t.Fatalf("state = %s, want a closed proposal", p.State)
	}
	if got := r.activity(t); len(got) != 2 {
		t.Fatalf("activity = %v, want opened and one close", got)
	}
}
