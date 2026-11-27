package webhooks

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/sealed"
)

type Deps struct {
	DB         *db.DB
	Activity   *activity.Store
	Principals Principals
	Domains    egress.Allowlist
	Sender     Sender
	Key        sealed.Key
	Logger     *slog.Logger
}

type Service struct {
	db         *db.DB
	activity   *activity.Store
	source     source
	principals Principals
	domains    egress.Allowlist
	sender     Sender
	key        sealed.Key
	logger     *slog.Logger
}

func NewService(d Deps) *Service {
	return &Service{db: d.DB, activity: d.Activity, source: source{store: d.Activity}, principals: d.Principals, domains: d.Domains, sender: d.Sender, key: d.Key, logger: d.Logger}
}

type Input struct {
	Name       string
	URL        string
	Format     string
	EventTypes []string
}

type Patch struct {
	Name       *string
	URL        *string
	Format     *string
	EventTypes *[]string
	Enabled    *bool
	Owner      *string
}

type Created struct {
	Webhook Webhook
	Secret  string
}

type Delivery struct {
	ID          int64
	DeliveryID  string
	EventType   string
	ActivitySeq *int64
	AttemptedAt time.Time
	DurationMs  int
	StatusCode  *int
	Outcome     string
	ErrorCode   string
}

func deliveryOf(row dbsqlc.WebhookDelivery) Delivery {
	d := Delivery{
		ID:          row.ID,
		DeliveryID:  row.DeliveryID,
		EventType:   row.EventType,
		AttemptedAt: row.AttemptedAt.Time.UTC(),
		DurationMs:  int(row.DurationMs),
		Outcome:     row.Outcome,
		ErrorCode:   row.ErrorCode.String,
	}
	if row.ActivitySeq.Valid {
		d.ActivitySeq = &row.ActivitySeq.Int64
	}
	if row.StatusCode.Valid {
		status := int(row.StatusCode.Int32)
		d.StatusCode = &status
	}
	return d
}

func deliveryParams(webhookID pgtype.UUID, deliveryID, eventType string, activitySeq pgtype.Int8, a attempt) dbsqlc.InsertWebhookDeliveryParams {
	return dbsqlc.InsertWebhookDeliveryParams{
		WebhookID:   webhookID,
		DeliveryID:  deliveryID,
		EventType:   eventType,
		ActivitySeq: activitySeq,
		DurationMs:  httpapi.Int32(int(min(a.duration.Milliseconds(), 1<<30))),
		StatusCode:  pgtype.Int4{Int32: httpapi.Int32(a.status), Valid: a.status > 0},
		Outcome:     a.outcome,
		ErrorCode:   pgtype.Text{String: a.errorCode, Valid: a.errorCode != ""},
	}
}

func newID() pgtype.UUID {
	var id pgtype.UUID
	_, _ = rand.Read(id.Bytes[:])
	id.Bytes[6] = id.Bytes[6]&0x0f | 0x40
	id.Bytes[8] = id.Bytes[8]&0x3f | 0x80
	id.Valid = true
	return id
}

func owner(by auth.Principal) (kind string, id pgtype.UUID, err error) {
	id, err = db.ParseUUID(by.ID)
	if err != nil {
		return "", pgtype.UUID{}, fmt.Errorf("webhook owner id: %w", err)
	}
	return string(by.Kind), id, nil
}

func (s *Service) checkHost(ctx context.Context, host string) error {
	list, err := s.domains.Allowed(ctx)
	if err != nil {
		return fmt.Errorf("read outbound allowlist: %w", err)
	}
	if !list.Allows(host) {
		return &egress.HostError{Host: host, Err: egress.ErrNotAllowed}
	}
	return nil
}

func (s *Service) lockAllowedHost(ctx context.Context, q *dbsqlc.Queries, host string) error {
	if err := q.LockOutboundDomains(ctx); err != nil {
		return err
	}
	return s.checkHost(ctx, host)
}

type cleanedInput struct {
	name, address, host, format string
	types                       []string
}

func cleanInput(in Input) (cleanedInput, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return cleanedInput{}, err
	}
	address, host, err := cleanURL(in.URL)
	if err != nil {
		return cleanedInput{}, err
	}
	format, err := cleanFormat(cmp.Or(in.Format, FormatGeneric))
	if err != nil {
		return cleanedInput{}, err
	}
	types, err := cleanEventTypes(in.EventTypes)
	if err != nil {
		return cleanedInput{}, err
	}
	return cleanedInput{name: name, address: address, host: host, format: format, types: types}, nil
}

func (s *Service) Create(ctx context.Context, by auth.Principal, in Input) (Created, error) {
	clean, err := cleanInput(in)
	if err != nil {
		return Created{}, err
	}
	if !s.key.Set() {
		return Created{}, ErrNoSecretKey
	}
	ownerKind, ownerID, err := owner(by)
	if err != nil {
		return Created{}, err
	}
	secret, err := newSecret()
	if err != nil {
		return Created{}, err
	}
	id := newID()
	ciphertext, err := sealSecret(s.key, id, secret)
	if err != nil {
		return Created{}, err
	}
	var created Webhook
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := s.lockAllowedHost(ctx, q, clean.host); err != nil {
			return err
		}
		if err := q.LockWebhooks(ctx); err != nil {
			return err
		}
		count, err := q.CountWebhooks(ctx)
		if err != nil {
			return err
		}
		if count >= maxWebhooks {
			return ErrTooMany
		}
		seq, err := recordCreated(ctx, q, by, id, recordedWebhook{Name: clean.name, Host: clean.host, EventTypes: clean.types})
		if err != nil {
			return err
		}
		xid, err := q.ActivityXid(ctx, seq)
		if err != nil {
			return err
		}
		if _, err := q.InsertWebhook(ctx, dbsqlc.InsertWebhookParams{
			ID: id, Name: clean.name, Url: clean.address, Format: clean.format, SecretCiphertext: ciphertext, EventTypes: clean.types,
			OwnerKind: ownerKind, OwnerID: ownerID,
			CursorXid: xid, CursorSeq: seq,
		}); err != nil {
			return err
		}
		created, err = view(ctx, q, id)
		return err
	})
	if err != nil {
		return Created{}, wrapStore("create webhook", err)
	}
	s.logger.InfoContext(ctx, "webhook created", slog.String("webhook_id", created.ID), slog.String("actor_id", by.ID))
	return Created{Webhook: created, Secret: secret}, nil
}

func view(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID) (Webhook, error) {
	row, err := q.GetWebhook(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Webhook{}, ErrNotFound
	}
	if err != nil {
		return Webhook{}, err
	}
	return webhookOf(row), nil
}

func wrapStore(action string, err error) error {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrTooMany), errors.Is(err, ErrNothingToSkip), errors.Is(err, ErrBusy), errors.Is(err, context.Canceled):
		return err
	}
	return fmt.Errorf("%s: %w", action, err)
}

func (s *Service) List(ctx context.Context) ([]Webhook, error) {
	rows, err := s.db.Queries().ListWebhooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	out := make([]Webhook, 0, len(rows))
	for _, r := range rows {
		w := webhookOf(dbsqlc.GetWebhookRow(r))
		if w.ConsecutiveFailures > 0 {
			w.Waiting = s.waiting(ctx, r.OwnerKind, r.OwnerID.String(), r.EventTypes, w.at)
		}
		out = append(out, w)
	}
	return out, nil
}

func (s *Service) pending(ctx context.Context, ownerKind, ownerID string, types []string, from cursor, limit int) ([]activity.Event, error) {
	owner, err := s.principals.Principal(ctx, auth.PrincipalKind(ownerKind), ownerID)
	if err != nil {
		return nil, err
	}
	audience, err := s.source.audience(ctx, owner)
	if err != nil {
		return nil, err
	}
	page, _, err := s.source.next(ctx, audience, types, time.Now().Add(-expiry), from, limit)
	return page, err
}

func (s *Service) waiting(ctx context.Context, ownerKind, ownerID string, types []string, from cursor) *Waiting {
	page, err := s.pending(ctx, ownerKind, ownerID, types, from, maxWaitingCount+1)
	if err != nil {
		return nil
	}
	return &Waiting{Count: min(len(page), maxWaitingCount), More: len(page) > maxWaitingCount}
}

func (s *Service) SkipEvent(ctx context.Context, by auth.Principal, id pgtype.UUID) (skipped Delivery, updated Webhook, err error) {
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		row, err := q.GetWebhookForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if row.ConsecutiveFailures == 0 || row.DisabledAt.Valid {
			return ErrNothingToSkip
		}
		if row.LeaseUntil.Valid && row.LeaseUntil.Time.After(time.Now()) {
			return ErrBusy
		}
		page, err := s.pending(ctx, row.OwnerKind, row.OwnerID.String(), row.EventTypes, cursor{xid: row.CursorXid.Uint64, seq: row.CursorSeq}, 1)
		if errors.Is(err, auth.ErrPrincipalGone) || (err == nil && len(page) == 0) {
			return ErrNothingToSkip
		}
		if err != nil {
			return err
		}
		e := page[0]
		stored, err := q.InsertWebhookDelivery(ctx, deliveryParams(id, id.String()+"."+strconv.FormatInt(e.Seq, 10), e.Type,
			pgtype.Int8{Int64: e.Seq, Valid: true}, attempt{outcome: outcomeSkipped, errorCode: errorSkippedByAdmin}))
		if err != nil {
			return err
		}
		at := cursorOf(e)
		if _, err := q.SkipWebhookEvent(ctx, dbsqlc.SkipWebhookEventParams{ID: id, CursorXid: pgtype.Uint64{Uint64: at.xid, Valid: true}, CursorSeq: at.seq}); err != nil {
			return err
		}
		skipped = deliveryOf(stored)
		updated, err = view(ctx, q, id)
		return err
	})
	if err != nil {
		return Delivery{}, Webhook{}, wrapStore("skip webhook event", err)
	}
	s.logger.InfoContext(ctx, "webhook event skipped", slog.String("webhook_id", updated.ID), slog.String("actor_id", by.ID), slog.String("event_type", skipped.EventType))
	return skipped, updated, nil
}

type cleanedPatch struct {
	name, address, host, format string
	types                       []string
	enabled                     *bool
	takeOwnership               bool
}

func (s *Service) clean(ctx context.Context, p Patch) (cleanedPatch, error) {
	c := cleanedPatch{enabled: p.Enabled}
	var err error
	if p.Owner != nil {
		if *p.Owner != ownerMe {
			return cleanedPatch{}, &InvalidError{Message: `owner can only be "me"`}
		}
		c.takeOwnership = true
	}
	if p.Name != nil {
		if c.name, err = cleanName(*p.Name); err != nil {
			return cleanedPatch{}, err
		}
	}
	if p.URL != nil {
		if c.address, c.host, err = cleanURL(*p.URL); err != nil {
			return cleanedPatch{}, err
		}
	}
	if p.Format != nil {
		if c.format, err = cleanFormat(*p.Format); err != nil {
			return cleanedPatch{}, err
		}
	}
	if p.EventTypes != nil {
		if c.types, err = cleanEventTypes(*p.EventTypes); err != nil {
			return cleanedPatch{}, err
		}
	}
	return c, nil
}

func (c cleanedPatch) rehomes(current dbsqlc.Webhook) bool {
	reenablesOrphan := c.enabled != nil && *c.enabled && current.DisabledReason.String == ReasonOwner
	return c.takeOwnership || reenablesOrphan
}

func (c cleanedPatch) apply(current dbsqlc.Webhook, ownerKind string, ownerID pgtype.UUID, now time.Time) dbsqlc.UpdateWebhookParams {
	at := pgtype.Timestamptz{Time: now, Valid: true}
	params := dbsqlc.UpdateWebhookParams{
		Name: cmp.Or(c.name, current.Name), Url: cmp.Or(c.address, current.Url), Format: cmp.Or(c.format, current.Format), EventTypes: current.EventTypes,
		OwnerKind: current.OwnerKind, OwnerID: current.OwnerID,
		ConsecutiveFailures: current.ConsecutiveFailures, NextAttemptAt: current.NextAttemptAt,
		DisabledAt: current.DisabledAt, DisabledReason: current.DisabledReason,
	}
	if c.rehomes(current) {
		params.OwnerKind, params.OwnerID = ownerKind, ownerID
	}
	if c.types != nil {
		params.EventTypes = c.types
	}
	enables := c.enabled != nil && *c.enabled
	if enables || (c.address != "" && c.address != current.Url) {
		params.ConsecutiveFailures, params.NextAttemptAt = 0, at
		params.DropsLease = true
	}
	if enables {
		params.DisabledAt, params.DisabledReason = pgtype.Timestamptz{}, pgtype.Text{}
	}
	if c.enabled != nil && !*c.enabled && !current.DisabledAt.Valid {
		params.DisabledAt = at
		params.DisabledReason = pgtype.Text{String: ReasonManual, Valid: true}
	}
	return params
}

func (s *Service) Update(ctx context.Context, by auth.Principal, id pgtype.UUID, p Patch) (Webhook, error) {
	ownerKind, ownerID, err := owner(by)
	if err != nil {
		return Webhook{}, err
	}
	cleaned, err := s.clean(ctx, p)
	if err != nil {
		return Webhook{}, err
	}
	var updated Webhook
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if cleaned.host != "" {
			if err := s.lockAllowedHost(ctx, q, cleaned.host); err != nil {
				return err
			}
		}
		current, err := q.GetWebhookForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		params := cleaned.apply(current, ownerKind, ownerID, time.Now())
		params.ID = id
		row, err := q.UpdateWebhook(ctx, params)
		if err != nil {
			return err
		}
		updated, err = view(ctx, q, id)
		if err != nil {
			return err
		}
		return recordUpdate(ctx, q, by, current, row)
	})
	if err != nil {
		return Webhook{}, wrapStore("update webhook", err)
	}
	s.logger.InfoContext(ctx, "webhook updated", slog.String("webhook_id", updated.ID), slog.String("actor_id", by.ID))
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error {
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		current, err := q.GetWebhookForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := q.DeleteWebhook(ctx, id); err != nil {
			return err
		}
		return record(ctx, q, by, activity.TypeWebhookDeleted, id, recordedWebhook{Name: current.Name, Host: hostOf(current.Url)})
	})
	if err != nil {
		return wrapStore("delete webhook", err)
	}
	s.logger.InfoContext(ctx, "webhook deleted", slog.String("webhook_id", id.String()), slog.String("actor_id", by.ID))
	return nil
}

func (s *Service) RotateSecret(ctx context.Context, by auth.Principal, id pgtype.UUID) (Created, error) {
	if !s.key.Set() {
		return Created{}, ErrNoSecretKey
	}
	secret, err := newSecret()
	if err != nil {
		return Created{}, err
	}
	ciphertext, err := sealSecret(s.key, id, secret)
	if err != nil {
		return Created{}, err
	}
	var rotated Webhook
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		current, err := q.GetWebhookForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := q.SetWebhookSecret(ctx, dbsqlc.SetWebhookSecretParams{ID: id, SecretCiphertext: ciphertext}); err != nil {
			return err
		}
		rotated, err = view(ctx, q, id)
		if err != nil {
			return err
		}
		return record(ctx, q, by, activity.TypeWebhookSecretRotated, id, recordedWebhook{Name: current.Name, Host: hostOf(current.Url)})
	})
	if err != nil {
		return Created{}, wrapStore("rotate webhook secret", err)
	}
	s.logger.InfoContext(ctx, "webhook secret rotated", slog.String("webhook_id", rotated.ID), slog.String("actor_id", by.ID))
	return Created{Webhook: rotated, Secret: secret}, nil
}

func (s *Service) Test(ctx context.Context, id pgtype.UUID) (Delivery, error) {
	row, err := s.db.Queries().GetWebhook(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, ErrNotFound
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("get webhook: %w", err)
	}
	secret, err := openSecret(s.key, id, row.SecretCiphertext)
	if err != nil {
		return Delivery{}, err
	}
	body, deliveryID, err := testPayload(id.String(), row.Format)
	if err != nil {
		return Delivery{}, err
	}
	got, err := deliver(ctx, s.sender, time.Now(), row.Url, secret, deliveryID, typeTest, body)
	if err != nil {
		return Delivery{}, err
	}
	stored, err := s.db.Queries().InsertWebhookDelivery(ctx, deliveryParams(id, deliveryID, typeTest, pgtype.Int8{}, got))
	if db.IsForeignKeyViolation(err) {
		return Delivery{}, ErrNotFound
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("record test delivery: %w", err)
	}
	return deliveryOf(stored), nil
}

func (s *Service) Deliveries(ctx context.Context, id pgtype.UUID, cursor string, limit int) (page []Delivery, next string, err error) {
	before, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	q := s.db.Queries()
	if _, err := q.GetWebhook(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	} else if err != nil {
		return nil, "", fmt.Errorf("get webhook: %w", err)
	}
	rows, err := q.ListWebhookDeliveries(ctx, dbsqlc.ListWebhookDeliveriesParams{
		WebhookID: id, BeforeID: before, RowLimit: httpapi.Int32(limit + 1),
	})
	if err != nil {
		return nil, "", fmt.Errorf("list webhook deliveries: %w", err)
	}
	if len(rows) > limit {
		rows = rows[:limit]
		next = encodeCursor(rows[limit-1].ID)
	}
	page = make([]Delivery, 0, len(rows))
	for _, r := range rows {
		page = append(page, deliveryOf(r))
	}
	return page, next, nil
}

func encodeCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}

func decodeCursor(raw string) (pgtype.Int8, error) {
	if raw == "" {
		return pgtype.Int8{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return pgtype.Int8{}, ErrInvalidCursor
	}
	id, err := strconv.ParseInt(string(decoded), 10, 64)
	if err != nil || id < 1 {
		return pgtype.Int8{}, ErrInvalidCursor
	}
	return pgtype.Int8{Int64: id, Valid: true}, nil
}
