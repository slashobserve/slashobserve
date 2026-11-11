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
	DB       *db.DB
	Activity *activity.Store
	Domains  egress.Allowlist
	Sender   Sender
	Key      sealed.Key
	Logger   *slog.Logger
}

type Service struct {
	db       *db.DB
	activity *activity.Store
	domains  egress.Allowlist
	sender   Sender
	key      sealed.Key
	logger   *slog.Logger
}

func NewService(d Deps) *Service {
	return &Service{db: d.DB, activity: d.Activity, domains: d.Domains, sender: d.Sender, key: d.Key, logger: d.Logger}
}

type Input struct {
	Name       string
	URL        string
	EventTypes []string
}

type Patch struct {
	Name       *string
	URL        *string
	EventTypes *[]string
	Enabled    *bool
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

func (s *Service) Create(ctx context.Context, by auth.Principal, in Input) (Created, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return Created{}, err
	}
	address, host, err := cleanURL(in.URL)
	if err != nil {
		return Created{}, err
	}
	types, err := cleanEventTypes(in.EventTypes)
	if err != nil {
		return Created{}, err
	}
	if !s.key.Set() {
		return Created{}, ErrNoSecretKey
	}
	if err := s.checkHost(ctx, host); err != nil {
		return Created{}, err
	}
	ownerKind, ownerID, err := owner(by)
	if err != nil {
		return Created{}, err
	}
	head, err := source{store: s.activity}.head(ctx)
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
		if _, err := q.InsertWebhook(ctx, dbsqlc.InsertWebhookParams{
			ID: id, Name: name, Url: address, SecretCiphertext: ciphertext, EventTypes: types,
			OwnerKind: ownerKind, OwnerID: ownerID,
			CursorXid: pgtype.Uint64{Uint64: head.xid, Valid: true}, CursorSeq: head.seq,
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
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrTooMany), errors.Is(err, context.Canceled):
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
		out = append(out, webhookOf(dbsqlc.GetWebhookRow(r)))
	}
	return out, nil
}

type cleanedPatch struct {
	name, address string
	types         []string
	enabled       *bool
}

func (s *Service) clean(ctx context.Context, p Patch) (cleanedPatch, error) {
	c := cleanedPatch{enabled: p.Enabled}
	var err error
	if p.Name != nil {
		if c.name, err = cleanName(*p.Name); err != nil {
			return cleanedPatch{}, err
		}
	}
	if p.URL != nil {
		var host string
		if c.address, host, err = cleanURL(*p.URL); err != nil {
			return cleanedPatch{}, err
		}
		if err := s.checkHost(ctx, host); err != nil {
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

func (c cleanedPatch) apply(current dbsqlc.Webhook, ownerKind string, ownerID pgtype.UUID, now time.Time) dbsqlc.UpdateWebhookParams {
	at := pgtype.Timestamptz{Time: now, Valid: true}
	params := dbsqlc.UpdateWebhookParams{
		Name: cmp.Or(c.name, current.Name), Url: cmp.Or(c.address, current.Url), EventTypes: current.EventTypes,
		OwnerKind: ownerKind, OwnerID: ownerID,
		ConsecutiveFailures: current.ConsecutiveFailures, NextAttemptAt: current.NextAttemptAt,
		DisabledAt: current.DisabledAt, DisabledReason: current.DisabledReason,
	}
	if c.types != nil {
		params.EventTypes = c.types
	}
	if c.address != "" && c.address != current.Url {
		params.ConsecutiveFailures, params.NextAttemptAt = 0, at
	}
	if c.enabled != nil && *c.enabled {
		params.ConsecutiveFailures, params.NextAttemptAt = 0, at
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
		current, err := q.GetWebhookForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		params := cleaned.apply(current, ownerKind, ownerID, time.Now())
		params.ID = id
		if _, err := q.UpdateWebhook(ctx, params); err != nil {
			return err
		}
		updated, err = view(ctx, q, id)
		return err
	})
	if err != nil {
		return Webhook{}, wrapStore("update webhook", err)
	}
	s.logger.InfoContext(ctx, "webhook updated", slog.String("webhook_id", updated.ID), slog.String("actor_id", by.ID))
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error {
	deleted, err := s.db.Queries().DeleteWebhook(ctx, id)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	if deleted == 0 {
		return ErrNotFound
	}
	s.logger.InfoContext(ctx, "webhook deleted", slog.String("webhook_id", id.String()), slog.String("actor_id", by.ID))
	return nil
}

func (s *Service) RotateSecret(ctx context.Context, by auth.Principal, id pgtype.UUID) (Created, error) {
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
	ciphertext, err := sealSecret(s.key, id, secret)
	if err != nil {
		return Created{}, err
	}
	var rotated Webhook
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if _, err := q.GetWebhookForUpdate(ctx, id); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if _, err := q.SetWebhookSecret(ctx, dbsqlc.SetWebhookSecretParams{ID: id, SecretCiphertext: ciphertext, OwnerKind: ownerKind, OwnerID: ownerID}); err != nil {
			return err
		}
		rotated, err = view(ctx, q, id)
		return err
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
	body, deliveryID, err := testPayload(id.String())
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
