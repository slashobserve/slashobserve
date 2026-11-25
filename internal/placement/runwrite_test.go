package placement

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func TestPlacementOfAWriteWithoutARunReadsNothing(t *testing.T) {
	folder := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	if err := RunWrite(t.Context(), nil, actor.Actor{}, folder); err != nil {
		t.Fatalf("a write outside a run: %v", err)
	}
	if err := RunWriteOnDashboard(t.Context(), nil, actor.Actor{}, folder); err != nil {
		t.Fatalf("a dashboard write outside a run: %v", err)
	}
}

func TestPlacementOfAWriteRefusesARunIdThatIsNotAUUID(t *testing.T) {
	folder := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	if err := RunWrite(t.Context(), nil, actor.Actor{RunID: "not-a-run"}, folder); err == nil {
		t.Fatal("a malformed run id was accepted")
	}
}

func TestPlacementRunOfReadsTheActorsRun(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	got, err := RunOf(actor.Actor{RunID: id})
	if err != nil || !got.Valid || got.String() != id {
		t.Fatalf("RunOf = %v, %v", got, err)
	}
	if got, err := RunOf(actor.Actor{}); err != nil || got.Valid {
		t.Fatalf("RunOf with no run = %v, %v", got, err)
	}
}

func TestPlacementOfAReadRestrictedRunsWriteStaysWhereOnlyAdminsRead(t *testing.T) {
	r := newHoldersRig(t)
	general := r.uuid(t, r.scalar(t, `SELECT id::text FROM folders WHERE general`))
	adminsOnly := foldertest.AdminsOnly(t, r.d, "Salaries")
	sharedDashboard := r.uuid(t, r.dashboard(t, general, readsProm))
	hiddenDashboard := r.uuid(t, r.dashboard(t, adminsOnly, readsProm))
	cases := []struct {
		name       string
		restricted bool
		folder     pgtype.UUID
		dashboard  pgtype.UUID
		refused    bool
	}{
		{"a run that read Admin-only data, writing where others read", true, general, sharedDashboard, true},
		{"a run that read Admin-only data, writing where only Admins read", true, adminsOnly, hiddenDashboard, false},
		{"a run that read nothing restricted, writing where others read", false, general, sharedDashboard, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			who := actor.Actor{RunID: r.run(t, c.restricted)}
			if err := RunWrite(t.Context(), r.d.Queries(), who, c.folder); errors.Is(err, folders.ErrRestrictedAnswer) != c.refused || err != nil && !c.refused {
				t.Fatalf("RunWrite = %v, refused %v", err, c.refused)
			}
			if err := RunWriteOnDashboard(t.Context(), r.d.Queries(), who, c.dashboard); errors.Is(err, folders.ErrRestrictedAnswer) != c.refused || err != nil && !c.refused {
				t.Fatalf("RunWriteOnDashboard = %v, refused %v", err, c.refused)
			}
		})
	}
}
