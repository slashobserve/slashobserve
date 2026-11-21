package placement

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestContentHoldingAnAnswerAboutAnotherAdminsOnlySubjectStaysWhereOnlyAdminsReadUntilTheSubjectMovesOut(t *testing.T) {
	r := newHoldersRig(t)
	g := New(nil)
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	for name, h := range answerHolders {
		t.Run(name, func(t *testing.T) {
			folder := foldertest.AdminsOnly(t, r.d, name)
			subject := r.observation(t, foldertest.AdminsOnly(t, r.d, name+" subject"))
			run := r.run(t, false)
			id := r.uuid(t, h.add(t, r, folder, run))
			if r.scalar(t, `SELECT (subject_observation_id IS NOT NULL)::text FROM agent_requests WHERE id = $1::uuid`, run) == "true" {
				t.Skip("an ask about an observation has no other subject")
			}
			r.d.Exec(t, `UPDATE agent_requests SET observation_id = $2::uuid WHERE id = $1::uuid`, run, subject)

			if err := g.OpeningGuard()(t.Context(), r.d.Queries(), dbsqlc.Folder{ID: folder}); !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("opening the folder: err = %v, want the restricted answer refusal", err)
			}
			if err := h.move(g, r, t, id, general); !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("moving it to General: err = %v, want the restricted answer refusal", err)
			}
			if err := h.move(g, r, t, id, foldertest.AdminsOnly(t, r.d, name+" too")); err != nil {
				t.Fatalf("moving it to another folder only Admins read: %v", err)
			}

			r.d.Exec(t, `UPDATE observations SET folder_id = $2 WHERE id = $1::uuid`, subject, general)
			if err := h.move(g, r, t, id, general); err != nil {
				t.Fatalf("moving it to General after the subject moved out: %v", err)
			}
		})
	}
}

func TestAnAnswerAboutAnAdminsOnlyDashboardSavedAsAnObservationCannotBeMovedCopiedOrOpenedOut(t *testing.T) {
	r := newHoldersRig(t)
	g := New(nil)
	q := r.d.Queries()
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	home := foldertest.AdminsOnly(t, r.d, "Leadership")
	other := foldertest.AdminsOnly(t, r.d, "Board")
	subject := r.uuid(t, r.dashboard(t, home, readsProm))
	run := r.run(t, false)
	r.ended(t, run, `dashboard_id = $2::uuid`, subject.String())

	saved := r.uuid(t, r.observation(t, home))
	r.d.Exec(t, `INSERT INTO observation_blocks (observation_id, kind, position, body, created_by_kind, created_by_id, created_by_name, run_id) VALUES ($1::uuid, 'text', 0, '{"markdown":"pending layoffs: payments team"}', 'user', gen_random_uuid(), 'Ada', $2::uuid)`, saved, run)
	pinned := r.uuid(t, r.dashboard(t, other, readsProm))
	r.d.Exec(t, `WITH n AS (INSERT INTO annotations (dashboard_id, time_from, text) VALUES ($1::uuid, now(), 'pending layoffs') RETURNING id)
INSERT INTO findings (agent_request_id, dashboard_id, annotation_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
SELECT $2::uuid, $1::uuid, n.id, 'Layoffs', 'pending layoffs: payments team', now() - interval '1 hour', now(), 'user', gen_random_uuid(), 'Ada' FROM n`, pinned, run)

	if err := g.MoveObservation(t.Context(), q, saved, general); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("moving the saved observation to General: err = %v, want the restricted answer refusal", err)
	}
	if _, err := Copy(t.Context(), q, Copied{Kind: folders.ContentObservation, ID: saved}, general); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("copying the saved observation into General: err = %v, want the restricted answer refusal", err)
	}
	if err := g.OpeningGuard()(t.Context(), q, dbsqlc.Folder{ID: other}); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("opening the folder of the dashboard it is pinned on: err = %v, want the restricted answer refusal", err)
	}
	if err := g.MoveDashboard(t.Context(), q, pinned, general); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("moving the dashboard it is pinned on to General: err = %v, want the restricted answer refusal", err)
	}
	if _, err := Copy(t.Context(), q, Copied{Kind: folders.ContentDashboard, ID: pinned}, general); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("copying the dashboard it is pinned on into General: err = %v, want the restricted answer refusal", err)
	}

	if err := g.MoveDashboard(t.Context(), q, subject, general); err != nil {
		t.Fatalf("moving the dashboard the answer is about to General: %v", err)
	}
	if err := g.MoveObservation(t.Context(), q, saved, general); err != nil {
		t.Fatalf("moving the saved observation after its subject moved out: %v", err)
	}
	if err := g.MoveDashboard(t.Context(), q, pinned, general); err != nil {
		t.Fatalf("moving the dashboard it is pinned on after its subject moved out: %v", err)
	}
}
