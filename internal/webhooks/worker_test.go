package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/events"
)

func TestWorkerDeliversMatchingEventsInLogOrderAndNoOthers(t *testing.T) {
	r := newRig(t)
	created := r.create(t, activity.TypeFindingCreated, activity.TypeRunSucceeded)
	first := r.append(t, activity.TypeFindingCreated)
	r.append(t, activity.TypeRunQueued)
	second := r.append(t, activity.TypeRunSucceeded)
	third := r.append(t, activity.TypeFindingCreated)

	r.pass(t)

	calls := r.sender.all()
	if got, want := seqsSent(t, calls), []int64{first, second, third}; !slices.Equal(got, want) {
		t.Fatalf("sent %v, want %v", got, want)
	}
	for _, c := range calls {
		if c.URL != receiverURL || c.Header.Get("X-SlashObserve-Signature") == "" {
			t.Fatalf("call %+v", c)
		}
	}
	for _, d := range r.deliveries(t, created.Webhook.ID) {
		if d.Outcome != outcomeDelivered || !d.StatusCode.Valid || d.StatusCode.Int32 != 204 {
			t.Fatalf("delivery %+v", d)
		}
	}
	r.pass(t)
	if got := len(r.sender.all()); got != 3 {
		t.Fatalf("a second pass sent %d events in total, want 3", got)
	}
}

func TestWorkerSignsEachAttemptWithTheStoredSecret(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	r.append(t, activity.TypeFindingCreated)

	r.pass(t)

	c := r.sender.all()[0]
	parts := strings.Split(c.Header.Get("X-SlashObserve-Signature"), ",")
	unix, err := strconv.ParseInt(strings.TrimPrefix(parts[0], "t="), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if got := signature(created.Secret, time.Unix(unix, 0), c.Body); got != c.Header.Get("X-SlashObserve-Signature") {
		t.Fatalf("signature %q does not verify with the secret", c.Header.Get("X-SlashObserve-Signature"))
	}
}

func TestAFailureHoldsLaterEventsBackUntilTheRetryIsDue(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	first := r.append(t, activity.TypeFindingCreated)
	second := r.append(t, activity.TypeFindingCreated)
	r.sender.script(func(n int, _ call) (egress.SendResult, error) {
		if n == 0 {
			return egress.SendResult{Status: http.StatusInternalServerError}, nil
		}
		return egress.SendResult{Status: http.StatusOK}, nil
	})

	r.pass(t)

	if got, want := seqsSent(t, r.sender.all()), []int64{first}; !slices.Equal(got, want) {
		t.Fatalf("after the failure sent %v, want only %v", got, want)
	}
	row := r.row(t, created.Webhook.ID)
	if row.ConsecutiveFailures != 1 || !row.NextAttemptAt.Time.After(time.Now()) || row.DisabledAt.Valid || row.LeaseID.Valid {
		t.Fatalf("webhook after a failure %+v", row)
	}
	r.pass(t)
	if got := len(r.sender.all()); got != 1 {
		t.Fatalf("a pass before the retry is due sent %d events in total, want 1", got)
	}

	r.makeDue(t, created.Webhook.ID)
	r.pass(t)

	if got, want := seqsSent(t, r.sender.all()), []int64{first, first, second}; !slices.Equal(got, want) {
		t.Fatalf("after the retry sent %v, want %v", got, want)
	}
	if again := r.sender.all(); again[0].deliveryID() != again[1].deliveryID() {
		t.Fatalf("the retry used delivery id %q, the first attempt %q", again[1].deliveryID(), again[0].deliveryID())
	}
	if row := r.row(t, created.Webhook.ID); row.ConsecutiveFailures != 0 {
		t.Fatalf("failures after success = %d", row.ConsecutiveFailures)
	}
	outcomes := []string{}
	for _, d := range r.deliveries(t, created.Webhook.ID) {
		outcomes = append(outcomes, d.Outcome)
	}
	if want := []string{outcomeDelivered, outcomeDelivered, outcomeFailed}; !slices.Equal(outcomes, want) {
		t.Fatalf("attempt outcomes newest first %v, want %v", outcomes, want)
	}
}

func TestARateLimitedReceiverSetsTheNextAttemptFromRetryAfter(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	r.append(t, activity.TypeFindingCreated)
	r.sender.script(func(int, call) (egress.SendResult, error) {
		return egress.SendResult{Status: http.StatusTooManyRequests, RetryAfter: 10 * time.Minute}, nil
	})

	r.pass(t)

	wait := time.Until(r.row(t, created.Webhook.ID).NextAttemptAt.Time)
	if wait < 9*time.Minute || wait > 10*time.Minute+5*time.Second {
		t.Fatalf("next attempt in %v, want about 10 minutes", wait)
	}
}

func TestA410DisablesTheWebhookAsGone(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	r.append(t, activity.TypeFindingCreated)
	r.sender.script(func(int, call) (egress.SendResult, error) { return egress.SendResult{Status: http.StatusGone}, nil })

	r.pass(t)

	row := r.row(t, created.Webhook.ID)
	if !row.DisabledAt.Valid || row.DisabledReason.String != ReasonGone || row.LeaseID.Valid {
		t.Fatalf("webhook after a 410 %+v", row)
	}
	r.makeDue(t, created.Webhook.ID)
	r.pass(t)
	if got := len(r.sender.all()); got != 1 {
		t.Fatalf("a disabled webhook was sent to again: %d calls", got)
	}
}

func TestTheFiftiethFailureInARowDisablesTheWebhook(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	r.append(t, activity.TypeFindingCreated)
	r.sender.script(func(int, call) (egress.SendResult, error) { return egress.SendResult{Status: 500}, nil })
	r.db.Exec(t, `UPDATE webhooks SET consecutive_failures = 48 WHERE id = $1::uuid`, created.Webhook.ID)

	r.pass(t)
	if row := r.row(t, created.Webhook.ID); row.DisabledAt.Valid || row.ConsecutiveFailures != 49 {
		t.Fatalf("after the 49th failure %+v", row)
	}
	r.makeDue(t, created.Webhook.ID)
	r.pass(t)

	row := r.row(t, created.Webhook.ID)
	if !row.DisabledAt.Valid || row.DisabledReason.String != ReasonFailing || row.ConsecutiveFailures != 50 {
		t.Fatalf("after the 50th failure %+v", row)
	}
}

func TestATooLargeEventIsSkippedAndLaterOnesStillGo(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	first := r.append(t, activity.TypeFindingCreated)
	second := r.append(t, activity.TypeFindingCreated)
	r.sender.script(func(n int, _ call) (egress.SendResult, error) {
		if n == 0 {
			return egress.SendResult{}, &egress.HostError{Host: receiverHost, Err: egress.ErrTooLarge}
		}
		return egress.SendResult{Status: 200}, nil
	})

	r.pass(t)

	if got, want := seqsSent(t, r.sender.all()), []int64{first, second}; !slices.Equal(got, want) {
		t.Fatalf("sent %v, want %v", got, want)
	}
	rows := r.deliveries(t, created.Webhook.ID)
	if len(rows) != 2 || rows[1].Outcome != outcomeSkipped || rows[1].ErrorCode.String != errorTooLarge || rows[0].Outcome != outcomeDelivered {
		t.Fatalf("attempts %+v", rows)
	}
	if row := r.row(t, created.Webhook.ID); row.ConsecutiveFailures != 0 || row.DisabledAt.Valid {
		t.Fatalf("webhook after a skip %+v", row)
	}
}

func TestAnOwnerDemotedToEditorDisablesTheWebhook(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	r.append(t, activity.TypeFindingCreated)
	demoted := adminOne
	demoted.Role = auth.RoleEditor
	r.principals.set(demoted)

	r.pass(t)

	row := r.row(t, created.Webhook.ID)
	if !row.DisabledAt.Valid || row.DisabledReason.String != ReasonOwner || len(r.sender.all()) != 0 {
		t.Fatalf("webhook %+v, calls %d", row, len(r.sender.all()))
	}
}

func TestAnOwnerWhoIsGoneDisablesTheWebhook(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	r.append(t, activity.TypeFindingCreated)
	delete(r.principals.byID, adminOne.ID)

	r.pass(t)

	if row := r.row(t, created.Webhook.ID); row.DisabledReason.String != ReasonOwner {
		t.Fatalf("webhook %+v", row)
	}
}

func TestAnUnreadableSecretDisablesTheWebhook(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	r.append(t, activity.TypeFindingCreated)
	r.worker = r.workerAsKey(t, []byte("another key of thirty-two bytes!"))

	r.pass(t)

	row := r.row(t, created.Webhook.ID)
	if row.DisabledReason.String != ReasonSecretUnreadable || len(r.sender.all()) != 0 {
		t.Fatalf("webhook %+v, calls %d", row, len(r.sender.all()))
	}
}

func (r *rig) workerAsKey(t *testing.T, key []byte) *Worker {
	t.Helper()
	other := *r
	other.key = key
	return other.workerAs(discard(), "instance-key")
}

func TestAnEventInAnotherPersonsPersonalFolderIsNeverSent(t *testing.T) {
	r := newRig(t)
	for _, u := range []auth.Principal{adminOne, adminTwo} {
		r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $1::uuid::text, $1::uuid::text || '@example.com', 'u')`, u.ID)
	}
	own := r.dashboardInPersonalFolder(t, adminOne.ID)
	others := r.dashboardInPersonalFolder(t, adminTwo.ID)
	r.create(t, activity.TypeDashboardCreated)
	ownSeq := r.appendFor(t, activity.TypeDashboardCreated, own)
	r.appendFor(t, activity.TypeDashboardCreated, others)

	r.pass(t)

	if got, want := seqsSent(t, r.sender.all()), []int64{ownSeq}; !slices.Equal(got, want) {
		t.Fatalf("sent %v, want only the owner's own personal folder event %v", got, want)
	}
}

func TestAnEventOfAFiledFindingOrSavedSearchFollowsItsFolder(t *testing.T) {
	r := newRig(t)
	for _, u := range []auth.Principal{adminOne, adminTwo} {
		r.db.Exec(t, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, name) VALUES ($1::uuid, 'https://idp', $1::uuid::text, $1::uuid::text || '@example.com', 'u')`, u.ID)
	}
	r.create(t, activity.TypeFindingCreated, activity.TypeSavedSearchCreated)
	ownFinding, ownFolder := r.findingInPersonalFolder(t, adminOne.ID)
	othersFinding, _ := r.findingInPersonalFolder(t, adminTwo.ID)
	ownSearch := r.savedSearchInPersonalFolder(t, adminOne.ID)
	othersSearch := r.savedSearchInPersonalFolder(t, adminTwo.ID)
	wantFinding := r.appendOwned(t, activity.TypeFindingCreated, activity.OnFinding(ownFinding))
	r.appendOwned(t, activity.TypeFindingCreated, activity.OnFinding(othersFinding))
	wantSearch := r.appendOwned(t, activity.TypeSavedSearchCreated, activity.OnSavedSearch(ownSearch))
	r.appendOwned(t, activity.TypeSavedSearchCreated, activity.OnSavedSearch(othersSearch))

	r.pass(t)

	sent := r.sender.all()
	if got, want := seqsSent(t, sent), []int64{wantFinding, wantSearch}; !slices.Equal(got, want) {
		t.Fatalf("sent %v, want only the owner's own %v", got, want)
	}
	var payload struct {
		Event activity.EventJSON `json:"event"`
	}
	if err := json.Unmarshal(sent[0].Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Event.DashboardID != nil || payload.Event.FolderID == nil || *payload.Event.FolderID != ownFolder {
		t.Fatalf("payload dashboard_id %v and folder_id %v, want no dashboard and folder %s", payload.Event.DashboardID, payload.Event.FolderID, ownFolder)
	}
}

func (r *rig) findingInPersonalFolder(t *testing.T, owner string) (id pgtype.UUID, folder string) {
	t.Helper()
	var raw string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `
		INSERT INTO findings (title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name, folder_id)
		VALUES ('F', 'm', now() - interval '1 hour', now(), 'user', $1::uuid, 'Seed', (SELECT id FROM folders WHERE owner_id = $1::uuid))
		RETURNING id::text, folder_id::text`, owner).Scan(&raw, &folder)
	if err != nil {
		t.Fatal(err)
	}
	return r.uuid(t, raw), folder
}

func (r *rig) savedSearchInPersonalFolder(t *testing.T, owner string) pgtype.UUID {
	t.Helper()
	var raw string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `
		INSERT INTO saved_searches (kind, title, datasource_uid, query, time_from, time_to, folder_id, created_by_kind, created_by_id, created_by_name)
		VALUES ('logs', 'S', 'loki', '{app="x"}', 'now-1h', 'now', (SELECT id FROM folders WHERE owner_id = $1::uuid), 'user', $1::uuid, 'Seed')
		RETURNING id::text`, owner).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	return r.uuid(t, raw)
}

func (r *rig) dashboardInPersonalFolder(t *testing.T, owner string) pgtype.UUID {
	t.Helper()
	var raw string
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `
		INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via, folder_id)
		VALUES ('Mine', 1, 'user', gen_random_uuid(), 'Seed', 'direct', (SELECT id FROM folders WHERE owner_id = $1::uuid))
		RETURNING id::text`, owner).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	return r.uuid(t, raw)
}

func TestAnEventOlderThanADayIsPassedOver(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	stale := r.append(t, activity.TypeFindingCreated)
	fresh := r.append(t, activity.TypeFindingCreated)
	r.db.Exec(t, `UPDATE activity SET created_at = now() - interval '25 hours' WHERE seq = $1`, stale)

	r.pass(t)

	if got, want := seqsSent(t, r.sender.all()), []int64{fresh}; !slices.Equal(got, want) {
		t.Fatalf("sent %v, want %v", got, want)
	}
	if row := r.row(t, created.Webhook.ID); row.CursorSeq != fresh {
		t.Fatalf("cursor at %d, want %d", row.CursorSeq, fresh)
	}
}

func TestACursorOverASparseFilterAdvancesToTheHead(t *testing.T) {
	r := newRig(t)
	created := r.create(t, activity.TypeRunSucceeded)
	r.append(t, activity.TypeFindingCreated)
	last := r.append(t, activity.TypeFindingCreated)

	r.pass(t)

	if len(r.sender.all()) != 0 {
		t.Fatalf("sent %d events of another type", len(r.sender.all()))
	}
	if row := r.row(t, created.Webhook.ID); row.CursorSeq != last {
		t.Fatalf("cursor at %d, want the head %d", row.CursorSeq, last)
	}
}

func TestAnOlderTransactionThatAppendsLaterHoldsBackAnEarlierSeqUntilItCommits(t *testing.T) {
	r := newRig(t)
	r.create(t)
	ctx := t.Context()
	hasXid := make(chan struct{})
	laterAppended := make(chan struct{})
	release := make(chan struct{})
	var olderSeq, laterSeq int64

	older := make(chan error, 1)
	go func() {
		older <- r.db.InTx(ctx, func(q *dbsqlc.Queries) error {
			if _, err := q.CreateFolder(ctx, dbsqlc.CreateFolderParams{Title: "assigns a transaction id"}); err != nil {
				return err
			}
			close(hasXid)
			select {
			case <-laterAppended:
			case <-time.After(waitTimeout):
				return errors.New("the later transaction never appended")
			}
			var err error
			olderSeq, err = activity.Append(ctx, q, activity.Entry{Type: activity.TypeFindingCreated, TargetID: targets(), Actor: someone}, nil)
			return err
		})
	}()
	later := make(chan error, 1)
	go func() {
		later <- r.db.InTx(ctx, func(q *dbsqlc.Queries) error {
			select {
			case <-hasXid:
			case <-time.After(waitTimeout):
				return errors.New("the older transaction never started")
			}
			var err error
			laterSeq, err = activity.Append(ctx, q, activity.Entry{Type: activity.TypeFindingCreated, TargetID: targets(), Actor: someone}, nil)
			close(laterAppended)
			if err != nil {
				return err
			}
			select {
			case <-release:
			case <-time.After(waitTimeout):
				return errors.New("never released")
			}
			return nil
		})
	}()
	select {
	case err := <-older:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("the older transaction did not commit")
	}
	if laterSeq >= olderSeq {
		t.Fatalf("setup: the later transaction's seq %d is not below the older one's %d", laterSeq, olderSeq)
	}

	r.pass(t)
	if got, want := seqsSent(t, r.sender.all()), []int64{olderSeq}; !slices.Equal(got, want) {
		t.Fatalf("while the later transaction is open sent %v, want %v", got, want)
	}

	close(release)
	if err := <-later; err != nil {
		t.Fatal(err)
	}
	r.pass(t)
	if got, want := seqsSent(t, r.sender.all()), []int64{olderSeq, laterSeq}; !slices.Equal(got, want) {
		t.Fatalf("after the commit sent %v, want %v", got, want)
	}
}

func TestTwoWorkersOnOneDatabaseSendEachEventOnce(t *testing.T) {
	r := newRig(t)
	r.create(t)
	other := r.workerAs(discard(), "instance-b")
	const events = 30
	want := make([]int64, 0, events)
	for range events {
		want = append(want, r.append(t, activity.TypeFindingCreated))
	}
	r.sender.script(func(int, call) (egress.SendResult, error) {
		time.Sleep(5 * time.Millisecond)
		return egress.SendResult{Status: 200}, nil
	})

	var wg sync.WaitGroup
	for _, w := range []*Worker{r.worker, other} {
		wg.Go(func() {
			for range 3 {
				w.pass(t.Context())
			}
		})
	}
	wg.Wait()

	got := seqsSent(t, r.sender.all())
	if !slices.Equal(got, want) {
		t.Fatalf("sent %v, want each of %v once and in order", got, want)
	}
}

func TestALostLeaseRecordsNothingAndStops(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	r.append(t, activity.TypeFindingCreated)
	r.append(t, activity.TypeFindingCreated)
	before := r.row(t, created.Webhook.ID).CursorSeq
	r.sender.script(func(int, call) (egress.SendResult, error) {
		r.db.Exec(t, `UPDATE webhooks SET lease_id = gen_random_uuid() WHERE id = $1::uuid`, created.Webhook.ID)
		return egress.SendResult{Status: 200}, nil
	})

	r.pass(t)

	if got := len(r.sender.all()); got != 1 {
		t.Fatalf("sent %d events after the lease was lost, want 1", got)
	}
	if rows := r.deliveries(t, created.Webhook.ID); len(rows) != 0 {
		t.Fatalf("recorded %d attempts without the lease", len(rows))
	}
	if row := r.row(t, created.Webhook.ID); row.CursorSeq != before {
		t.Fatalf("cursor moved to %d without the lease", row.CursorSeq)
	}
}

func TestADeletedWebhookMidPassRecordsNothing(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	r.append(t, activity.TypeFindingCreated)
	r.sender.script(func(int, call) (egress.SendResult, error) {
		r.db.Exec(t, `DELETE FROM webhooks WHERE id = $1::uuid`, created.Webhook.ID)
		return egress.SendResult{Status: 200}, nil
	})

	r.pass(t)

	var left int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM webhook_deliveries`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("%d attempts left, err %v", left, err)
	}
}

func TestTheWorkerStopsAClaimWhenTheOwnerChangesMidPass(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	r.append(t, activity.TypeFindingCreated)
	r.append(t, activity.TypeFindingCreated)
	r.sender.script(func(n int, _ call) (egress.SendResult, error) {
		if n == 0 {
			if _, err := r.svc.Update(t.Context(), adminTwo, r.uuid(t, created.Webhook.ID), Patch{}); err != nil {
				t.Error(err)
			}
		}
		return egress.SendResult{Status: 200}, nil
	})

	r.pass(t)
	if got := len(r.sender.all()); got != 1 {
		t.Fatalf("sent %d events, want the claim to stop after the owner changed", got)
	}
	r.pass(t)
	if got := len(r.sender.all()); got != 2 {
		t.Fatalf("sent %d events, want the second one under the new owner on the next claim", got)
	}
}

func TestNoSecretOrAddressReachesTheLogs(t *testing.T) {
	r := newRig(t)
	created := r.create(t)
	r.append(t, activity.TypeFindingCreated)
	r.sender.script(func(int, call) (egress.SendResult, error) { return egress.SendResult{Status: 500}, nil })

	r.pass(t)

	logs := r.logs.String()
	if !strings.Contains(logs, "webhook delivery failed") {
		t.Fatalf("no failure line in %s", logs)
	}
	for _, secret := range []string{created.Secret, receiverHost, "X-SlashObserve", "finding.created", "t="} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logs hold %q: %s", secret, logs)
		}
	}
}

func TestRunDeliversOnAnActivityWakeAndStopsWithItsContext(t *testing.T) {
	r := newRig(t)
	r.create(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	broker := events.NewBroker(r.db.Listen, discard())
	go broker.Run(ctx)
	worker := NewWorker(WorkerDeps{
		DB: r.db.DB, Activity: r.store, Principals: r.principals, Sender: r.sender, Key: r.key,
		Followers: broker, InstanceID: "instance-run", Logger: discard(),
	})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		worker.Run(ctx)
	}()

	seq := r.append(t, activity.TypeFindingCreated)

	deadline := time.After(pollInterval - time.Second)
	for len(r.sender.all()) == 0 {
		select {
		case <-deadline:
			t.Fatal("the event was not sent before the poll interval, so the activity wake did not run a pass")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if got := seqsSent(t, r.sender.all()); !slices.Equal(got, []int64{seq}) {
		t.Fatalf("sent %v, want [%d]", got, seq)
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(waitTimeout):
		t.Fatal("Run did not return after its context was canceled")
	}
}
