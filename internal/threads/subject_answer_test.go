package threads

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) answerAbout(t *testing.T, req revisions.StartRequest) (pgtype.UUID, findings.Finding) {
	t.Helper()
	run, f := r.restrictedAnswer(t, admin, req)
	r.db.Exec(t, `UPDATE agent_requests SET read_restricted = false WHERE id = $1`, run)
	return run, f
}

func (r *rig) generalFolder(t *testing.T) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id FROM folders WHERE general`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) applyOnto(t *testing.T, dashboard pgtype.UUID, f findings.Finding) error {
	t.Helper()
	id, revise, finding := dashboard.String(), revisions.KindRevise, f.ID
	_, err := r.revisions.Start(reading(t.Context()), admin, revisions.StartRequest{DashboardID: &id, Kind: &revise, AppliesFindingID: &finding})
	return err
}

func TestAnAnswerAboutADashboardOnlyAdminsReadIsPlacedOnlyOnItOrWhereOnlyAdminsReadUntilItMovesOut(t *testing.T) {
	r := newShareRig(t)
	dash := r.createDashboard(t, "rps")
	home := r.adminsOnlyFolder(t)
	r.moveInto(t, "dashboards", dash, home)
	shared := r.createDashboard(t, "errors")
	about := revisions.StartRequest{DashboardID: new(dash.String())}

	_, f := r.answerAbout(t, about)
	if _, _, err := r.findings.SaveAsDashboard(reading(t.Context()), admin, mustParse(t, f.ID)); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("save as a dashboard in the default folder: err = %v, want the restricted answer refusal", err)
	}
	if err := r.applyOnto(t, shared, f); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("apply onto a dashboard in General: err = %v, want the restricted answer refusal", err)
	}
	if err := r.applyOnto(t, dash, f); errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("apply onto the dashboard it is about: %v", err)
	}
	saved, _ := r.answerAbout(t, about)
	if _, _, err := r.findings.SaveAsObservation(reading(t.Context()), admin, saved); err != nil {
		t.Fatalf("save as observation: %v", err)
	}
	if folder := r.savedInto(t, saved); folder != home {
		t.Fatalf("saved into %v, want the dashboard's folder %v", folder, home)
	}
	sharedRun, _ := r.answerAbout(t, about)
	if _, err := r.service.ShareRun(reading(t.Context()), admin, ShareInput{RunID: sharedRun}); err != nil {
		t.Fatalf("share into a thread on the dashboard it is about: %v", err)
	}

	r.moveInto(t, "dashboards", dash, r.generalFolder(t))
	_, f = r.answerAbout(t, about)
	if _, _, err := r.findings.SaveAsDashboard(reading(t.Context()), admin, mustParse(t, f.ID)); err != nil {
		t.Fatalf("save as a dashboard after the dashboard moved out: %v", err)
	}
	if err := r.applyOnto(t, shared, f); errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("apply onto a dashboard in General after the dashboard moved out: %v", err)
	}
	saved, _ = r.answerAbout(t, about)
	if _, _, err := r.findings.SaveAsObservation(reading(t.Context()), admin, saved); err != nil {
		t.Fatalf("save as observation after the dashboard moved out: %v", err)
	}
	if folder := r.savedInto(t, saved); folder == home {
		t.Fatal("an answer about a dashboard that moved out was still saved where only Admins read")
	}
}

func TestAnAnswerAboutAnObservationOnlyAdminsReadGoesOnItsPageButNotOutUntilItMovesOut(t *testing.T) {
	r := newShareRig(t)
	observation := r.filedObservationIn(t, admin, r.adminsOnlyFolder(t))
	about := revisions.StartRequest{ObservationID: new(observation.String())}
	blocks := r.blockCount(t, observation)

	run, f := r.answerAbout(t, about)
	if _, _, err := r.findings.SaveAsDashboard(reading(t.Context()), admin, mustParse(t, f.ID)); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("save as a dashboard in the default folder: err = %v, want the restricted answer refusal", err)
	}
	if _, err := r.service.ShareRun(reading(t.Context()), admin, ShareInput{RunID: run}); err != nil {
		t.Fatalf("show it on the page of the observation it is about: %v", err)
	}
	if n := r.blockCount(t, observation); n <= blocks {
		t.Fatalf("blocks after the share = %d, want more than %d", n, blocks)
	}

	r.moveInto(t, "observations", observation, r.generalFolder(t))
	_, f = r.answerAbout(t, about)
	if _, _, err := r.findings.SaveAsDashboard(reading(t.Context()), admin, mustParse(t, f.ID)); err != nil {
		t.Fatalf("save as a dashboard after the observation moved out: %v", err)
	}
}
