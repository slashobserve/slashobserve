package findings_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func (r rig) general(t *testing.T) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM folders WHERE general`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return mustUUID(t, raw)
}

func (r rig) appendThreadRow(t *testing.T, q *dbsqlc.Queries, observation pgtype.UUID) error {
	t.Helper()
	_, err := activity.Append(t.Context(), q, activity.Entry{
		Type:     activity.TypeThreadOpened,
		TargetID: mustUUID(t, "5f3c1a52-8a0e-4b8c-9a55-0a6f3e4b7d10"),
		Owner:    activity.OnObservation(observation),
		Actor:    actor.Actor{Kind: auth.KindUser, ID: editor.ID, Name: editor.Name},
	}, nil)
	return err
}

func (r rig) move(t *testing.T, by auth.Principal, f findings.Finding, folder pgtype.UUID) (findings.Finding, error) {
	t.Helper()
	return r.service.Move(t.Context(), by, mustUUID(t, f.ID), folder)
}

func (r rig) movedRows(t *testing.T, by auth.Principal) []activity.Event {
	t.Helper()
	var moved []activity.Event
	for _, e := range r.activityFor(t, by) {
		if e.Type == activity.TypeFindingMoved {
			moved = append(moved, e)
		}
	}
	return moved
}

func TestMovingAFindingOutOfMyDashboardsSharesItWithTheTargetsReaders(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, editor, findings.SaveInput{Title: "Slow pod"})
	if err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error { return r.appendThreadRow(t, q, mustUUID(t, saved.ObservationID)) }); err != nil {
		t.Fatal(err)
	}
	personal := r.personalFolder(t, editor)
	shared := r.folder(t, "Incidents")
	if _, err := r.service.Get(t.Context(), viewer, mustUUID(t, saved.ID)); !errors.Is(err, findings.ErrNotFound) {
		t.Fatalf("before the move a viewer reads it: %v", err)
	}
	if got := r.activityFor(t, viewer); len(got) != 0 {
		t.Fatalf("before the move a viewer sees %v", typesOf(got))
	}

	moved, err := r.move(t, editor, saved, shared)
	if err != nil {
		t.Fatal(err)
	}
	if moved.FolderID != shared.String() || moved.Folder == nil || moved.Folder.Title != "Incidents" || !moved.Abilities.Edit {
		t.Fatalf("moved finding: folder %q %+v abilities %+v", moved.FolderID, moved.Folder, moved.Abilities)
	}
	read, err := r.service.Get(t.Context(), viewer, mustUUID(t, saved.ID))
	if err != nil || read.Abilities.Edit || !read.Abilities.View {
		t.Fatalf("a viewer of the target: %+v, %v", read.Abilities, err)
	}

	got := r.activityFor(t, viewer)
	want := []string{activity.TypeObservationCreated, activity.TypeObservationBlockAdded, activity.TypeFindingCreated, activity.TypeThreadOpened, activity.TypeObservationMoved, activity.TypeFindingMoved}
	if !slices.Equal(typesOf(got), want) {
		t.Fatalf("a viewer of the target sees %v, want %v", typesOf(got), want)
	}
	for _, e := range got {
		if e.FolderID != shared.String() {
			t.Errorf("%s is in folder %s, want %s", e.Type, e.FolderID, shared)
		}
	}
	data := dataOf(t, got[5])
	if data["title"] != "Slow pod" || data["from_folder_id"] != personal.String() || data["to_folder_id"] != shared.String() {
		t.Errorf("moved data = %v", data)
	}
}

func TestMovingAFindingOutOfGeneralRecordsGeneralAsWhereItCameFrom(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, ciKey, findings.SaveInput{})
	shared := r.folder(t, "Incidents")

	moved, err := r.move(t, editor, saved, shared)
	if err != nil {
		t.Fatal(err)
	}
	if moved.FolderID != shared.String() || moved.Folder == nil || moved.Folder.General {
		t.Fatalf("moved finding: %q %+v", moved.FolderID, moved.Folder)
	}
	rows := r.movedRows(t, viewer)
	if len(rows) != 1 {
		t.Fatalf("moved rows = %d", len(rows))
	}
	var data map[string]any
	if err := json.Unmarshal(rows[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["from_folder_id"] != r.general(t).String() || data["to_folder_id"] != shared.String() {
		t.Errorf("moved data = %v", data)
	}
}

func TestMovingAFindingIsRefusedWhereTheRulesSay(t *testing.T) {
	r := newRig(t)
	shared := r.folder(t, "Incidents")
	other := r.folder(t, "Payments")
	filed := r.save(t, editor, findings.SaveInput{FolderID: shared})
	onDashboard := r.save(t, editor, findings.SaveInput{DashboardID: r.open})
	viewersOwn := r.save(t, viewer, findings.SaveInput{})
	editorsPersonal := r.personalFolder(t, editor)
	missing := mustUUID(t, "99999999-9999-4999-8999-999999999999")
	hidden := r.save(t, editor, findings.SaveInput{})

	cases := []struct {
		name   string
		by     auth.Principal
		move   findings.Finding
		target pgtype.UUID
		want   error
	}{
		{"a finding on a dashboard", editor, onDashboard, shared, findings.ErrOnDashboard},
		{"a viewer of the source folder", viewer, filed, other, folders.ErrForbidden},
		{"a target without Edit", viewer, viewersOwn, r.general(t), folders.ErrForbidden},
		{"a personal folder", editor, filed, editorsPersonal, folders.ErrMoveIntoPersonal},
		{"an unknown folder", editor, filed, missing, findings.ErrFolderNotFound},
		{"a finding the caller cannot read", otherEditor, hidden, shared, findings.ErrNotFound},
	}
	for _, c := range cases {
		if _, err := r.move(t, c.by, c.move, c.target); !errors.Is(err, c.want) {
			t.Errorf("%s: error %v, want %v", c.name, err, c.want)
		}
	}
	if rows := r.movedRows(t, admin); len(rows) != 0 {
		t.Fatalf("refused moves appended %d rows", len(rows))
	}
}

func TestMovingAFindingIntoItsOwnFolderChangesNothing(t *testing.T) {
	r := newRig(t)
	shared := r.folder(t, "Incidents")
	filed := r.save(t, editor, findings.SaveInput{FolderID: shared})
	same, err := r.move(t, editor, filed, shared)
	if err != nil || same.FolderID != shared.String() {
		t.Fatalf("move: %+v, %v", same, err)
	}
	if rows := r.movedRows(t, editor); len(rows) != 0 {
		t.Fatalf("a no-op move appended %d rows", len(rows))
	}
}

func TestAThreadRowCommittedDuringAMoveEndsInTheTargetFolder(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, editor, findings.SaveInput{})
	id := mustUUID(t, saved.ID)
	shared := r.folder(t, "Incidents")

	appended, release, holderDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		holderDone <- r.db.InTx(context.WithoutCancel(t.Context()), func(q *dbsqlc.Queries) error {
			if err := r.appendThreadRow(t, q, id); err != nil {
				return err
			}
			close(appended)
			<-release
			return nil
		})
	}()
	select {
	case <-appended:
	case <-time.After(10 * time.Second):
		t.Fatal("the holding transaction never appended")
	}

	moveDone := make(chan error, 1)
	go func() {
		_, err := r.service.Move(context.WithoutCancel(t.Context()), editor, id, shared)
		moveDone <- err
	}()
	waitForALockWait(t, r)
	close(release)

	for name, done := range map[string]chan error{"holder": holderDone, "move": moveDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s never finished", name)
		}
	}
	var folder string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT folder_id::text FROM activity WHERE type = $1`, activity.TypeThreadOpened).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	if folder != shared.String() {
		t.Fatalf("the concurrent thread row is in folder %s, want %s", folder, shared)
	}
}

func TestMovingAFindingOverHTTP(t *testing.T) {
	d := newDocumented(t)
	shared := d.folder(t, "Incidents")
	filed := d.save(t, editor, findings.SaveInput{FolderID: shared})
	onDashboard := d.save(t, editor, findings.SaveInput{DashboardID: d.open})
	mine := d.save(t, editor, findings.SaveInput{})
	personal := d.personalFolder(t, editor)
	path := func(f findings.Finding) string { return "/api/v1/findings/" + f.ID + "/move" }
	body := func(folder string) string { return `{"folder_id":"` + folder + `"}` }

	refusals := []struct {
		name    string
		by      auth.Principal
		finding findings.Finding
		body    string
		status  int
		code    string
	}{
		{"no folder", editor, mine, `{}`, http.StatusBadRequest, "invalid_request"},
		{"a folder that is not a UUID", editor, mine, body("nope"), http.StatusBadRequest, "invalid_request"},
		{"an unknown folder", editor, mine, body("99999999-9999-4999-8999-999999999999"), http.StatusNotFound, "not_found"},
		{"a finding the caller cannot read", otherEditor, mine, body(shared.String()), http.StatusNotFound, "not_found"},
		{"a viewer of the source", viewer, filed, body(shared.String()), http.StatusForbidden, "forbidden_role"},
		{"a finding on a dashboard", editor, onDashboard, body(shared.String()), http.StatusConflict, "finding_on_dashboard"},
		{"My Dashboards", editor, filed, body(personal.String()), http.StatusConflict, "personal_folder"},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			if code := errorCode(t, d.send(t, c.by, http.MethodPost, path(c.finding), c.body, c.status)); code != c.code {
				t.Fatalf("code %q, want %q", code, c.code)
			}
		})
	}
	rec := d.send(t, editor, http.MethodPost, path(mine), body(shared.String()), http.StatusOK)
	var got struct {
		Finding struct {
			FolderID string `json:"folder_id"`
		} `json:"finding"`
		Folder struct {
			Title string `json:"title"`
		} `json:"folder"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Finding.FolderID != shared.String() || got.Folder.Title != "Incidents" {
		t.Fatalf("move answered %s, %v", rec.Body, err)
	}
}
