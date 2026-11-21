package placement

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (r holdersRig) run(t *testing.T, readRestricted bool) string {
	t.Helper()
	return r.scalar(t, `INSERT INTO agent_requests (intent, prompt, requested_by_kind, requested_by_id, requested_by_name, read_restricted) VALUES ('ask', 'who changed it', 'user', gen_random_uuid(), 'Root', $1) RETURNING id::text`, readRestricted)
}

func (r holdersRig) uuid(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		t.Fatal(err)
	}
	return id
}

type answerHolder struct {
	add  func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string
	move func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error
}

var answerHolders = map[string]answerHolder{
	"a dashboard version a run saved": {
		add: func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string {
			id := r.dashboard(t, folder, readsProm)
			r.d.Exec(t, `UPDATE dashboard_versions SET agent_request_id = $2::uuid WHERE dashboard_id = $1::uuid`, id, run)
			return id
		},
		move: func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error {
			return g.MoveDashboard(t.Context(), r.d.Queries(), id, to)
		},
	},
	"an observation block a run wrote": {
		add: func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string {
			id := r.observation(t, folder)
			r.d.Exec(t, `INSERT INTO observation_blocks (observation_id, kind, position, body, created_by_kind, created_by_id, created_by_name, run_id) VALUES ($1::uuid, 'text', 0, '{"markdown":"Root changed it"}', 'user', gen_random_uuid(), 'Ada', $2::uuid)`, id, run)
			return id
		},
		move: func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error {
			return g.MoveObservation(t.Context(), r.d.Queries(), id, to)
		},
	},
}

func TestContentHoldingAnAnswerThatReadAdminOnlyDataStaysWhereOnlyAdminsRead(t *testing.T) {
	r := newHoldersRig(t)
	g := New(nil)
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	for name, h := range answerHolders {
		t.Run(name, func(t *testing.T) {
			folder := foldertest.AdminsOnly(t, r.d, name)
			id := r.uuid(t, h.add(t, r, folder, r.run(t, true)))

			err := g.OpeningGuard()(t.Context(), r.d.Queries(), dbsqlc.Folder{ID: folder})
			if !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("opening the folder: err = %v, want the restricted answer refusal", err)
			}
			if err := h.move(g, r, t, id, general); !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("moving it to General: err = %v, want the restricted answer refusal", err)
			}
			if err := h.move(g, r, t, id, foldertest.AdminsOnly(t, r.d, name+" too")); err != nil {
				t.Fatalf("moving it to another folder only Admins read: %v", err)
			}
		})
	}
}

func TestContentHoldingOnlyUnrestrictedAnswersMovesAndOpensAsBefore(t *testing.T) {
	r := newHoldersRig(t)
	g := New(nil)
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	for name, h := range answerHolders {
		t.Run(name, func(t *testing.T) {
			folder := foldertest.AdminsOnly(t, r.d, name)
			id := r.uuid(t, h.add(t, r, folder, r.run(t, false)))
			if err := g.OpeningGuard()(t.Context(), r.d.Queries(), dbsqlc.Folder{ID: folder}); err != nil {
				t.Fatalf("opening the folder: %v", err)
			}
			if err := h.move(g, r, t, id, general); err != nil {
				t.Fatalf("moving it to General: %v", err)
			}
		})
	}
}
