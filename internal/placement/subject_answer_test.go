package placement

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestAnAnswerAboutWhatSitsInAnAdminsOnlyFolderIsPlacedOnlyWhereOnlyAdminsReadUntilItMovesOut(t *testing.T) {
	r := newHoldersRig(t)
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	subjects := map[string]struct {
		column, table string
		add           func(t *testing.T, folder pgtype.UUID) string
	}{
		"a dashboard":                      {"dashboard_id", "dashboards", func(t *testing.T, f pgtype.UUID) string { return r.dashboard(t, f, readsProm) }},
		"an observation it is asked about": {"subject_observation_id", "observations", r.observation},
		"the observation it files into":    {"observation_id", "observations", r.observation},
	}
	for name, s := range subjects {
		t.Run(name, func(t *testing.T) {
			home := foldertest.AdminsOnly(t, r.d, name)
			subject := s.add(t, home)
			run := r.run(t, false)
			r.d.Exec(t, `UPDATE agent_requests SET `+s.column+` = $2::uuid WHERE id = $1::uuid`, run, subject)
			runID := r.uuid(t, run)
			shared := r.dashboard(t, general, readsProm)

			if err := Answer(t.Context(), r.d.Queries(), runID, general); !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("placing its answer in General: err = %v, want the restricted answer refusal", err)
			}
			if err := AnswerOnDashboard(t.Context(), r.d.Queries(), runID, r.uuid(t, shared)); !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("placing its answer on a dashboard in General: err = %v, want the restricted answer refusal", err)
			}
			if err := Answer(t.Context(), r.d.Queries(), runID, r.personalFolder(t)); !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("placing its answer in My Dashboards: err = %v, want the restricted answer refusal", err)
			}
			for where, folder := range map[string]pgtype.UUID{"the subject's own folder": home, "another folder only Admins read": foldertest.AdminsOnly(t, r.d, name+" too")} {
				if err := Answer(t.Context(), r.d.Queries(), runID, folder); err != nil {
					t.Fatalf("placing its answer in %s: %v", where, err)
				}
			}

			r.d.Exec(t, `UPDATE `+s.table+` SET folder_id = $2 WHERE id = $1::uuid`, subject, general)
			if err := Answer(t.Context(), r.d.Queries(), runID, general); err != nil {
				t.Fatalf("placing its answer in General after the subject moved there: %v", err)
			}
			if err := AnswerOnDashboard(t.Context(), r.d.Queries(), runID, r.uuid(t, shared)); err != nil {
				t.Fatalf("placing its answer on a shared dashboard after the subject moved out: %v", err)
			}
		})
	}
}
