package webhooks

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestAnAdminOwnersWebhookSendsTheEventsOfAnAdminsOnlyFolderAnEditorMade(t *testing.T) {
	r := newRig(t)
	maker := auth.Principal{Kind: auth.KindUser, ID: "99999999-9999-4999-8999-999999999999", Name: "Eve", Role: auth.RoleEditor}
	folder := foldertest.AdminsOnlyMadeBy(t, r.db, "Payroll", maker)
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `
		INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via, folder_id)
		VALUES ('Salaries', 1, 'user', $1::uuid, 'Eve', 'direct', $2::uuid)
		RETURNING id::text`, maker.ID, folder.String()).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	r.create(t, activity.TypeDashboardCreated)
	seq := r.appendFor(t, activity.TypeDashboardCreated, r.uuid(t, raw))

	r.pass(t)

	if got := seqsSent(t, r.sender.all()); !slices.Equal(got, []int64{seq}) {
		t.Fatalf("sent %v, want the Admins-only folder's event %d", got, seq)
	}
}
