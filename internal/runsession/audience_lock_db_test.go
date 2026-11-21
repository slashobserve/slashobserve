package runsession

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

const lockWait = 300 * time.Millisecond

func TestTheAudienceOfARunWaitsForAMoveOrAnOpeningInProgressAndReadsWhatItLeft(t *testing.T) {
	cases := map[string]struct {
		change string
		of     func(r *rig, folder pgtype.UUID) pgtype.UUID
	}{
		"a move of its dashboard out of a folder only Admins read": {"UPDATE dashboards SET folder_id = general_folder_id() WHERE id = $1", func(r *rig, _ pgtype.UUID) pgtype.UUID { return r.dash }},
		"opening the folder its dashboard is in":                   {"UPDATE folders SET admins_only = false WHERE id = $1", func(_ *rig, folder pgtype.UUID) pgtype.UUID { return folder }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			req := asRole(r.revise(), auth.RoleAdmin)
			folder := r.moveDashboard(true)
			tx, err := r.d.SQLDB(t).BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.ExecContext(t.Context(), tc.change, tc.of(r, folder)); err != nil {
				t.Fatal(err)
			}
			read := make(chan datasource.Audience, 1)
			go func() {
				run, err := r.svc.Scope(context.Background(), req)
				if err != nil {
					t.Error(err)
				}
				read <- run.Access.Audience
			}()
			select {
			case got := <-read:
				t.Fatalf("the run read its audience %q before the change committed", got)
			case <-time.After(lockWait):
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if got := <-read; got != datasource.AudienceEveryone {
				t.Fatalf("audience after the change = %q, want %q", got, datasource.AudienceEveryone)
			}
		})
	}
}
