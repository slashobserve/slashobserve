package activity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/origin"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const (
	payrollGroup = "payroll-team"
	ownerUser    = "77777777-7777-4777-8777-777777777777"
	waitTimeout  = 10 * time.Second
)

var (
	editorActor = actor.Actor{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Eve"}
	plain       = auth.Principal{Kind: auth.KindUser, ID: "88888888-8888-4888-8888-888888888888", Name: "Una", Role: auth.RoleViewer}
	reader      = auth.Principal{Kind: auth.KindUser, ID: "33333333-3333-4333-8333-333333333333", Name: "Vic", Role: auth.RoleViewer}
	payroll     = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Pat", Role: auth.RoleViewer, Groups: []string{payrollGroup}}
	admin       = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Root", Role: auth.RoleAdmin}
)

type rig struct {
	db    *dbtest.Database
	store *Store
}

func newRig(t *testing.T) rig {
	t.Helper()
	d := dbtest.New(t)
	return rig{db: d, store: NewStore(d.DB, folders.NewAccess(d.DB))}
}

func (r rig) dashboardIn(t *testing.T, folder pgtype.UUID) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ('Checkout', 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	if folder.Valid {
		foldertest.Move(t, r.db, id, folder)
	}
	return id
}

func (r rig) openFolder(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	f, err := r.db.Queries().CreateFolder(t.Context(), dbsqlc.CreateFolderParams{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return f.ID
}

var targets atomic.Int64

func newTarget() pgtype.UUID {
	id, err := db.ParseUUID(fmt.Sprintf("00000000-0000-4000-8000-%012d", targets.Add(1)))
	if err != nil {
		panic(err)
	}
	return id
}

func (r rig) append(t *testing.T, e Entry) int64 {
	t.Helper()
	seq, err := r.tryAppend(t.Context(), e)
	if err != nil {
		t.Fatalf("append %s: %v", e.Type, err)
	}
	return seq
}

func (r rig) tryAppend(ctx context.Context, e Entry) (int64, error) {
	if !e.TargetID.Valid {
		e.TargetID = newTarget()
	}
	if e.Actor.ID == "" {
		e.Actor = editorActor
	}
	var seq int64
	err := r.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		seq, err = Append(ctx, q, e, nil)
		return err
	})
	return seq, err
}

func (r rig) folderOf(t *testing.T, seq int64) string {
	t.Helper()
	var got string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT folder_id::text FROM activity WHERE seq = $1`, seq).Scan(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func (r rig) waitForAdvisoryLockWait(t *testing.T) {
	t.Helper()
	conn := r.db.SQLDB(t)
	deadline := time.Now().Add(waitTimeout)
	for {
		var waiting int
		if err := conn.QueryRowContext(t.Context(), `SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no transaction is waiting on the dashboard activity lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func finished[T any](t *testing.T, c <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(waitTimeout):
		t.Fatalf("%s did not finish", what)
		panic("unreachable")
	}
}

func (r rig) audience(t *testing.T, p auth.Principal) Audience {
	t.Helper()
	a, err := r.store.Audience(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func seqsOf(events []Event) []int64 {
	out := make([]int64, len(events))
	for i, e := range events {
		out[i] = e.Seq
	}
	return out
}

func sorted(s []int64) []int64 {
	out := slices.Clone(s)
	slices.Sort(out)
	return out
}

type heldTx struct {
	release chan struct{}
	done    chan error
}

func (r rig) hold(ctx context.Context, body func(q *dbsqlc.Queries) error, waitFor <-chan struct{}) *heldTx {
	h := &heldTx{release: make(chan struct{}), done: make(chan error, 1)}
	go func() {
		h.done <- r.db.InTx(ctx, func(q *dbsqlc.Queries) error {
			if waitFor != nil {
				<-waitFor
			}
			if err := body(q); err != nil {
				return err
			}
			<-h.release
			return nil
		})
	}()
	return h
}

func (h *heldTx) releaseOnCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		select {
		case <-h.release:
		default:
			close(h.release)
		}
	})
}

func (h *heldTx) commit(t *testing.T) {
	t.Helper()
	close(h.release)
	select {
	case err := <-h.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("held transaction did not commit")
	}
}

func signalled(t *testing.T, c <-chan struct{}) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for the other transaction")
	}
}

func (r rig) next(t *testing.T, a Audience, from Position, limit int) ([]Event, Position, bool) {
	t.Helper()
	got, to, held, err := r.store.Next(t.Context(), a, Filter{}, from, limit)
	if err != nil {
		t.Fatal(err)
	}
	return got, to, held
}

func TestReadsStopAtTheHorizon(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	all := r.audience(t, admin)
	r.append(t, Entry{Type: TypeFindingCreated})
	head, _, err := r.store.Start(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	appended := make(chan struct{})
	var aSeq int64
	a := r.hold(ctx, func(q *dbsqlc.Queries) error {
		var err error
		aSeq, err = Append(ctx, q, Entry{Type: TypeFindingCreated, TargetID: newTarget(), Actor: editorActor}, nil)
		close(appended)
		return err
	}, nil)
	signalled(t, appended)
	bSeq := r.append(t, Entry{Type: TypeFindingCreated})

	got, to, held := r.next(t, all, head, 10)
	if len(got) != 0 || to != head || !held {
		t.Fatalf("while a transaction is open: events %v, position moved %v, held %v", seqsOf(got), to != head, held)
	}

	a.commit(t)
	got, _, held = r.next(t, all, head, 10)
	if want := []int64{aSeq, bSeq}; !slices.Equal(seqsOf(got), want) || held {
		t.Fatalf("after the commit: events %v (held %v), want %v", seqsOf(got), held, want)
	}
}

func TestAnOlderTransactionThatAppendsLaterIsNotSkipped(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	all := r.audience(t, admin)
	head, _, err := r.store.Start(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	rHasXid := make(chan struct{})
	fAppended := make(chan struct{})
	var rSeq, fSeq int64
	older := make(chan error, 1)
	go func() {
		older <- r.db.InTx(ctx, func(q *dbsqlc.Queries) error {
			if _, err := q.CreateFolder(ctx, dbsqlc.CreateFolderParams{Title: "assigns a transaction id"}); err != nil {
				return err
			}
			close(rHasXid)
			select {
			case <-fAppended:
			case <-time.After(waitTimeout):
				return errors.New("the later transaction never appended")
			}
			var err error
			rSeq, err = Append(ctx, q, Entry{Type: TypeFindingCreated, TargetID: newTarget(), Actor: editorActor}, nil)
			return err
		})
	}()
	f := r.hold(ctx, func(q *dbsqlc.Queries) error {
		var err error
		fSeq, err = Append(ctx, q, Entry{Type: TypeFindingCreated, TargetID: newTarget(), Actor: editorActor}, nil)
		close(fAppended)
		return err
	}, rHasXid)
	select {
	case err := <-older:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("the older transaction did not commit")
	}
	if fSeq >= rSeq {
		t.Fatalf("setup: the later transaction's seq %d is not below the older one's %d", fSeq, rSeq)
	}

	got, rPosition, _ := r.next(t, all, head, 10)
	if want := []int64{rSeq}; !slices.Equal(seqsOf(got), want) {
		t.Fatalf("while the later transaction is open: %v, want %v", seqsOf(got), want)
	}

	f.commit(t)
	got, _, _ = r.next(t, all, rPosition, 10)
	if want := []int64{fSeq}; !slices.Equal(seqsOf(got), want) {
		t.Fatalf("after the later transaction commits: %v, want %v", seqsOf(got), want)
	}
}

func TestAudience(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	general := r.dashboardIn(t, pgtype.UUID{})
	open := r.dashboardIn(t, r.openFolder(t, "Open"))
	restricted := r.dashboardIn(t, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))
	r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', 'owner', 'owner@example.com', 'Owner')`, ownerUser)
	var personalFolder pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(ctx, `SELECT id FROM folders WHERE owner_id = $1::uuid`, ownerUser).Scan(&personalFolder); err != nil {
		t.Fatal(err)
	}
	personal := r.dashboardIn(t, personalFolder)

	inGeneral := r.append(t, Entry{Type: TypeDashboardCreated, DashboardID: general})
	inOpen := r.append(t, Entry{Type: TypeDashboardCreated, DashboardID: open})
	inRestricted := r.append(t, Entry{Type: TypeDashboardCreated, DashboardID: restricted})
	inPersonal := r.append(t, Entry{Type: TypeDashboardCreated, DashboardID: personal})
	globalAnnotation := r.append(t, Entry{Type: TypeAnnotationCreated, Global: true})
	readersOwn := r.append(t, Entry{Type: TypeFindingCreated, Actor: actor.Of(reader, origin.Origin{}, "")})
	someoneElses := r.append(t, Entry{Type: TypeFindingCreated})

	tests := []struct {
		name string
		who  auth.Principal
		want []int64
	}{
		{"a viewer sees open folders and General", plain, []int64{inGeneral, inOpen, globalAnnotation}},
		{"a group member also sees the restricted folder", payroll, []int64{inGeneral, inOpen, inRestricted, globalAnnotation}},
		{"an actor sees its own row with no folder", reader, []int64{inGeneral, inOpen, globalAnnotation, readersOwn}},
		{"an admin sees every row with no folder but no personal folder of another", admin, []int64{inGeneral, inOpen, inRestricted, globalAnnotation, readersOwn, someoneElses}},
		{"the owner sees their personal folder", auth.Principal{Kind: auth.KindUser, ID: ownerUser, Name: "Owner", Role: auth.RoleViewer}, []int64{inGeneral, inOpen, inPersonal, globalAnnotation}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := r.store.List(ctx, r.audience(t, tt.who), Filter{}, "", 100)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(sorted(seqsOf(got)), sorted(tt.want)) {
				t.Fatalf("got %v, want %v", sorted(seqsOf(got)), sorted(tt.want))
			}
		})
	}
}

func TestAppendTakesTheDashboardsFolder(t *testing.T) {
	r := newRig(t)
	folder := r.openFolder(t, "Open")
	dashboard := r.dashboardIn(t, folder)

	seq := r.append(t, Entry{Type: TypeDashboardVersionCreated, DashboardID: dashboard})

	var got string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT folder_id::text FROM activity WHERE seq = $1`, seq).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != folder.String() {
		t.Fatalf("folder = %s, want %s", got, folder.String())
	}
}

func TestAnAppendTakesNoDashboardRowLock(t *testing.T) {
	cases := []struct {
		name   string
		hold   string
		finish func(t *testing.T, tx *sql.Tx, dashboard pgtype.UUID)
	}{
		{
			name: "a row lock held for a delete",
			hold: `SELECT 1 FROM dashboards WHERE id = $1 FOR UPDATE`,
			finish: func(t *testing.T, tx *sql.Tx, dashboard pgtype.UUID) {
				if _, err := tx.ExecContext(t.Context(), `DELETE FROM dashboards WHERE id = $1`, dashboard.String()); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "an uncommitted version bump",
			hold: `UPDATE dashboards SET latest_version = latest_version + 1 WHERE id = $1`,
			finish: func(t *testing.T, tx *sql.Tx, _ pgtype.UUID) {
				if err := tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			folder := r.openFolder(t, "Open")
			dashboard := r.dashboardIn(t, folder)
			tx, err := r.db.SQLDB(t).BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.ExecContext(t.Context(), tc.hold, dashboard.String()); err != nil {
				t.Fatal(err)
			}

			type appended struct {
				seq int64
				err error
			}
			done := make(chan appended, 1)
			go func() {
				seq, err := r.tryAppend(t.Context(), Entry{Type: TypeDashboardVersionCreated, DashboardID: dashboard})
				done <- appended{seq, err}
			}()
			got := finished(t, done, "the append")
			if got.err != nil {
				t.Fatal(got.err)
			}

			tc.finish(t, tx, dashboard)
			if folderID := r.folderOf(t, got.seq); folderID != folder.String() {
				t.Fatalf("folder = %s, want %s", folderID, folder.String())
			}
		})
	}
}

func TestAnAppendDuringAMoveReadsTheNewFolder(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	from := r.openFolder(t, "From")
	to := r.openFolder(t, "To")
	dashboard := r.dashboardIn(t, from)

	moved := make(chan struct{})
	mover := r.hold(ctx, func(q *dbsqlc.Queries) error {
		if _, err := q.MoveDashboard(ctx, dbsqlc.MoveDashboardParams{ID: dashboard, FolderID: to}); err != nil {
			return err
		}
		if err := MoveDashboard(ctx, q, dashboard, to); err != nil {
			return err
		}
		close(moved)
		return nil
	}, nil)
	mover.releaseOnCleanup(t)
	signalled(t, moved)

	type appended struct {
		seq int64
		err error
	}
	done := make(chan appended, 1)
	go func() {
		seq, err := r.tryAppend(ctx, Entry{Type: TypeDashboardVersionCreated, DashboardID: dashboard})
		done <- appended{seq, err}
	}()
	r.waitForAdvisoryLockWait(t)
	mover.commit(t)

	got := finished(t, done, "the append")
	if got.err != nil {
		t.Fatal(got.err)
	}
	if folderID := r.folderOf(t, got.seq); folderID != to.String() {
		t.Fatalf("folder = %s, want %s", folderID, to.String())
	}
}

func TestAMoveWaitsForAnOpenAppend(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	from := r.openFolder(t, "From")
	to := r.openFolder(t, "To")
	dashboard := r.dashboardIn(t, from)

	var heldSeq int64
	appended := make(chan struct{})
	appender := r.hold(ctx, func(q *dbsqlc.Queries) error {
		var err error
		heldSeq, err = Append(ctx, q, Entry{Type: TypeDashboardVersionCreated, TargetID: newTarget(), DashboardID: dashboard, Actor: editorActor}, nil)
		close(appended)
		return err
	}, nil)
	appender.releaseOnCleanup(t)
	signalled(t, appended)

	move := make(chan error, 1)
	go func() {
		move <- r.db.InTx(ctx, func(q *dbsqlc.Queries) error {
			if _, err := q.MoveDashboard(ctx, dbsqlc.MoveDashboardParams{ID: dashboard, FolderID: to}); err != nil {
				return err
			}
			return MoveDashboard(ctx, q, dashboard, to)
		})
	}()
	r.waitForAdvisoryLockWait(t)
	appender.commit(t)

	if err := finished(t, move, "the move"); err != nil {
		t.Fatal(err)
	}
	if folderID := r.folderOf(t, heldSeq); folderID != to.String() {
		t.Fatalf("held row folder = %s, want %s", folderID, to.String())
	}
}

func TestAppendForAMissingDashboardHasNoFolder(t *testing.T) {
	r := newRig(t)

	seq := r.append(t, Entry{Type: TypeDashboardDeleted, DashboardID: newTarget()})

	var folder pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT folder_id FROM activity WHERE seq = $1`, seq).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	if folder.Valid {
		t.Fatalf("folder = %s, want none", folder.String())
	}
}

func TestAppendRoundTripsTheActorAndData(t *testing.T) {
	r := newRig(t)
	run := newTarget()
	by := actor.Actor{Kind: auth.KindServiceAccount, ID: editorActor.ID, Name: "ci", Origin: origin.Origin{Kind: origin.MCP, Client: "claude-code"}, RunID: run.String()}
	seq := r.append(t, Entry{Type: TypeRunSucceeded, TargetID: run, Actor: by, Data: map[string]any{"result_version": 4}})

	got, _, err := r.store.List(t.Context(), r.audience(t, admin), Filter{}, "", 10)

	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Seq != seq {
		t.Fatalf("got %+v", got)
	}
	e := got[0]
	if e.Type != TypeRunSucceeded || e.TargetKind != "run" || e.TargetID != run.String() || e.DashboardID != "" || e.FolderID != "" {
		t.Errorf("event = %+v", e)
	}
	if e.Actor != by {
		t.Errorf("actor = %+v, want %+v", e.Actor, by)
	}
	var data map[string]int
	if err := json.Unmarshal(e.Data, &data); err != nil || data["result_version"] != 4 {
		t.Errorf("data = %s (%v)", e.Data, err)
	}
	if e.CreatedAt.IsZero() {
		t.Error("created_at is empty")
	}
}

func TestListPagesNewestFirst(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	all := r.audience(t, admin)
	var seqs []int64
	for range 5 {
		seqs = append(seqs, r.append(t, Entry{Type: TypeFindingCreated}))
	}

	first, next, err := r.store.List(ctx, all, Filter{}, "", 2)
	if err != nil || next == "" {
		t.Fatalf("first page: %v, next %q", err, next)
	}
	second, next, err := r.store.List(ctx, all, Filter{}, next, 2)
	if err != nil || next == "" {
		t.Fatalf("second page: %v, next %q", err, next)
	}
	third, next, err := r.store.List(ctx, all, Filter{}, next, 2)
	if err != nil || next != "" {
		t.Fatalf("third page: %v, next %q", err, next)
	}

	pages := [][]int64{seqsOf(first), seqsOf(second), seqsOf(third)}
	want := [][]int64{{seqs[4], seqs[3]}, {seqs[2], seqs[1]}, {seqs[0]}}
	for i := range want {
		if !slices.Equal(pages[i], want[i]) {
			t.Errorf("page %d = %v, want %v", i+1, pages[i], want[i])
		}
	}
	if _, _, err := r.store.List(ctx, all, Filter{}, "garbage", 2); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("garbage cursor: got %v", err)
	}
}

func TestListAppliesTheFilter(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	all := r.audience(t, admin)
	dashboard := r.dashboardIn(t, pgtype.UUID{})
	created := r.append(t, Entry{Type: TypeDashboardCreated, DashboardID: dashboard})
	moved := r.append(t, Entry{Type: TypeDashboardMoved, DashboardID: dashboard})
	other := r.append(t, Entry{Type: TypeFindingCreated})

	tests := []struct {
		name   string
		filter Filter
		want   []int64
	}{
		{"a dashboard", Filter{DashboardID: dashboard}, []int64{moved, created}},
		{"a type", Filter{Types: []string{TypeFindingCreated}}, []int64{other}},
		{"two types", Filter{Types: []string{TypeDashboardMoved, TypeFindingCreated}}, []int64{other, moved}},
		{"a time in the future", Filter{Since: time.Now().Add(time.Hour)}, []int64{}},
		{"a time in the past", Filter{Since: time.Now().Add(-time.Hour)}, []int64{other, moved, created}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := r.store.List(ctx, all, tt.filter, "", 10)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(seqsOf(got), tt.want) {
				t.Fatalf("got %v, want %v", seqsOf(got), tt.want)
			}
		})
	}
}

func TestStartResumesAfterASeqAndResetsForAnUnknownOne(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	all := r.audience(t, admin)

	empty, reset, err := r.store.Start(ctx, "")
	if err != nil || reset {
		t.Fatalf("empty log: %v, reset %v", err, reset)
	}
	var seqs []int64
	for range 3 {
		seqs = append(seqs, r.append(t, Entry{Type: TypeFindingCreated}))
	}
	if got, _, _ := r.next(t, all, empty, 10); !slices.Equal(seqsOf(got), seqs) {
		t.Fatalf("from the empty head: %v, want %v", seqsOf(got), seqs)
	}

	head, reset, err := r.store.Start(ctx, "")
	if err != nil || reset {
		t.Fatalf("head: %v, reset %v", err, reset)
	}
	if got, _, held := r.next(t, all, head, 10); len(got) != 0 || held {
		t.Fatalf("after the head: %v, held %v", seqsOf(got), held)
	}

	resumed, reset, err := r.store.Start(ctx, strconv.FormatInt(seqs[0], 10))
	if err != nil || reset {
		t.Fatalf("resume: %v, reset %v", err, reset)
	}
	if got, _, _ := r.next(t, all, resumed, 10); !slices.Equal(seqsOf(got), seqs[1:]) {
		t.Fatalf("after seq 1: %v, want %v", seqsOf(got), seqs[1:])
	}

	beginning, reset, err := r.store.Start(ctx, "0")
	if err != nil || reset || beginning != (Position{}) {
		t.Fatalf("zero: position %+v, reset %v, err %v", beginning, reset, err)
	}
	if got, _, _ := r.next(t, all, beginning, 10); !slices.Equal(seqsOf(got), seqs) {
		t.Fatalf("from zero: %v, want %v", seqsOf(got), seqs)
	}

	unknown, reset, err := r.store.Start(ctx, "999999")
	if err != nil || !reset || unknown != head {
		t.Fatalf("unknown seq: position %+v, reset %v, err %v", unknown, reset, err)
	}

	for _, bad := range []string{"abc", "-1", "-3", "1.5", " 2"} {
		if _, _, err := r.store.Start(ctx, bad); !errors.Is(err, ErrInvalidLastEventID) {
			t.Errorf("Start(%q): got %v, want ErrInvalidLastEventID", bad, err)
		}
	}
}

func TestNextReportsAFullBatchWithoutHeld(t *testing.T) {
	r := newRig(t)
	all := r.audience(t, admin)
	for range 3 {
		r.append(t, Entry{Type: TypeFindingCreated})
	}

	got, to, held := r.next(t, all, Position{}, 2)

	if len(got) != 2 || held {
		t.Fatalf("got %d events, held %v", len(got), held)
	}
	rest, _, held := r.next(t, all, to, 2)
	if len(rest) != 1 || held {
		t.Fatalf("rest: %d events, held %v", len(rest), held)
	}
}

func TestAFilteredNextFromTheStartAdvancesToTheHead(t *testing.T) {
	r := newRig(t)
	all := r.audience(t, admin)
	for range 3 {
		r.append(t, Entry{Type: TypeFindingCreated})
	}
	head, _, err := r.store.Start(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	onlyMoves := Filter{Types: []string{TypeDashboardMoved}}

	got, to, _, err := r.store.Next(t.Context(), all, onlyMoves, Position{}, 10)

	if err != nil || len(got) != 0 || to != head {
		t.Fatalf("got %v, to %+v, want no rows and %+v, err %v", seqsOf(got), to, head, err)
	}
	moved := r.append(t, Entry{Type: TypeDashboardMoved})
	got, _, _, err = r.store.Next(t.Context(), all, onlyMoves, to, 10)
	if err != nil || !slices.Equal(seqsOf(got), []int64{moved}) {
		t.Fatalf("after the head: %v, want [%d], err %v", seqsOf(got), moved, err)
	}
}

func TestNextReturnsEachEventsPosition(t *testing.T) {
	r := newRig(t)
	all := r.audience(t, admin)
	seqs := []int64{
		r.append(t, Entry{Type: TypeFindingCreated}),
		r.append(t, Entry{Type: TypeFindingCreated}),
		r.append(t, Entry{Type: TypeFindingCreated}),
	}

	got, _, _ := r.next(t, all, Position{}, 10)

	if !slices.Equal(seqsOf(got), seqs) {
		t.Fatalf("got %v, want %v", seqsOf(got), seqs)
	}
	from := Position{}
	for _, e := range got {
		page, to, _ := r.next(t, all, from, 1)
		if len(page) != 1 || e.Position != to || e.Position.Seq() != e.Seq || e.Position.Xid() == 0 {
			t.Fatalf("event %d has position %+v, a one-row page ends at %+v", e.Seq, e.Position, to)
		}
		if At(e.Position.Xid(), e.Position.Seq()) != e.Position {
			t.Fatalf("At does not rebuild position %+v", e.Position)
		}
		from = to
	}
}
