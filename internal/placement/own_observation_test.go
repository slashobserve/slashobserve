package placement

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestAnInvestigationsOwnObservationInAnAdminsOnlyFolderKeepsItsAnswerButASavedAnswersObservationDoesNot(t *testing.T) {
	r := newHoldersRig(t)
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	for name, investigation := range map[string]bool{"an investigation": true, "a saved answer": false} {
		t.Run(name, func(t *testing.T) {
			own := r.observation(t, foldertest.AdminsOnly(t, r.d, name))
			run := r.run(t, false)
			set := `observation_id = $2::uuid, investigation = false`
			if investigation {
				set = `observation_id = $2::uuid, investigation = true`
			}
			r.ended(t, run, set, own)
			r.d.Exec(t, `UPDATE observations SET run_id = $2::uuid WHERE id = $1::uuid`, own, run)

			err := Answer(t.Context(), r.d.Queries(), r.uuid(t, run), general)
			if investigation && !errors.Is(err, folders.ErrRestrictedAnswer) {
				t.Fatalf("placing the answer in General: err = %v, want the restricted answer refusal", err)
			}
			if !investigation && err != nil {
				t.Fatalf("placing the answer in General: %v", err)
			}
		})
	}
}
