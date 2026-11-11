package webhooks

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/sealed"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const (
	receiverHost = "hooks.example.test"
	receiverURL  = "https://" + receiverHost + "/in"
	waitTimeout  = 10 * time.Second
)

var (
	testKey    = sealed.Key("0123456789abcdef0123456789abcdef")
	adminOne   = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Root", Role: auth.RoleAdmin}
	adminTwo   = auth.Principal{Kind: auth.KindUser, ID: "66666666-6666-4666-8666-666666666666", Name: "Second", Role: auth.RoleAdmin}
	editorOnly = auth.Principal{Kind: auth.KindUser, ID: "77777777-7777-4777-8777-777777777777", Name: "Edie", Role: auth.RoleEditor}
	someone    = actor.Actor{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Eve"}
)

type principals struct {
	mu   sync.Mutex
	byID map[string]auth.Principal
}

func (p *principals) set(pr auth.Principal) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.byID[pr.ID] = pr
}

func (p *principals) Principal(_ context.Context, _ auth.PrincipalKind, id string) (auth.Principal, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pr, ok := p.byID[id]
	if !ok {
		return auth.Principal{}, auth.ErrPrincipalGone
	}
	return pr, nil
}

type call struct {
	URL    string
	Header http.Header
	Body   []byte
}

func (c call) deliveryID() string { return c.Header.Get("X-SlashObserve-Delivery") }

type fakeSender struct {
	mu    sync.Mutex
	calls []call
	reply func(n int, c call) (egress.SendResult, error)
}

func (f *fakeSender) Send(_ context.Context, req egress.SendRequest) (egress.SendResult, error) {
	f.mu.Lock()
	c := call{URL: req.URL, Header: req.Header.Clone(), Body: bytes.Clone(req.Body)}
	f.calls = append(f.calls, c)
	n := len(f.calls) - 1
	reply := f.reply
	f.mu.Unlock()
	if reply == nil {
		return egress.SendResult{Status: http.StatusNoContent}, nil
	}
	return reply(n, c)
}

func (f *fakeSender) all() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

func (f *fakeSender) script(reply func(n int, c call) (egress.SendResult, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reply = reply
}

type allowList struct {
	mu   sync.Mutex
	list domains.List
}

func (a *allowList) Allowed(context.Context) (domains.List, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.list, nil
}

func (a *allowList) set(t *testing.T, entries ...string) {
	t.Helper()
	list := domains.List{}
	for _, e := range entries {
		p, err := domains.ParsePattern(e)
		if err != nil {
			t.Fatal(err)
		}
		list = append(list, p)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.list = list
}

type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

type rig struct {
	db         *dbtest.Database
	store      *activity.Store
	principals *principals
	sender     *fakeSender
	allow      *allowList
	logs       *logBuffer
	svc        *Service
	worker     *Worker
	key        sealed.Key
}

func newRig(t *testing.T) *rig {
	t.Helper()
	return newRigWithKey(t, testKey)
}

func newRigWithKey(t *testing.T, key sealed.Key) *rig {
	t.Helper()
	database := dbtest.New(t)
	r := &rig{
		db:         database,
		store:      activity.NewStore(database.DB, folders.NewAccess(database.DB)),
		principals: &principals{byID: map[string]auth.Principal{}},
		sender:     &fakeSender{},
		allow:      &allowList{},
		logs:       &logBuffer{},
		key:        key,
	}
	r.allow.set(t, receiverHost)
	for _, p := range []auth.Principal{adminOne, adminTwo, editorOnly} {
		r.principals.set(p)
	}
	logger := slog.New(slog.NewJSONHandler(r.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r.svc = NewService(Deps{DB: database.DB, Activity: r.store, Domains: r.allow, Sender: r.sender, Key: key, Logger: logger})
	r.worker = r.workerAs(logger, "instance-a")
	return r
}

func (r *rig) workerAs(logger *slog.Logger, instance string) *Worker {
	return NewWorker(WorkerDeps{
		DB: r.db.DB, Activity: r.store, Principals: r.principals, Sender: r.sender, Key: r.key,
		InstanceID: instance, Logger: logger,
	})
}

func (r *rig) create(t *testing.T, types ...string) Created {
	t.Helper()
	if len(types) == 0 {
		types = activity.Types()
	}
	created, err := r.svc.Create(t.Context(), adminOne, Input{Name: "Pipeline", URL: receiverURL, EventTypes: types})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func (r *rig) uuid(t *testing.T, id string) pgtype.UUID {
	t.Helper()
	parsed, err := db.ParseUUID(id)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

var targets = func() func() pgtype.UUID {
	var n atomic.Int64
	return func() pgtype.UUID {
		id, err := db.ParseUUID(fmt.Sprintf("00000000-0000-4000-8000-%012d", n.Add(1)))
		if err != nil {
			panic(err)
		}
		return id
	}
}()

func (r *rig) append(t *testing.T, typ string) int64 {
	t.Helper()
	return r.appendFor(t, typ, pgtype.UUID{})
}

func (r *rig) appendFor(t *testing.T, typ string, dashboard pgtype.UUID) int64 {
	t.Helper()
	return r.appendOwned(t, typ, activity.OnDashboard(dashboard))
}

func (r *rig) appendOwned(t *testing.T, typ string, owner activity.Owner) int64 {
	t.Helper()
	var seq int64
	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		var err error
		seq, err = activity.Append(t.Context(), q, activity.Entry{Type: typ, TargetID: targets(), Owner: owner, Actor: someone}, nil)
		return err
	})
	if err != nil {
		t.Fatalf("append %s: %v", typ, err)
	}
	return seq
}

func (r *rig) pass(t *testing.T) {
	t.Helper()
	r.worker.pass(t.Context())
}

func (r *rig) row(t *testing.T, id string) dbsqlc.Webhook {
	t.Helper()
	row, err := r.db.Queries().GetWebhookForUpdate(t.Context(), r.uuid(t, id))
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func (r *rig) makeDue(t *testing.T, id string) {
	t.Helper()
	r.db.Exec(t, `UPDATE webhooks SET next_attempt_at = now() - interval '1 second' WHERE id = $1::uuid`, id)
}

func (r *rig) deliveries(t *testing.T, id string) []dbsqlc.WebhookDelivery {
	t.Helper()
	rows, err := r.db.Queries().ListWebhookDeliveries(t.Context(), dbsqlc.ListWebhookDeliveriesParams{WebhookID: r.uuid(t, id), RowLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func seqsSent(t *testing.T, calls []call) []int64 {
	t.Helper()
	out := make([]int64, 0, len(calls))
	for _, c := range calls {
		parts := strings.Split(c.deliveryID(), ".")
		seq, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
		if err != nil {
			t.Fatalf("delivery id %q: %v", c.deliveryID(), err)
		}
		out = append(out, seq)
	}
	return out
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }
