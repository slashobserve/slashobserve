package findings_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/observations"
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
	if run, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), request); err != nil || run.ObservationID.Valid {
		t.Fatalf("run observation %v, %v", run.ObservationID, err)
	}
	if n := r.count(t, `SELECT count(*) FROM activity WHERE type = 'finding.created'`); n != 0 {
		t.Fatalf("%d finding activity entries for an answer nobody filed", n)
	}
}

func TestAnAnswerKeepsItsSuggestedChangeAndObserveWorthyFlag(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, r.open)
	saved := r.save(t, editor, findings.SaveInput{AgentRequestID: request, DashboardID: r.open, SuggestedChange: "Split the error panel by pod.", ObserveWorthy: true})
	got, found, err := r.service.ForRequest(reading(t.Context()), editor, request)
	if err != nil || !found || got.ID != saved.ID {
		t.Fatalf("read back: %+v found %v, %v", got, found, err)
	}
	body := findings.JSONOf(got, "")
	if body.SuggestedChange == nil || *body.SuggestedChange != "Split the error panel by pod." || !body.ObserveWorthy {
		t.Fatalf("answer suggested_change %v observe_worthy %v", body.SuggestedChange, body.ObserveWorthy)
	}
	plain := findings.JSONOf(r.save(t, editor, findings.SaveInput{AgentRequestID: r.request(t, editor, r.open), DashboardID: r.open}), "")
	if plain.SuggestedChange != nil || plain.ObserveWorthy {
		t.Fatalf("an answer with neither: suggested_change %v observe_worthy %v", plain.SuggestedChange, plain.ObserveWorthy)
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
		if _, found, err := r.service.ForRequest(reading(t.Context()), c.who, request); err != nil || found != c.want {
			t.Errorf("%s: found %v, err %v, want found %v", c.name, found, err, c.want)
		}
		got, getErr := r.service.Get(reading(t.Context()), c.who, mustUUID(t, saved.ID))
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

	first, replayed, err := r.service.SaveAsObservation(reading(t.Context()), editor, request)
	if err != nil || replayed || !first.Saved() || first.ID != answer.ID {
		t.Fatalf("first save: %+v replayed %v, %v", first, replayed, err)
	}
	if n := r.count(t, `SELECT count(*) FROM observations WHERE id = $1::uuid AND folder_id = $2 AND title = 'Checkout errors by pod'`, first.ObservationID, folder); n != 1 {
		t.Fatalf("observation %s is not in the dashboard's folder %s", first.ObservationID, folder)
	}
	if n := r.count(t, `SELECT count(*) FROM observation_blocks WHERE observation_id = $1::uuid AND kind = 'text' AND run_id = $2`, first.ObservationID, request); n != 1 {
		t.Fatalf("text blocks of the saved answer: %d", n)
	}
	run, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), request)
	if err != nil || run.ObservationID.String() != first.ObservationID {
		t.Fatalf("run observation %v, want %s, %v", run.ObservationID, first.ObservationID, err)
	}

	again, replayed, err := r.service.SaveAsObservation(reading(t.Context()), editor, request)
	if err != nil || !replayed || again.ObservationID != first.ObservationID {
		t.Fatalf("repeat: %+v replayed %v, %v", again, replayed, err)
	}
	if n := r.count(t, `SELECT count(*) FROM observations`); n != 1 {
		t.Fatalf("%d observations after a repeated save", n)
	}
}

func TestSavingAHomeAnswerAsAnObservationKeepsTheRunABuild(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, pgtype.UUID{})
	r.save(t, editor, findings.SaveInput{AgentRequestID: request, Title: "Which logs exist"})

	saved, _, err := r.service.SaveAsObservation(reading(t.Context()), editor, request)
	if err != nil || !saved.Saved() {
		t.Fatalf("save: %+v, %v", saved, err)
	}
	run, err := r.db.Queries().GetAgentRequestByID(reading(t.Context()), request)
	if err != nil {
		t.Fatal(err)
	}
	if run.Kind != "build" || run.Intent != "ask" || run.ObservationID.String() != saved.ObservationID {
		t.Fatalf("the saved run is kind %q intent %q observation %v, want a build ask naming %s", run.Kind, run.Intent, run.ObservationID, saved.ObservationID)
	}
}

func TestOnlyTheRequesterSavesAnAnswerAsAnObservation(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, pgtype.UUID{})
	r.save(t, editor, findings.SaveInput{AgentRequestID: request})

	if _, _, err := r.service.SaveAsObservation(reading(t.Context()), admin, request); !errors.Is(err, findings.ErrRequesterOnly) {
		t.Errorf("an Admin: %v, want ErrRequesterOnly", err)
	}
	if _, _, err := r.service.SaveAsObservation(reading(t.Context()), otherEditor, request); !errors.Is(err, findings.ErrNotFound) {
		t.Errorf("another Editor: %v, want ErrNotFound", err)
	}
	if _, _, err := r.service.SaveAsObservation(reading(t.Context()), editor, r.request(t, editor, pgtype.UUID{})); !errors.Is(err, findings.ErrNotFound) {
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

	if _, _, err := r.service.Pin(reading(t.Context()), editor, mustUUID(t, answer.ID)); !errors.Is(err, findings.ErrNotSaved) {
		t.Fatalf("pin: %v, want ErrNotSaved", err)
	}
	if _, _, err := r.service.SaveAsObservation(reading(t.Context()), editor, request); err != nil {
		t.Fatal(err)
	}
	if _, created, err := r.service.Pin(reading(t.Context()), editor, mustUUID(t, answer.ID)); err != nil || !created {
		t.Fatalf("pin after saving: created %v, %v", created, err)
	}
}

func TestAnUnsavedAnswerOnADashboardFollowsWhoCanStillSeeThatDashboard(t *testing.T) {
	r := newRig(t)
	private := r.dashboard(t, "Mine")
	r.db.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, private, r.personalFolder(t, editor))
	onPrivate := r.request(t, editor, private)
	answer := r.save(t, editor, findings.SaveInput{AgentRequestID: onPrivate, DashboardID: private})

	if _, err := r.service.Get(reading(t.Context()), admin, mustUUID(t, answer.ID)); !errors.Is(err, findings.ErrNotFound) {
		t.Errorf("an Admin reads an answer on a dashboard in someone's My Dashboards: %v, want ErrNotFound", err)
	}
	if _, found, err := r.service.ForRequest(reading(t.Context()), admin, onPrivate); err != nil || found {
		t.Errorf("an Admin finds the answer of the run: found %v, %v", found, err)
	}
	if _, err := r.service.Get(reading(t.Context()), editor, mustUUID(t, answer.ID)); err != nil {
		t.Errorf("the requester: %v", err)
	}

	shared := r.request(t, editor, r.open)
	onShared := r.save(t, editor, findings.SaveInput{AgentRequestID: shared, DashboardID: r.open})
	restricted := r.folder(t, "Restricted")
	r.db.Exec(t, `INSERT INTO folder_permissions (folder_id, group_name, level) VALUES ($1, 'sre', 'view')`, restricted)
	r.db.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, r.open, restricted)
	if _, err := r.service.Get(reading(t.Context()), editor, mustUUID(t, onShared.ID)); !errors.Is(err, findings.ErrNotFound) {
		t.Errorf("the requester who lost the dashboard reads its answer: %v, want ErrNotFound", err)
	}
}

func TestDeletingARunTakesItsUnsavedAnswerWithIt(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, r.open)
	r.save(t, editor, findings.SaveInput{AgentRequestID: request})

	if _, err := r.db.SQLDB(t).ExecContext(reading(t.Context()), `DELETE FROM dashboards WHERE id = $1`, r.open); err != nil {
		t.Fatalf("delete the dashboard the run was asked on: %v", err)
	}
	if n := r.count(t, `SELECT count(*) FROM findings`); n != 0 {
		t.Fatalf("%d findings left after their run was deleted", n)
	}
}

func TestAReadOfAnUnsavedAnswerIsRecordedAsAReadOfItsRun(t *testing.T) {
	r := newRig(t)
	var read []pgtype.UUID
	r.service.RecordAnswerReadsWith(func(_ context.Context, run pgtype.UUID) { read = append(read, run) })
	unsaved := r.request(t, editor, pgtype.UUID{})
	answer := r.save(t, editor, findings.SaveInput{AgentRequestID: unsaved})
	filed := r.request(t, editor, pgtype.UUID{})
	r.save(t, editor, findings.SaveInput{AgentRequestID: filed})
	saved, _, err := r.service.SaveAsObservation(reading(t.Context()), editor, filed)
	if err != nil {
		t.Fatal(err)
	}

	for _, who := range []auth.Principal{editor, admin} {
		if _, err := r.service.Read(reading(t.Context()), who, mustUUID(t, answer.ID)); err != nil {
			t.Fatalf("%s reads the unsaved answer: %v", who.Name, err)
		}
	}
	if _, err := r.service.Read(reading(t.Context()), otherEditor, mustUUID(t, answer.ID)); !errors.Is(err, findings.ErrNotFound) {
		t.Fatalf("another Editor reads the unsaved answer: %v, want ErrNotFound", err)
	}
	if _, err := r.service.Read(reading(t.Context()), editor, mustUUID(t, saved.ID)); err != nil {
		t.Fatalf("read the saved answer: %v", err)
	}
	if len(read) != 2 || read[0] != unsaved || read[1] != unsaved {
		t.Fatalf("recorded run reads %v, want the unsaved answer's run %v twice", read, unsaved)
	}
}

func TestAnObservationMadeFromAnAnswerCarriesTheRunAndItsFindingAndKeepsTheAnswersWindow(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, pgtype.UUID{})
	onset := windowTo.Add(-30 * time.Minute)
	answered := findings.SaveInput{
		From: windowTo.Add(-24 * time.Hour),
		To:   windowTo.Add(-23 * time.Hour),
	}
	r.save(t, editor, findings.SaveInput{
		AgentRequestID: request,
		Title:          "Checkout errors",
		From:           answered.From,
		To:             answered.To,
		Structured:     findings.Structured{Onset: &onset, Impact: "12% of orders failed."},
	})

	saved, _, err := r.service.SaveAsObservation(reading(t.Context()), editor, request)
	if err != nil {
		t.Fatal(err)
	}
	o, err := r.observations.Get(reading(t.Context()), editor, mustUUID(t, saved.ObservationID))
	if err != nil {
		t.Fatal(err)
	}
	if o.From == nil || o.To == nil || !o.From.Equal(answered.From) || !o.To.Equal(answered.To) {
		t.Errorf("observation window %v to %v, want the answer's %v to %v", o.From, o.To, answered.From, answered.To)
	}
	var kinds []observations.Kind
	for _, b := range o.Blocks {
		kinds = append(kinds, b.Kind)
		if b.RunID != request.String() {
			t.Errorf("block %s belongs to run %q, want %s", b.Kind, b.RunID, request)
		}
	}
	if len(kinds) < 2 || kinds[0] != observations.KindRun || kinds[1] != observations.KindText {
		t.Errorf("block kinds %v, want the run block then the answer", kinds)
	}
	if len(o.Findings) != 1 || o.Findings[0].RunID != request.String() || o.Findings[0].Fields.Impact != "12% of orders failed." {
		t.Errorf("finding fields %+v, want the run's saved finding", o.Findings)
	}
}
