package dashboards

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/placement"
)

func (r *rig) copiedRestricted(t *testing.T, id string) bool {
	t.Helper()
	var copied bool
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT copied_restricted_answer FROM dashboards WHERE id = $1::uuid`, id).Scan(&copied); err != nil {
		t.Fatal(err)
	}
	return copied
}

func TestADuplicateOfADashboardHoldingARestrictedAnswerGoesOnlyWhereOnlyAdminsReadAndHoldsItThere(t *testing.T) {
	r, _ := newProbeRig(t)
	ada := r.addPerson(t, "ada")
	ada.principal.Role = auth.RoleAdmin
	locked := r.folder(t, "Audit", true)
	source, _, err := r.service.Create(reading(t.Context()), ada.principal, locked, panelsSpec("Sign-ins", "p1"), "")
	if err != nil {
		t.Fatal(err)
	}
	request := r.runningRequestFor(t, ada)
	r.markReadRestricted(t, request)
	r.db.Exec(t, `UPDATE dashboard_versions SET agent_request_id = $2 WHERE dashboard_id = $1::uuid`, source.ID, request)
	id := mustUUID(t, source.ID)

	for name, folder := range map[string]pgtype.UUID{"the General folder": r.generalFolder(t), "My Dashboards": {}} {
		if _, _, err := r.service.Duplicate(reading(t.Context()), ada.principal, id, folder, "Copy"); !errors.Is(err, folders.ErrRestrictedAnswer) {
			t.Fatalf("duplicate into %s: err = %v, want the restricted answer refusal", name, err)
		}
	}
	copied, _, err := r.service.Duplicate(reading(t.Context()), ada.principal, id, r.folder(t, "Audit copies", true), "Copy")
	if err != nil {
		t.Fatalf("duplicate into another folder only Admins read: %v", err)
	}
	if !r.copiedRestricted(t, copied.ID) {
		t.Fatal("the copy does not hold the answer it copied")
	}
	if _, _, err := r.service.Move(reading(t.Context()), ada.principal, mustUUID(t, copied.ID), r.generalFolder(t)); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("moving the copy to General: err = %v, want the restricted answer refusal", err)
	}
}

func TestADuplicateOfADashboardHoldingNoRestrictedAnswerGoesAnywhere(t *testing.T) {
	r, _ := newProbeRig(t)
	ada := r.addPerson(t, "ada")
	ada.principal.Role = auth.RoleAdmin
	source, _, err := r.service.Create(reading(t.Context()), ada.principal, r.folder(t, "Audit", true), panelsSpec("Sign-ins", "p1"), "")
	if err != nil {
		t.Fatal(err)
	}
	copied, _, err := r.service.Duplicate(reading(t.Context()), ada.principal, mustUUID(t, source.ID), r.generalFolder(t), "Copy")
	if err != nil {
		t.Fatalf("duplicate into General: %v", err)
	}
	if r.copiedRestricted(t, copied.ID) {
		t.Fatal("a copy of a dashboard with no restricted answer holds one")
	}
}

func TestADashboardMadeFromAnObservationHoldingARestrictedAnswerIsRefusedWhereOthersRead(t *testing.T) {
	r, _ := newProbeRig(t)
	ada := r.addPerson(t, "ada")
	ada.principal.Role = auth.RoleAdmin
	var observation pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `WITH o AS (INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name) VALUES ($1, 'Sign-ins', 'user', gen_random_uuid(), 'Ada') RETURNING id),
run AS (INSERT INTO agent_requests (intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, read_restricted) VALUES ('ask', 'investigate', 'who signed in', 'user', gen_random_uuid(), 'Ada', true) RETURNING id)
INSERT INTO observation_blocks (observation_id, kind, position, body, created_by_kind, created_by_id, created_by_name, run_id)
SELECT o.id, 'text', 0, '{"markdown":"Root signed in 41 times"}', 'user', gen_random_uuid(), 'Ada', run.id FROM o, run RETURNING observation_id`, r.folder(t, "Audit", true)).Scan(&observation); err != nil {
		t.Fatal(err)
	}
	from := placement.Copied{Kind: folders.ContentObservation, ID: observation}
	if _, _, err := r.service.CreateCopy(reading(t.Context()), ada.principal, panelsSpec("Sign-ins", "p1"), "", from); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("a copy into the default folder: err = %v, want the restricted answer refusal", err)
	}
	r.db.Exec(t, `UPDATE observation_blocks SET run_id = NULL WHERE observation_id = $1`, observation)
	made, _, err := r.service.CreateCopy(reading(t.Context()), ada.principal, panelsSpec("Sign-ins", "p1"), "", from)
	if err != nil {
		t.Fatalf("a copy of an observation with no restricted answer: %v", err)
	}
	if r.copiedRestricted(t, made.ID) {
		t.Fatal("a copy of an observation with no restricted answer holds one")
	}
}
