package placement

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (r holdersRig) block(t *testing.T, observation pgtype.UUID, run string) {
	t.Helper()
	r.d.Exec(t, `INSERT INTO observation_blocks (observation_id, kind, position, body, created_by_kind, created_by_id, created_by_name, run_id) VALUES ($1, 'text', 0, '{"markdown":"pending layoffs: payments team"}', 'user', gen_random_uuid(), 'Ada', $2::uuid)`, observation, run)
}

func TestWhatWasPlacedFromAKeptAnswerStaysHeldWhenItsRunOrSubjectIsDeleted(t *testing.T) {
	r := newHoldersRig(t)
	g := New(nil)
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	moveObservation := func(t *testing.T, id pgtype.UUID) error {
		return g.MoveObservation(t.Context(), r.d.Queries(), id, general)
	}
	moveDashboard := func(t *testing.T, id pgtype.UUID) error {
		return g.MoveDashboard(t.Context(), r.d.Queries(), id, general)
	}
	blockIn := func(t *testing.T, elsewhere pgtype.UUID, run string) (func(*testing.T, pgtype.UUID) error, pgtype.UUID) {
		placed := r.uuid(t, r.observation(t, elsewhere))
		r.block(t, placed, run)
		return moveObservation, placed
	}
	dashboardMadeIn := func(t *testing.T, elsewhere pgtype.UUID, run string) (func(*testing.T, pgtype.UUID) error, pgtype.UUID) {
		placed := r.uuid(t, r.dashboard(t, elsewhere, readsProm))
		r.d.Exec(t, `UPDATE dashboards SET created_run_id = $2::uuid WHERE id = $1`, placed, run)
		return moveDashboard, placed
	}
	cases := map[string]struct {
		place   func(t *testing.T, elsewhere pgtype.UUID, run string) (func(*testing.T, pgtype.UUID) error, pgtype.UUID)
		deleted func(t *testing.T, run string) string
		held    bool
	}{
		"a restricted run's block, after its dashboard is deleted": {
			place: blockIn,
			deleted: func(t *testing.T, run string) string {
				r.d.Exec(t, `UPDATE agent_requests SET read_restricted = true WHERE id = $1::uuid`, run)
				return `DELETE FROM dashboards WHERE id = (SELECT dashboard_id FROM agent_requests WHERE id = $1::uuid)`
			},
			held: true,
		},
		"a block of an answer about an Admins-only dashboard, after the dashboard is deleted": {
			place: blockIn,
			deleted: func(t *testing.T, run string) string {
				return `DELETE FROM dashboards WHERE id = (SELECT dashboard_id FROM agent_requests WHERE id = $1::uuid)`
			},
			held: true,
		},
		"a dashboard made from an answer about an Admins-only dashboard, after the dashboard is deleted": {
			place: dashboardMadeIn,
			deleted: func(t *testing.T, run string) string {
				return `DELETE FROM dashboards WHERE id = (SELECT dashboard_id FROM agent_requests WHERE id = $1::uuid)`
			},
			held: true,
		},
		"a restricted run's block, after the run is deleted": {
			place: blockIn,
			deleted: func(t *testing.T, run string) string {
				r.d.Exec(t, `UPDATE agent_requests SET read_restricted = true WHERE id = $1::uuid`, run)
				return `DELETE FROM agent_requests WHERE id = $1::uuid`
			},
			held: true,
		},
		"a block of an answer about a dashboard in General, after the dashboard is deleted": {
			place: blockIn,
			deleted: func(t *testing.T, run string) string {
				r.d.Exec(t, `UPDATE dashboards SET folder_id = general_folder_id() WHERE id = (SELECT dashboard_id FROM agent_requests WHERE id = $1::uuid)`, run)
				return `DELETE FROM dashboards WHERE id = (SELECT dashboard_id FROM agent_requests WHERE id = $1::uuid)`
			},
			held: false,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			home, elsewhere := foldertest.AdminsOnly(t, r.d, name), foldertest.AdminsOnly(t, r.d, name+" elsewhere")
			subject := r.dashboard(t, home, readsProm)
			run := r.run(t, false)
			r.ended(t, run, `dashboard_id = $2::uuid`, subject)
			move, placed := c.place(t, elsewhere, run)
			r.d.Exec(t, c.deleted(t, run), run)

			err := move(t, placed)
			if c.held && !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("moving it to General: err = %v, want the restricted answer refusal", err)
			}
			if !c.held && err != nil {
				t.Fatalf("moving it to General: %v", err)
			}
		})
	}
}

func TestARunOnASharedDashboardThatFiledIntoADeletedAdminsOnlyObservationKeepsWhatItPlacedHeld(t *testing.T) {
	r := newHoldersRig(t)
	g := New(nil)
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	board := foldertest.AdminsOnly(t, r.d, "Board")
	filed := r.observation(t, foldertest.AdminsOnly(t, r.d, "Leadership"))
	on := r.dashboard(t, general, readsProm)
	run := r.run(t, false)
	r.ended(t, run, `dashboard_id = $2::uuid`, on)
	r.d.Exec(t, `UPDATE agent_requests SET observation_id = $2::uuid WHERE id = $1::uuid`, run, filed)
	saved := r.uuid(t, r.observation(t, board))
	r.block(t, saved, run)

	r.d.Exec(t, `DELETE FROM observations WHERE id = $1::uuid`, filed)

	if err := g.MoveObservation(t.Context(), r.d.Queries(), saved, general); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("moving the block's observation to General: err = %v, want the restricted answer refusal", err)
	}
}
