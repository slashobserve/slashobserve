package users

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (r *rig) personalFolder(owner pgtype.UUID) pgtype.UUID {
	r.t.Helper()
	var id pgtype.UUID
	if err := r.db.SQLDB(r.t).QueryRowContext(r.t.Context(), "SELECT id FROM folders WHERE personal AND owner_id = $1", owner).Scan(&id); err != nil {
		r.t.Fatal(err)
	}
	return id
}

func (r *rig) dashboardExists(id pgtype.UUID) bool {
	r.t.Helper()
	var exists bool
	if err := r.db.SQLDB(r.t).QueryRowContext(r.t.Context(), "SELECT EXISTS (SELECT 1 FROM dashboards WHERE id = $1)", id).Scan(&exists); err != nil {
		r.t.Fatal(err)
	}
	return exists
}

func TestRemovingADeactivatedPersonDeletesTheirPersonalItemsAndKeepsSharedWork(t *testing.T) {
	r := newRig(t)
	ada := r.addUser("ada", time.Now().Add(-time.Hour))
	r.makeAdmin(ada)
	bob := r.addUser("bob", time.Now())
	shared := r.addDashboard(bob, "shared")
	private := r.addDashboard(bob, "private")
	foldertest.Move(t, r.db, private, r.personalFolder(bob))
	admin := adminCaller(ada, "Ada")

	active := decodePerson(t, http.StatusOK, r.doJSON(http.MethodPatch, "/api/v1/users/"+bob.String(), `{"role":null}`, admin))
	if active.PersonalItems != nil {
		t.Fatalf("an active person's personal_items = %+v, want null", active.PersonalItems)
	}
	rec := r.do(http.MethodDelete, "/api/v1/users/"+bob.String(), admin)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"not_deactivated"`) {
		t.Fatalf("removing an active person = %d %s, want 409 not_deactivated", rec.Code, rec.Body)
	}

	p := decodePerson(t, http.StatusOK, r.do(http.MethodPost, "/api/v1/users/"+bob.String()+"/deactivate", admin))
	if p.PersonalItems == nil || *p.PersonalItems != (personalItemsJSON{Dashboards: 1}) {
		t.Fatalf("a deactivated person's personal_items = %+v, want one dashboard", p.PersonalItems)
	}

	if rec := r.do(http.MethodDelete, "/api/v1/users/"+bob.String(), admin); rec.Code != http.StatusNoContent {
		t.Fatalf("removing a deactivated person = %d %s, want 204", rec.Code, rec.Body)
	}
	if r.dashboardExists(private) {
		t.Error("the dashboard in their My Dashboards survived the removal")
	}
	if !r.dashboardExists(shared) {
		t.Error("the shared dashboard went with the person")
	}
	var author string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), "SELECT author_name FROM dashboard_versions WHERE dashboard_id = $1", shared).Scan(&author); err != nil || author != "shared" {
		t.Errorf("shared version author = %q, %v; want the stored name", author, err)
	}
	if rec := r.do(http.MethodDelete, "/api/v1/users/"+bob.String(), admin); rec.Code != http.StatusNotFound {
		t.Fatalf("removing twice = %d %s, want 404", rec.Code, rec.Body)
	}
}
