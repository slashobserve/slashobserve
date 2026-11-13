package threads

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/observations"
)

func (r *rig) createObservation(t *testing.T, by auth.Principal, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	o, err := r.observations.Create(t.Context(), by, observations.CreateInput{FolderID: folder, Title: "Checkout 5xx"})
	if err != nil {
		t.Fatal(err)
	}
	return mustParse(t, o.ID)
}

func TestAnObservationHasOneDiscussionThatOpensRepliesAndResolves(t *testing.T) {
	r := newRig(t)
	id := r.createObservation(t, testUser, pgtype.UUID{})
	tg := target(KindObservation, id)

	thread, first := r.open(t, testUser, OpenInput{Target: tg, Body: "what changed at 09:10?"})
	if thread.Target != tg || thread.Status != StatusOpen || thread.DashboardID != "" || thread.Version != nil {
		t.Fatalf("thread %+v", thread)
	}
	r.reply(t, commenter, first.ID, "a deploy")
	resolved, changed, err := r.service.SetStatus(t.Context(), testUser, mustParse(t, thread.ID), StatusResolved, nil)
	if err != nil || !changed || resolved.Status != StatusResolved {
		t.Fatalf("resolve = %+v, %v, %v", resolved, changed, err)
	}
	reopened, changed, err := r.service.SetStatus(t.Context(), testUser, mustParse(t, thread.ID), StatusOpen, nil)
	if err != nil || !changed || reopened.Status != StatusOpen {
		t.Fatalf("reopen = %+v, %v, %v", reopened, changed, err)
	}

	page := r.list(t, viewer, tg)
	if len(page.Threads) != 1 || len(page.Messages) != 2 || page.CanComment {
		t.Fatalf("a viewer reads %+v", page)
	}
	if page := r.list(t, commenter, tg); !page.CanComment {
		t.Error("a commenter can write")
	}
	found, err := r.observations.Get(t.Context(), testUser, id)
	if err != nil || found.ThreadID != thread.ID {
		t.Errorf("the observation names thread %q, %v", found.ThreadID, err)
	}
}

func TestASecondDiscussionOnAnObservationIsRefusedWithTheFirstOnesID(t *testing.T) {
	r := newRig(t)
	tg := target(KindObservation, r.createObservation(t, testUser, pgtype.UUID{}))
	thread, _ := r.open(t, testUser, OpenInput{Target: tg, Body: "first"})

	_, _, err := r.service.Open(t.Context(), commenter, OpenInput{Target: tg, Body: "second"}, nil)
	var exists *ObservationThreadExistsError
	if !errors.As(err, &exists) || exists.ThreadID.String() != thread.ID {
		t.Fatalf("second open: %v", err)
	}
	if page := r.list(t, testUser, tg); len(page.Threads) != 1 || len(page.Messages) != 1 {
		t.Errorf("page %+v", page)
	}
}

func TestAnObservationInAHiddenFolderIsNotFoundToEveryoneWhoCannotSeeIt(t *testing.T) {
	r := newRig(t)
	folder := foldertest.Restricted(t, r.db, "Payroll", payrollGroup)
	id := r.createObservation(t, admin, folder)
	tg := target(KindObservation, id)
	if _, err := r.service.Abilities(t.Context(), admin, tg); err != nil {
		t.Fatalf("the admin: %v", err)
	}
	thread, _ := r.open(t, admin, OpenInput{Target: tg, Body: "private"})

	if _, _, err := r.service.Open(t.Context(), viewer, OpenInput{Target: tg, Body: "hi"}, nil); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("open: %v", err)
	}
	if _, err := r.service.List(t.Context(), viewer, ListQuery{Target: tg, Limit: 10}); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("list: %v", err)
	}
	if _, _, err := r.service.Reply(t.Context(), viewer, mustParse(t, thread.ID), "hi", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("reply: %v", err)
	}
	missing := target(KindObservation, mustParse(t, "00000000-0000-4000-8000-000000000000"))
	if _, err := r.service.List(t.Context(), testUser, ListQuery{Target: missing, Limit: 10}); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("a missing observation: %v", err)
	}
}

func TestAViewerCannotStartADiscussionOnAnObservation(t *testing.T) {
	r := newRig(t)
	tg := target(KindObservation, r.createObservation(t, testUser, pgtype.UUID{}))
	if _, _, err := r.service.Open(t.Context(), viewer, OpenInput{Target: tg, Body: "hi"}, nil); err == nil || errors.Is(err, ErrTargetNotFound) {
		t.Errorf("a viewer opening: %v", err)
	}
}

func TestAnObservationThreadIsAnAnchorlessTarget(t *testing.T) {
	r := newRig(t)
	tg := target(KindObservation, r.createObservation(t, testUser, pgtype.UUID{}))
	version, panel := 1, "p"
	for name, in := range map[string]OpenInput{"a version": {Target: tg, Body: "x", Version: &version}, "a panel": {Target: tg, Body: "x", PanelID: &panel}} {
		_, _, err := r.service.Open(t.Context(), testUser, in, nil)
		var invalid *InvalidInputError
		if !errors.As(err, &invalid) || invalid.Code != CodeInvalidTarget {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestThreadEventsOnAnObservationAreOwnedByIt(t *testing.T) {
	r := newRig(t)
	id := r.createObservation(t, testUser, pgtype.UUID{})
	thread, first := r.open(t, testUser, OpenInput{Target: target(KindObservation, id), Body: "private words"})
	r.reply(t, commenter, first.ID, "more private words")
	if _, _, err := r.service.SetStatus(t.Context(), testUser, mustParse(t, thread.ID), StatusResolved, nil); err != nil {
		t.Fatal(err)
	}

	rows, err := r.db.SQLDB(t).QueryContext(t.Context(), `SELECT type, coalesce(observation_id::text, ''), coalesce(folder_id::text, ''), data::text FROM activity WHERE target_kind = 'thread' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var types []string
	for rows.Next() {
		var typ, observation, folder, data string
		if err := rows.Scan(&typ, &observation, &folder, &data); err != nil {
			t.Fatal(err)
		}
		if observation != id.String() || folder == "" || strings.Contains(data, "private") {
			t.Errorf("%s: observation %q folder %q data %s", typ, observation, folder, data)
		}
		types = append(types, typ)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{activity.TypeThreadOpened, activity.TypeThreadReplied, activity.TypeThreadResolved}; strings.Join(types, ",") != strings.Join(want, ",") {
		t.Errorf("types %v, want %v", types, want)
	}
}

func TestOpeningASecondObservationThreadOverHTTPAnswers409WithTheThreadID(t *testing.T) {
	r := newRig(t)
	tg := "observation:" + r.createObservation(t, testUser, pgtype.UUID{}).String()
	first := r.call(t, commenter, http.MethodPost, "/threads", map[string]any{"target": tg, "body": "first"})
	first.expect(t, http.StatusCreated, "")
	if first.at(t, "thread", "target", "kind") != "observation" || first.at(t, "thread", "dashboard_id") != nil {
		t.Errorf("thread = %v", first.body["thread"])
	}
	second := r.call(t, commenter, http.MethodPost, "/threads", map[string]any{"target": tg, "body": "second"})
	second.expect(t, http.StatusConflict, CodeObservationThreadExists)
	if second.at(t, "thread_id") != first.at(t, "thread", "id") {
		t.Errorf("409 body %v, want the id of %v", second.body, first.at(t, "thread", "id"))
	}
}

func TestTargetsParseTheObservationKind(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000001"
	got, err := ParseTarget("observation:" + id)
	if err != nil || got.Kind != KindObservation || got.ID.String() != id {
		t.Fatalf("parsed %+v, %v", got, err)
	}
	if _, err := ParseTarget("run:" + id); err == nil || !strings.Contains(err.Error(), "dashboard, annotation, observation or proposal") {
		t.Errorf("an unknown kind: %v", err)
	}
}
