package findings_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
)

func TestAnAnswerToAQuestionStaysOnTheRunAndMakesNoObservation(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, pgtype.UUID{})

	f := r.save(t, editor, findings.SaveInput{AgentRequestID: request})
	if f.Saved() || f.ObservationID != "" || f.Folder != nil {
		t.Fatalf("the answer is %+v, want one kept on the run", f)
	}
	if n := r.count(t, `SELECT count(*) FROM observations`); n != 0 {
		t.Fatalf("%d observations after a plain answer", n)
	}
	if run, err := r.db.Queries().GetAgentRequestByID(t.Context(), request); err != nil || run.ObservationID.Valid {
		t.Fatalf("run observation %v, %v", run.ObservationID, err)
	}
	if n := r.count(t, `SELECT count(*) FROM activity WHERE type = 'finding.created'`); n != 0 {
		t.Fatalf("%d finding activity entries for an answer nobody filed", n)
	}
}

func TestAnUnsavedAnswerReachesOnlyTheRequesterAndAdmins(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, r.open)
	saved := r.save(t, editor, findings.SaveInput{AgentRequestID: request, DashboardID: r.open})

	cases := []struct {
		name string
		who  auth.Principal
		want bool
	}{
		{"the requester", editor, true},
		{"an Admin", admin, true},
		{"another Editor", otherEditor, false},
		{"a Viewer", viewer, false},
	}
	for _, c := range cases {
		if _, found, err := r.service.ForRequest(t.Context(), c.who, request); err != nil || found != c.want {
			t.Errorf("%s: found %v, err %v, want found %v", c.name, found, err, c.want)
		}
		got, getErr := r.service.Get(t.Context(), c.who, mustUUID(t, saved.ID))
		if (getErr == nil) != c.want {
			t.Errorf("%s: get err %v, want readable %v", c.name, getErr, c.want)
		}
		if c.want && (got.Abilities.Edit || !got.Abilities.View) {
			t.Errorf("%s: abilities %+v on an unsaved answer", c.name, got.Abilities)
		}
	}
}

func TestSavingAnAnswerAsAnObservationFilesItOnceInTheDashboardsFolder(t *testing.T) {
	r := newRig(t)
	folder := r.folder(t, "Payments")
	r.db.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, r.open, folder)
	request := r.request(t, editor, r.open)
	answer := r.save(t, editor, findings.SaveInput{AgentRequestID: request, DashboardID: r.open, Title: "Checkout errors by pod"})

	first, replayed, err := r.service.SaveAsObservation(t.Context(), editor, request)
	if err != nil || replayed || !first.Saved() || first.ID != answer.ID {
		t.Fatalf("first save: %+v replayed %v, %v", first, replayed, err)
	}
	if n := r.count(t, `SELECT count(*) FROM observations WHERE id = $1::uuid AND folder_id = $2 AND title = 'Checkout errors by pod'`, first.ObservationID, folder); n != 1 {
		t.Fatalf("observation %s is not in the dashboard's folder %s", first.ObservationID, folder)
	}
	if n := r.count(t, `SELECT count(*) FROM observation_blocks WHERE observation_id = $1::uuid AND kind = 'text' AND run_id = $2`, first.ObservationID, request); n != 1 {
		t.Fatalf("text blocks of the saved answer: %d", n)
	}
	run, err := r.db.Queries().GetAgentRequestByID(t.Context(), request)
	if err != nil || run.ObservationID.String() != first.ObservationID {
		t.Fatalf("run observation %v, want %s, %v", run.ObservationID, first.ObservationID, err)
	}

	again, replayed, err := r.service.SaveAsObservation(t.Context(), editor, request)
	if err != nil || !replayed || again.ObservationID != first.ObservationID {
		t.Fatalf("repeat: %+v replayed %v, %v", again, replayed, err)
	}
	if n := r.count(t, `SELECT count(*) FROM observations`); n != 1 {
		t.Fatalf("%d observations after a repeated save", n)
	}
}

func TestOnlyTheRequesterSavesAnAnswerAsAnObservation(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, pgtype.UUID{})
	r.save(t, editor, findings.SaveInput{AgentRequestID: request})

	if _, _, err := r.service.SaveAsObservation(t.Context(), admin, request); !errors.Is(err, findings.ErrRequesterOnly) {
		t.Errorf("an Admin: %v, want ErrRequesterOnly", err)
	}
	if _, _, err := r.service.SaveAsObservation(t.Context(), otherEditor, request); !errors.Is(err, findings.ErrNotFound) {
		t.Errorf("another Editor: %v, want ErrNotFound", err)
	}
	if _, _, err := r.service.SaveAsObservation(t.Context(), editor, r.request(t, editor, pgtype.UUID{})); !errors.Is(err, findings.ErrNotFound) {
		t.Errorf("a run without an answer: %v, want ErrNotFound", err)
	}
	if n := r.count(t, `SELECT count(*) FROM observations`); n != 0 {
		t.Fatalf("%d observations after refused saves", n)
	}
}

func TestAnUnsavedAnswerCannotBePinnedButCanBecomeADashboard(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, r.open)
	answer := r.save(t, editor, findings.SaveInput{AgentRequestID: request, DashboardID: r.open})

	if _, _, err := r.service.Pin(t.Context(), editor, mustUUID(t, answer.ID)); !errors.Is(err, findings.ErrNotSaved) {
		t.Fatalf("pin: %v, want ErrNotSaved", err)
	}
	if _, _, err := r.service.SaveAsObservation(t.Context(), editor, request); err != nil {
		t.Fatal(err)
	}
	if _, created, err := r.service.Pin(t.Context(), editor, mustUUID(t, answer.ID)); err != nil || !created {
		t.Fatalf("pin after saving: created %v, %v", created, err)
	}
}
