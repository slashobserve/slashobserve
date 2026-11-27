package placement

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (r holdersRig) run(t *testing.T, readRestricted bool) string {
	t.Helper()
	return r.scalar(t, `INSERT INTO agent_requests (intent, kind, prompt, requested_by_kind, requested_by_id, requested_by_name, read_restricted) VALUES ('ask', 'investigate', 'who changed it', 'user', gen_random_uuid(), 'Root', $1) RETURNING id::text`, readRestricted)
}

func (r holdersRig) ended(t *testing.T, run, set, id string) {
	t.Helper()
	r.d.Exec(t, `UPDATE agent_requests SET `+set+`, status = 'failed', state = 'failed', failure_code = 'internal', finished_at = now() WHERE id = $1::uuid`, run, id)
}

func (r holdersRig) uuid(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		t.Fatal(err)
	}
	return id
}

func (r holdersRig) reply(t *testing.T, thread, run string) {
	t.Helper()
	r.d.Exec(t, `INSERT INTO thread_messages (thread_id, body, author_kind, author_id, author_name, run_id, run_assigned_kind) VALUES ($1::uuid, 'Root changed it', 'user', gen_random_uuid(), 'Ada', $2::uuid, 'builtin')`, thread, run)
}

type answerHolder struct {
	add    func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string
	move   func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error
	copied bool
}

var answerHolders = map[string]answerHolder{
	"a dashboard made from a run's answer": {
		add: func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string {
			id := r.dashboard(t, folder, readsProm)
			r.d.Exec(t, `UPDATE dashboards SET created_run_id = $2::uuid WHERE id = $1::uuid`, id, run)
			return id
		},
		move: func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error {
			return g.MoveDashboard(t.Context(), r.d.Queries(), id, to)
		},
	},
	"a copy of content holding a run's answer": {
		add: func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string {
			id := r.dashboard(t, folder, readsProm)
			r.d.Exec(t, `UPDATE dashboards SET copied_restricted_answer = (SELECT read_restricted FROM agent_requests WHERE id = $2::uuid) WHERE id = $1::uuid`, id, run)
			return id
		},
		move: func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error {
			return g.MoveDashboard(t.Context(), r.d.Queries(), id, to)
		},
		copied: true,
	},
	"an annotation copied from content holding a run's answer": {
		add: func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string {
			id := r.dashboard(t, folder, readsProm)
			r.d.Exec(t, `INSERT INTO annotations (dashboard_id, time_from, text, copied_restricted_answer) SELECT $1::uuid, now(), 'Sign-ins by Root', read_restricted FROM agent_requests WHERE id = $2::uuid`, id, run)
			return id
		},
		move: func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error {
			return g.MoveDashboard(t.Context(), r.d.Queries(), id, to)
		},
		copied: true,
	},
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
	"a run's reply in a thread on a dashboard": {
		add: func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string {
			id := r.dashboard(t, folder, readsProm)
			thread := r.scalar(t, `INSERT INTO threads (dashboard_id, dashboard_version) VALUES ($1::uuid, 1) RETURNING id::text`, id)
			r.reply(t, thread, run)
			return id
		},
		move: func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error {
			return g.MoveDashboard(t.Context(), r.d.Queries(), id, to)
		},
	},
	"a run's reply in a thread on an annotation of a dashboard": {
		add: func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string {
			id := r.dashboard(t, folder, readsProm)
			annotation := r.scalar(t, `INSERT INTO annotations (dashboard_id, time_from, text) VALUES ($1::uuid, now(), 'deploy') RETURNING id::text`, id)
			thread := r.scalar(t, `INSERT INTO threads (annotation_id) VALUES ($1::uuid) RETURNING id::text`, annotation)
			r.reply(t, thread, run)
			return id
		},
		move: func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error {
			return g.MoveDashboard(t.Context(), r.d.Queries(), id, to)
		},
	},
	"a run's reply in a thread on an observation": {
		add: func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string {
			id := r.observation(t, folder)
			thread := r.scalar(t, `INSERT INTO threads (observation_id) VALUES ($1::uuid) RETURNING id::text`, id)
			r.reply(t, thread, run)
			return id
		},
		move: func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error {
			return g.MoveObservation(t.Context(), r.d.Queries(), id, to)
		},
	},
	"a run on a dashboard, with its steps and answer": {
		add: func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string {
			id := r.dashboard(t, folder, readsProm)
			r.ended(t, run, `dashboard_id = $2::uuid`, id)
			return id
		},
		move: func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error {
			return g.MoveDashboard(t.Context(), r.d.Queries(), id, to)
		},
	},
	"an ask about an observation, with its steps and answer": {
		add: func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string {
			id := r.observation(t, folder)
			r.ended(t, run, `subject_observation_id = $2::uuid`, id)
			return id
		},
		move: func(g *Guard, r holdersRig, t *testing.T, id, to pgtype.UUID) error {
			return g.MoveObservation(t.Context(), r.d.Queries(), id, to)
		},
	},
	"a proposal a run opened": {
		add: func(t *testing.T, r holdersRig, folder pgtype.UUID, run string) string {
			id := r.dashboard(t, folder, readsProm)
			r.d.Exec(t, `INSERT INTO proposals (dashboard_id, title, author_kind, author_id, author_name, run_id) VALUES ($1::uuid, 'Add errors', 'user', gen_random_uuid(), 'Root', $2::uuid)`, id, run)
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
			if err := h.move(g, r, t, id, general); !errors.Is(err, folders.ErrRestrictedAnswer) || folders.RestrictedAnswerReason(err) != folders.ConfinedPermanent {
				t.Fatalf("moving it to General: err = %v, want the permanent restricted answer refusal", err)
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

func TestARunInFlightKeepsWhatItIsAboutWhereOnlyAdminsReadUntilItEnds(t *testing.T) {
	r := newHoldersRig(t)
	g := New(nil)
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	for name, h := range map[string]struct {
		about string
		add   func(t *testing.T, folder pgtype.UUID) string
		move  func(id, to pgtype.UUID) error
	}{
		"a dashboard": {`dashboard_id`, func(t *testing.T, folder pgtype.UUID) string { return r.dashboard(t, folder, readsProm) },
			func(id, to pgtype.UUID) error { return g.MoveDashboard(t.Context(), r.d.Queries(), id, to) }},
		"an observation": {`subject_observation_id`, r.observation,
			func(id, to pgtype.UUID) error { return g.MoveObservation(t.Context(), r.d.Queries(), id, to) }},
	} {
		t.Run(name, func(t *testing.T) {
			folder := foldertest.AdminsOnly(t, r.d, name)
			raw := h.add(t, folder)
			id := r.uuid(t, raw)
			run := r.run(t, false)
			r.d.Exec(t, `UPDATE agent_requests SET `+h.about+` = $2::uuid, status = 'running', state = 'running' WHERE id = $1::uuid`, run, raw)

			err := g.OpeningGuard()(t.Context(), r.d.Queries(), dbsqlc.Folder{ID: folder})
			if !errors.Is(err, folders.ErrRestrictedAnswer) || !strings.Contains(err.Error(), "has not finished") || folders.RestrictedAnswerReason(err) != folders.ConfinedRunInFlight {
				t.Fatalf("opening the folder while a run is working: err = %v", err)
			}
			if err := h.move(id, general); !errors.Is(err, folders.ErrRestrictedAnswer) || !strings.Contains(err.Error(), "wait for it to end") || folders.RestrictedAnswerReason(err) != folders.ConfinedRunInFlight {
				t.Fatalf("moving it to General while a run is working: err = %v", err)
			}
			shared := r.uuid(t, r.scalar(t, `INSERT INTO folders (title) VALUES ($1) RETURNING id::text`, name+" shared"))
			sharedItem := r.uuid(t, h.add(t, shared))
			r.d.Exec(t, `UPDATE agent_requests SET `+h.about+` = $2::uuid WHERE id = $1::uuid`, r.run(t, false), sharedItem)
			if err := h.move(sharedItem, general); err != nil {
				t.Fatalf("moving shared content with a run waiting: %v", err)
			}

			r.d.Exec(t, `UPDATE agent_requests SET status = 'succeeded', state = 'succeeded', finished_at = now() WHERE id = $1::uuid`, run)
			if err := g.OpeningGuard()(t.Context(), r.d.Queries(), dbsqlc.Folder{ID: folder}); err != nil {
				t.Fatalf("opening the folder after the run ended: %v", err)
			}
			if err := h.move(id, general); err != nil {
				t.Fatalf("moving it to General after the run ended: %v", err)
			}
		})
	}
}

func TestAMoveRefusedForARestrictedAnswerNamesTheRunOnlyToWhoMayReadIt(t *testing.T) {
	r := newHoldersRig(t)
	g := New(nil)
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	id := r.observation(t, foldertest.AdminsOnly(t, r.d, "named answer"))
	run := r.run(t, true)
	r.d.Exec(t, `UPDATE agent_requests SET subject_observation_id = $2::uuid WHERE id = $1::uuid`, run, id)
	named := `the answer to "who changed it" (run ` + run + `)`

	admin := auth.WithPrincipal(t.Context(), auth.Principal{Kind: auth.KindUser, ID: uuid.NewString(), Role: auth.RoleAdmin})
	editor := auth.WithPrincipal(t.Context(), auth.Principal{Kind: auth.KindUser, ID: uuid.NewString(), Role: auth.RoleEditor})
	asker := auth.WithPrincipal(t.Context(), auth.Principal{Kind: auth.KindUser, ID: r.scalar(t, `SELECT requested_by_id::text FROM agent_requests WHERE id = $1::uuid`, run), Role: auth.RoleEditor})
	for name, tc := range map[string]struct {
		ctx   context.Context
		named bool
	}{"an Admin": {admin, true}, "the Editor who asked": {asker, true}, "an Editor who did not ask": {editor, false}, "no caller": {t.Context(), false}} {
		t.Run(name, func(t *testing.T) {
			err := g.MoveObservation(tc.ctx, r.d.Queries(), r.uuid(t, id), general)
			if !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("moving it to General: err = %v, want the restricted answer refusal", err)
			}
			if strings.Contains(err.Error(), named) != tc.named {
				t.Fatalf("message = %q, names the answer = %v, want %v", err.Error(), !tc.named, tc.named)
			}
		})
	}
}
