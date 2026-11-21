package revisions_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestARunThatReadAdminOnlyDataIsShownOnlyToItsRequesterAndAdmins(t *testing.T) {
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	for _, restricted := range []bool{false, true} {
		svc, d := newService(t, succeedWith(2))
		dash := seedDashboard(t, d, 2)
		id := observeFor(t, svc, user, "build it")
		d.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', intent = 'apply', result_version = 2, dashboard_id = $2, read_restricted = $3 WHERE id = $1", mustUUID(t, id), dash, restricted)
		ctx := reading(context.Background())

		for name, who := range map[string]auth.Principal{"its requester": user, "an Admin": bea, "another Editor": eve} {
			want := who.ID != eve.ID || !restricted
			_, _, readErr := svc.ReadObserve(ctx, mustUUID(t, id), who)
			_, getErr := svc.Get(ctx, who, dash, mustUUID(t, id))
			items, _, listErr := svc.List(ctx, who, dash, 10, "")
			if listErr != nil {
				t.Fatal(listErr)
			}
			listed := slices.ContainsFunc(items, func(r revisions.Revision) bool { return r.ID == id })
			if (readErr == nil) != want || (getErr == nil) != want || listed != want {
				t.Errorf("restricted %v, %s: read %v, get %v, listed %v; want shown %v", restricted, name, readErr, getErr, listed, want)
			}
			if !want && (!errors.Is(readErr, revisions.ErrRequestNotFound) || !errors.Is(getErr, revisions.ErrNotFound)) {
				t.Errorf("restricted %v, %s: read %v, get %v; want the not found answers", restricted, name, readErr, getErr)
			}
		}
		if _, err := svc.GetObserve(ctx, mustUUID(t, id), eve); (err == nil) == restricted {
			t.Errorf("restricted %v: GetObserve for another Editor = %v", restricted, err)
		}
	}
}

func TestARunKeptByAnAdminsOnlySubjectIsShownOnlyToItsRequesterAndAdminsUntilTheSubjectMovesOut(t *testing.T) {
	eve := auth.Principal{Kind: auth.KindUser, ID: strangerID, Name: "Eve", Role: auth.RoleEditor}
	for _, movedOut := range []bool{false, true} {
		svc, d := newService(t, succeedWith(2))
		dash := seedDashboard(t, d, 2)
		var subject string
		if err := d.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name) VALUES ($1, 'Leadership notes', 'user', gen_random_uuid(), 'Root') RETURNING id::text`, foldertest.AdminsOnly(t, d, "Leadership")).Scan(&subject); err != nil {
			t.Fatal(err)
		}
		id := observeFor(t, svc, user, "build it")
		d.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', intent = 'apply', result_version = 2, dashboard_id = $2, observation_id = $3::uuid WHERE id = $1", mustUUID(t, id), dash, subject)
		if movedOut {
			d.Exec(t, `UPDATE observations SET folder_id = general_folder_id() WHERE id = $1::uuid`, subject)
		}
		ctx := reading(context.Background())

		for name, who := range map[string]auth.Principal{"its requester": user, "an Admin": bea, "another Editor": eve} {
			want := who.ID != eve.ID || movedOut
			_, _, readErr := svc.ReadObserve(ctx, mustUUID(t, id), who)
			_, getErr := svc.Get(ctx, who, dash, mustUUID(t, id))
			items, _, listErr := svc.List(ctx, who, dash, 10, "")
			if listErr != nil {
				t.Fatal(listErr)
			}
			listed := slices.ContainsFunc(items, func(r revisions.Revision) bool { return r.ID == id })
			if (readErr == nil) != want || (getErr == nil) != want || listed != want {
				t.Errorf("moved out %v, %s: read %v, get %v, listed %v; want shown %v", movedOut, name, readErr, getErr, listed, want)
			}
		}
	}
}
