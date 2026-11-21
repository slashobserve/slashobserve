package activity

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestAnAdminsOnlyFolderHidesItsActivityFromItsNonAdminCreator(t *testing.T) {
	r := newRig(t)
	dashboard := r.dashboardIn(t, foldertest.AdminsOnlyMadeBy(t, r.db, "Payroll", plain))
	seq := r.append(t, Entry{Type: TypeDashboardCreated, Owner: OnDashboard(dashboard)})

	creator, _, err := r.store.List(t.Context(), r.audience(t, plain), Filter{}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(seqsOf(creator), seq) {
		t.Fatalf("the folder's creator reads %v, want no row of the Admins-only folder", seqsOf(creator))
	}
	admins, _, err := r.store.List(t.Context(), r.audience(t, admin), Filter{}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(seqsOf(admins), seq) {
		t.Fatalf("an Admin reads %v, want %d", seqsOf(admins), seq)
	}
}

func TestAnAdminsOnlyFolderKeepsItsProposalsOutOfTheCreatorsInbox(t *testing.T) {
	r := newRig(t)
	r.inboxUser(t, mia)
	dashboard := r.dashboardIn(t, foldertest.AdminsOnlyMadeBy(t, r.db, "Payroll", mia))
	proposal := r.uuid(t, r.scalar(t, `INSERT INTO proposals (dashboard_id, title, author_kind, author_id, author_name) VALUES ($1::uuid, 'x', 'user', $2::uuid, 'Ola') RETURNING id::text`, dashboard.String(), ola.ID))
	r.append(t, Entry{Type: TypeProposalOpened, TargetID: proposal, Owner: OnDashboard(dashboard), Actor: ola})

	wantReasons(t, "the creator of the Admins-only folder", r.inbox(t, mia, ViewForYou))
	wantReasons(t, "everything for the creator", r.inbox(t, mia, ViewEverything))
	wantReasons(t, "an Admin", r.inbox(t, admin, ViewForYou), ReasonProposal)
}
