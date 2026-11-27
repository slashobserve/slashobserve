package observations_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/placement"
)

func (r rig) restrictedAnswerIn(t *testing.T, o observations.Observation) {
	t.Helper()
	b := r.addText(t, admin, o, "Root signed in 41 times")
	r.db.Exec(t, `WITH run AS (INSERT INTO agent_requests (intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, read_restricted)
VALUES ('ask', 'investigate', 'who signed in', 'user', $2::uuid, 'Ada', true) RETURNING id)
UPDATE observation_blocks SET run_id = (SELECT id FROM run) WHERE id = $1::uuid`, b.ID, admin.ID)
}

func (r rig) annotationCopied(t *testing.T, id string) bool {
	t.Helper()
	var copied bool
	if err := r.db.SQLDB(t).QueryRow(`SELECT copied_restricted_answer FROM annotations WHERE id = $1::uuid`, id).Scan(&copied); err != nil {
		t.Fatal(err)
	}
	return copied
}

func TestAPinOfAnObservationHoldingARestrictedAnswerGoesOnlyOnADashboardOnlyAdminsReadAndHoldsItThere(t *testing.T) {
	r := newRig(t)
	o := r.create(t, admin, foldertest.AdminsOnly(t, r.db, "Audit"))
	r.restrictedAnswerIn(t, o)
	id := mustUUID(t, o.ID)
	shared := r.dashboard(t)

	if _, _, err := r.service.Pin(reading(t.Context()), admin, id, shared); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("pin onto a dashboard in General: err = %v, want the restricted answer refusal", err)
	}
	locked := r.dashboard(t)
	r.db.Exec(t, `UPDATE dashboards SET folder_id = $2::uuid WHERE id = $1::uuid`, locked, foldertest.AdminsOnly(t, r.db, "Audit boards"))
	pin, created, err := r.service.Pin(reading(t.Context()), admin, id, locked)
	if err != nil || !created {
		t.Fatalf("pin onto a dashboard only Admins read: created %v, %v", created, err)
	}
	if !r.annotationCopied(t, pin.AnnotationID) {
		t.Fatal("the pin does not hold the answer it copied")
	}
}

func TestAPinOfAnObservationHoldingNoRestrictedAnswerHoldsNothing(t *testing.T) {
	r := newRig(t)
	o := r.create(t, admin, foldertest.AdminsOnly(t, r.db, "Audit"))
	pin, _, err := r.service.Pin(reading(t.Context()), admin, mustUUID(t, o.ID), r.dashboard(t))
	if err != nil {
		t.Fatalf("pin onto a dashboard in General: %v", err)
	}
	if r.annotationCopied(t, pin.AnnotationID) {
		t.Fatal("a pin of an observation with no restricted answer holds one")
	}
}

func TestADashboardMadeFromAnObservationIsACopyOfThatObservation(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	r.addPanel(t, editor, o)
	id := mustUUID(t, o.ID)
	if _, _, _, err := r.service.MakeDashboard(reading(t.Context()), editor, id, pgtype.UUID{}, false); err != nil {
		t.Fatal(err)
	}
	want := placement.Copied{Kind: folders.ContentObservation, ID: id}
	if len(r.made.from) != 1 || r.made.from[0] != want {
		t.Fatalf("made from %+v, want a copy of the observation %+v", r.made.from, want)
	}
}

func (r rig) runWorkingOn(t *testing.T, by auth.Principal, o observations.Observation) string {
	t.Helper()
	var run string
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO agent_requests (intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name)
VALUES ('ask', 'investigate', 'why', 'user', $1::uuid, $2) RETURNING id::text`, by.ID, by.Name).Scan(&run); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE agent_requests SET subject_observation_id = $2::uuid, status = 'running', state = 'running' WHERE id = $1::uuid`, run, o.ID)
	return run
}

func TestAnObservationReadSaysWhatKeepsItWhereOnlyAdminsRead(t *testing.T) {
	r := newRig(t)
	confinement := func(o observations.Observation) folders.Confinement {
		t.Helper()
		read, err := r.service.Get(reading(t.Context()), admin, mustUUID(t, o.ID))
		if err != nil {
			t.Fatal(err)
		}
		return read.Confinement
	}
	o := r.create(t, admin, foldertest.AdminsOnly(t, r.db, "Audit"))
	if got := confinement(o); got != folders.NotConfined {
		t.Fatalf("a new observation: confinement = %q, want none", got)
	}

	run := r.runWorkingOn(t, admin, o)
	if got := confinement(o); got != folders.ConfinedRunInFlight {
		t.Fatalf("while a run about it is working: confinement = %q, want run_in_flight", got)
	}
	r.db.Exec(t, `UPDATE agent_requests SET status = 'succeeded', state = 'succeeded', finished_at = now() WHERE id = $1::uuid`, run)
	if got := confinement(o); got != folders.NotConfined {
		t.Fatalf("after the run ended: confinement = %q, want none", got)
	}

	r.restrictedAnswerIn(t, o)
	if got := confinement(o); got != folders.ConfinedPermanent {
		t.Fatalf("holding an answer that read Admin-only data: confinement = %q, want permanent", got)
	}

	shared := r.create(t, editor, r.shared)
	r.runWorkingOn(t, editor, shared)
	if got := confinement(shared); got != folders.NotConfined {
		t.Fatalf("a run in flight in a shared folder: confinement = %q, want none", got)
	}
}
