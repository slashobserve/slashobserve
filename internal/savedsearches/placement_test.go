package savedsearches_test

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
)

var admin = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Ada", Role: auth.RoleAdmin}

func newAdminsOnlyRig(t *testing.T) rig {
	t.Helper()
	r := newRig(t)
	r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', 'ada', 'ada@example.com', 'Ada')`, admin.ID)
	registry := newRegistry(t)
	if err := registry.AddAdminsOnly(datasource.Instance{UID: "audit", Name: "Audit", Datasource: fakeSource{kind: "loki"}}); err != nil {
		t.Fatal(err)
	}
	r.service = savedsearches.New(r.db.DB, folders.NewAccess(r.db.DB), registry, nil, slog.New(slog.DiscardHandler))
	return r
}

func auditSearch(title string) savedsearches.CreateInput {
	in := logsSearch(title)
	in.DatasourceUID = "audit"
	return in
}

func refusedAsRestricted(t *testing.T, err error) {
	t.Helper()
	var refusal *folders.RestrictedContentError
	if !errors.As(err, &refusal) || refusal.Datasource != "audit" {
		t.Fatalf("err = %v, want a restricted content refusal naming audit", err)
	}
}

func TestASavedSearchOnAnAdminsOnlyDatasourceStaysInAnAdminsOnlyFolder(t *testing.T) {
	r := newAdminsOnlyRig(t)
	ctx := reading(t.Context())
	open, locked := foldertest.Open(t, r.db, "Team"), foldertest.AdminsOnly(t, r.db, "Payroll")

	in := auditSearch("Payroll errors")
	in.FolderID = open
	_, err := r.service.Create(ctx, admin, in)
	refusedAsRestricted(t, err)

	in.FolderID = locked
	saved := r.save(t, admin, in)
	id := mustUUID(t, saved.ID)

	_, err = r.service.Update(ctx, admin, id, savedsearches.UpdateInput{FolderID: open})
	refusedAsRestricted(t, err)
	query := `{app="payroll"}`
	if _, err := r.service.Update(ctx, admin, id, savedsearches.UpdateInput{Query: &query}); err != nil {
		t.Fatalf("edit inside the admins only folder = %v", err)
	}

	creator := auth.Principal{Kind: auth.KindUser, ID: admin.ID, Name: "Ada", Role: auth.RoleEditor}
	listed, _, err := r.service.List(ctx, creator, savedsearches.ListFilter{}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range listed {
		if s.ID == saved.ID {
			t.Fatalf("a non-Admin listed the saved search and its query %+v", s)
		}
	}
}
