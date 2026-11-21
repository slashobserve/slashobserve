package placement

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestAKeptAnswerIsSavedInTheFolderOfItsFirstAdminsOnlySubject(t *testing.T) {
	r := newHoldersRig(t)
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	cases := map[string]struct {
		set  string
		args func(first, second pgtype.UUID) (string, string)
		want int
	}{
		"a run on a dashboard in General that files into an Admins-only observation": {
			`dashboard_id = $2::uuid, observation_id = $3::uuid`,
			func(first, second pgtype.UUID) (string, string) {
				return r.dashboard(t, general, readsProm), r.observation(t, second)
			}, 2,
		},
		"a run on an Admins-only dashboard that files into another Admins-only observation": {
			`dashboard_id = $2::uuid, observation_id = $3::uuid`,
			func(first, second pgtype.UUID) (string, string) {
				return r.dashboard(t, first, readsProm), r.observation(t, second)
			}, 1,
		},
		"an ask about an Admins-only observation": {
			`subject_observation_id = $2::uuid, observation_id = $3::uuid`,
			func(first, second pgtype.UUID) (string, string) { return r.observation(t, second), "" }, 2,
		},
		"a run about nothing Admins only": {
			`dashboard_id = $2::uuid, observation_id = $3::uuid`,
			func(first, second pgtype.UUID) (string, string) {
				return r.dashboard(t, general, readsProm), r.observation(t, general)
			}, 0,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			first, second := foldertest.AdminsOnly(t, r.d, name+" first"), foldertest.AdminsOnly(t, r.d, name+" second")
			run := r.run(t, false)
			a, b := c.args(first, second)
			var filed any
			if b != "" {
				filed = b
			}
			r.d.Exec(t, `UPDATE agent_requests SET `+c.set+` WHERE id = $1::uuid`, run, a, filed)
			got, err := r.d.Queries().RestrictedAnswerFolder(t.Context(), dbsqlc.RestrictedAnswerFolderParams{RunID: r.uuid(t, run), AuditFolderKey: "no audit folder in this test"})
			if err != nil {
				t.Fatal(err)
			}
			want := map[int]pgtype.UUID{1: first, 2: second}[c.want]
			if got != want {
				t.Fatalf("folder = %v, want %v", got, want)
			}
		})
	}
}
