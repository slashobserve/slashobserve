package public

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (r rcaRig) publishedIn(t *testing.T, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	id := r.observation(t, folder)
	if _, err := r.observations.Publish(reading(t.Context()), admin, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAnObservationWithALiveRCALinkCannotMoveIntoAnAdminsOnlyFolderUntilTheLinkIsRevoked(t *testing.T) {
	r := newRCARig(t)
	id := r.published(t)
	link := r.createLink(t, id, 1, CreateRCAInput{})
	locked := foldertest.AdminsOnly(t, r.db, "Payroll")

	_, err := r.observations.Move(reading(t.Context()), admin, id, locked)
	if !folders.LinkedRefusal(err) || !strings.Contains(fmt.Sprint(err), "revoke") {
		t.Fatalf("move = %v, want a refusal that says to revoke the links", err)
	}
	linkID, err := db.ParseUUID(link.Link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.service.DeleteRCALink(reading(t.Context()), admin, id, linkID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.observations.Move(reading(t.Context()), admin, id, locked); err != nil {
		t.Fatalf("move after revoking = %v", err)
	}
}

func TestAFolderHoldingAnObservationWithAnRCALinkCannotBeMadeAdminsOnly(t *testing.T) {
	r := newRCARig(t)
	r.createLink(t, r.published(t), 1, CreateRCAInput{})

	got := makeAdminsOnly(r.foldersAsAdmin(), r.folder)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), `"admins_only_folder"`) {
		t.Fatalf("admins only = %d %s, want 409 admins_only_folder", got.Code, got.Body)
	}
}

func TestNoPublicLinkOpensContentInAFolderThatBecameAdminsOnly(t *testing.T) {
	r := newRCARig(t)
	rca := r.createLink(t, r.published(t), 1, CreateRCAInput{})
	dashboard := r.create(t, CreateInput{Mode: ModeFrozen})
	if _, err := r.service.ResolveRCA(t.Context(), rca.Token); err != nil {
		t.Fatalf("RCA link before = %v", err)
	}
	if _, err := r.service.Resolve(t.Context(), dashboard.Token); err != nil {
		t.Fatalf("dashboard link before = %v", err)
	}

	r.db.Exec(t, `UPDATE folders SET admins_only = true WHERE NOT personal`)

	if _, err := r.service.ResolveRCA(t.Context(), rca.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RCA link after = %v, want %v", err, ErrNotFound)
	}
	if _, err := r.service.Resolve(t.Context(), dashboard.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("dashboard link after = %v, want %v", err, ErrNotFound)
	}
}

func TestAnRCALinkAndAnAdminsOnlySwitchRacingEachOtherNeverBothSucceed(t *testing.T) {
	r := newRCARig(t)
	engine := r.foldersAsAdmin()
	for round := range 12 {
		folder := foldertest.Open(t, r.db, fmt.Sprintf("Team %d", round))
		id := r.publishedIn(t, folder)

		start := make(chan struct{})
		var wg sync.WaitGroup
		var linkErr error
		var switched *httptest.ResponseRecorder
		wg.Go(func() {
			<-start
			_, linkErr = r.service.CreateRCALink(reading(t.Context()), admin, id, 1, CreateRCAInput{})
		})
		wg.Go(func() {
			<-start
			switched = makeAdminsOnly(engine, folder)
		})
		close(start)
		wg.Wait()

		switch {
		case linkErr == nil && switched.Code == http.StatusConflict && strings.Contains(switched.Body.String(), `"admins_only_folder"`):
		case errors.Is(linkErr, ErrRCAAdminsOnly) && switched.Code == http.StatusOK:
		default:
			t.Fatalf("round %d: link = %v, switch = %d %s; want exactly one refused", round, linkErr, switched.Code, switched.Body)
		}
	}
}
