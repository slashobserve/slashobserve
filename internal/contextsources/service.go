package contextsources

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/sealed"
)

const (
	MaxSources      = 50
	secretPurpose   = "slashobserve context source secret v1"
	uniqueViolation = "23505"
)

var (
	ErrNotFound      = errors.New("context source not found")
	ErrToolNotFound  = errors.New("tool not found")
	ErrUIDTaken      = errors.New("a context source with this uid already exists")
	ErrTooMany       = errors.New("too many context sources")
	ErrProvisioned   = errors.New("this context source is defined in the context sources file on the server and cannot be changed here")
	ErrNoSecretKey   = errors.New("SLASHOBSERVE_SECRET_KEY must be set to store a secret")
	ErrAllowlistFull = errors.New("the outbound allowlist is full")
	ErrSourceChanged = errors.New("the context source changed while the server was being asked; run discovery again")
	ErrToolChanged   = errors.New("the tool changed since it was read; review it again")
)

type RefusedError struct {
	Tool   string
	Reason string
}

func (e *RefusedError) Error() string { return "tool " + e.Tool + " is refused: " + e.Reason }

type Domains interface {
	Allowed(ctx context.Context) (domains.List, error)
	SaveOnAllowedHost(ctx context.Context, by auth.Principal, host string, save func(*dbsqlc.Queries) error) (string, error)
	Release(ctx context.Context, by auth.Principal, host string) error
}

type Deps struct {
	Limits    Limits
	DB        *db.DB
	Key       sealed.Key
	Domains   Domains
	Connector Connector
	Logger    *slog.Logger
}

type Service struct {
	limits    Limits
	gate      gate
	db        *db.DB
	key       sealed.Key
	domains   Domains
	connector Connector
	logger    *slog.Logger
}

func New(d Deps) *Service {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{limits: d.Limits.filled(), db: d.DB, key: d.Key, domains: d.Domains, connector: d.Connector, logger: logger}
}

type ToolCounts struct {
	Offered  int
	Approved int
	Changed  int
	Gone     int
	Refused  int
}

func (c *ToolCounts) add(state ToolState, n int) {
	switch state {
	case ToolOffered:
		c.Offered += n
	case ToolApproved:
		c.Approved += n
	case ToolChanged:
		c.Changed += n
	case ToolGone:
		c.Gone += n
	case ToolRefused:
		c.Refused += n
	}
}

type Source struct {
	UID          string
	Name         string
	Description  string
	URL          string
	Audience     datasource.Audience
	SecretHeader string
	SecretSet    bool
	Provisioned  bool
	Private      bool
	State        string
	Failure      string
	CheckedAt    *time.Time
	Tools        ToolCounts
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func sourceOf(row dbsqlc.ContextSource, counts ToolCounts) Source {
	s := Source{
		UID:          row.Uid,
		Name:         row.Name,
		Description:  row.Description.String,
		URL:          row.Url,
		Audience:     datasource.Audience(row.Audience),
		SecretHeader: row.SecretHeader.String,
		SecretSet:    len(row.Secret) > 0,
		Provisioned:  row.Provisioned,
		Private:      row.AllowPrivate,
		Failure:      row.Failure.String,
		State:        sourceState(row.CheckedAt.Valid, row.Failure.String),
		Tools:        counts,
		CreatedAt:    row.CreatedAt.Time.UTC(),
		UpdatedAt:    row.UpdatedAt.Time.UTC(),
	}
	if row.CheckedAt.Valid {
		at := row.CheckedAt.Time.UTC()
		s.CheckedAt = &at
	}
	return s
}

func toolOf(row dbsqlc.ContextSourceTool) Tool {
	t := Tool{
		Name:      row.Name,
		State:     ToolState(row.State),
		Refusal:   row.Refusal.String,
		Pinned:    Definition{Description: row.Description, InputSchema: row.InputSchema, Hash: row.Hash},
		UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if row.SeenHash.Valid {
		t.Seen = &Definition{Description: row.SeenDescription.String, InputSchema: row.SeenInputSchema, Hash: row.SeenHash.String}
	}
	if row.ApprovedBy.Valid {
		t.ApprovedBy = uuid.UUID(row.ApprovedBy.Bytes).String()
	}
	if row.ApprovedAt.Valid {
		t.ApprovedAt = row.ApprovedAt.Time.UTC()
	}
	return t
}

func putParams(sourceID pgtype.UUID, t Tool) dbsqlc.PutContextSourceToolParams {
	p := dbsqlc.PutContextSourceToolParams{
		SourceID:    sourceID,
		Name:        t.Name,
		State:       string(t.State),
		Refusal:     text(t.Refusal),
		Description: t.Pinned.Description,
		InputSchema: t.Pinned.InputSchema,
		Hash:        t.Pinned.Hash,
	}
	if t.Seen != nil {
		p.SeenDescription = pgtype.Text{String: t.Seen.Description, Valid: true}
		p.SeenInputSchema = t.Seen.InputSchema
		p.SeenHash = text(t.Seen.Hash)
	}
	if t.ApprovedBy != "" {
		if id, err := db.ParseUUID(t.ApprovedBy); err == nil {
			p.ApprovedBy = id
		}
	}
	if !t.ApprovedAt.IsZero() {
		p.ApprovedAt = pgtype.Timestamptz{Time: t.ApprovedAt, Valid: true}
	}
	return p
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func newID() pgtype.UUID {
	var id pgtype.UUID
	_, _ = rand.Read(id.Bytes[:])
	id.Bytes[6] = id.Bytes[6]&0x0f | 0x40
	id.Bytes[8] = id.Bytes[8]&0x3f | 0x80
	id.Valid = true
	return id
}

func countsOf(rows []dbsqlc.ContextSourceTool) ToolCounts {
	var c ToolCounts
	for _, r := range rows {
		c.add(ToolState(r.State), 1)
	}
	return c
}

func (s *Service) row(ctx context.Context, uid string) (dbsqlc.ContextSource, error) {
	row, err := s.db.Queries().GetContextSource(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.ContextSource{}, ErrNotFound
	}
	return row, err
}

func (s *Service) List(ctx context.Context) ([]Source, error) {
	q := s.db.Queries()
	rows, err := q.ListContextSources(ctx)
	if err != nil {
		return nil, err
	}
	tally, err := q.ListContextSourceToolCounts(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[pgtype.UUID]*ToolCounts{}
	for _, t := range tally {
		c := counts[t.SourceID]
		if c == nil {
			c = &ToolCounts{}
			counts[t.SourceID] = c
		}
		c.add(ToolState(t.State), int(t.N))
	}
	out := make([]Source, len(rows))
	for i, row := range rows {
		var c ToolCounts
		if found := counts[row.ID]; found != nil {
			c = *found
		}
		out[i] = sourceOf(row, c)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, uid string) (Source, error) {
	row, err := s.row(ctx, uid)
	if err != nil {
		return Source{}, err
	}
	tools, err := s.db.Queries().ListContextSourceTools(ctx, row.ID)
	if err != nil {
		return Source{}, err
	}
	return sourceOf(row, countsOf(tools)), nil
}

func (s *Service) Tools(ctx context.Context, uid string) ([]Tool, error) {
	row, err := s.row(ctx, uid)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Queries().ListContextSourceTools(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	out := make([]Tool, len(rows))
	for i, r := range rows {
		out[i] = toolOf(r)
	}
	return out, nil
}

func (s *Service) seal(id pgtype.UUID, secret string) ([]byte, error) {
	if !s.key.Set() {
		return nil, ErrNoSecretKey
	}
	return s.key.Seal(secretPurpose, id, []byte(secret))
}

func (s *Service) open(row dbsqlc.ContextSource) (datasource.Secret, string) {
	if len(row.Secret) == 0 {
		return datasource.Secret{}, ""
	}
	plain, err := s.key.Open(secretPurpose, row.ID, row.Secret)
	switch {
	case errors.Is(err, sealed.ErrNoKey):
		return datasource.Secret{}, FailureNoSecretKey
	case err != nil:
		return datasource.Secret{}, FailureUnreadable
	}
	return datasource.NewSecret(string(plain)), ""
}

func (s *Service) saveOnAllowedHost(ctx context.Context, by auth.Principal, host string, save func(*dbsqlc.Queries) error) error {
	added, err := s.domains.SaveOnAllowedHost(ctx, by, host, save)
	switch {
	case err == nil:
		if added != "" {
			s.logger.InfoContext(ctx, "allowed an outbound domain", slog.String("actor_id", by.ID), slog.String("domain", added))
		}
		return nil
	case errors.Is(err, allowlist.ErrLocked):
		return &egress.HostError{Host: host, Err: egress.ErrNotAllowed}
	case errors.Is(err, allowlist.ErrTooMany):
		return ErrAllowlistFull
	}
	return err
}

func (s *Service) releaseHost(ctx context.Context, by auth.Principal, host string) {
	if err := s.domains.Release(context.WithoutCancel(ctx), by, host); err != nil {
		s.logger.WarnContext(ctx, "could not release an outbound domain", logattrs.Failure("outbound_release_failed", err, slog.String("host", host))...)
	}
}

func hostOfURL(raw string) string {
	host, err := checkAddress(raw, true)
	if err != nil {
		return ""
	}
	return host
}

type Input struct {
	UID          string
	Name         string
	Description  string
	URL          string
	Audience     datasource.Audience
	SecretHeader string
	Secret       string
	AllowPrivate bool
}

func (in Input) normalized() Input {
	in.UID = strings.TrimSpace(in.UID)
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.URL = strings.TrimSpace(in.URL)
	in.SecretHeader = strings.TrimSpace(in.SecretHeader)
	if in.Name == "" {
		in.Name = in.UID
	}
	if in.Audience == "" {
		in.Audience = datasource.AudienceEveryone
	}
	return in
}

func (in Input) check() (string, error) {
	if err := checkUID(in.UID); err != nil {
		return "", err
	}
	if err := checkName(in.Name); err != nil {
		return "", err
	}
	if err := checkDescription(in.Description); err != nil {
		return "", err
	}
	if err := checkAudience(in.Audience); err != nil {
		return "", err
	}
	host, err := checkAddress(in.URL, in.AllowPrivate)
	if err != nil {
		return "", err
	}
	if in.Secret != "" {
		if err := checkSecret(in.Secret); err != nil {
			return "", err
		}
		if in.SecretHeader == "" {
			return "", invalid("secret_header", "is required when a secret is set")
		}
	}
	if in.SecretHeader != "" {
		if err := checkHeader(in.SecretHeader); err != nil {
			return "", err
		}
	}
	return host, nil
}

func (s *Service) Create(ctx context.Context, by auth.Principal, in Input) (Source, error) {
	return s.create(ctx, by, in, false)
}

func (s *Service) create(ctx context.Context, by auth.Principal, in Input, provisioned bool) (Source, error) {
	in = in.normalized()
	host, err := in.check()
	if err != nil {
		return Source{}, err
	}
	id := newID()
	params := dbsqlc.InsertContextSourceParams{ID: id, Uid: in.UID, Name: in.Name, Description: text(in.Description), Url: in.URL, Audience: string(in.Audience), SecretHeader: text(in.SecretHeader), Provisioned: provisioned}
	if in.Secret != "" {
		if params.Secret, err = s.seal(id, in.Secret); err != nil {
			return Source{}, err
		}
	}
	if creator, err := db.ParseUUID(by.ID); err == nil && by.Kind == auth.KindUser {
		params.CreatedBy = creator
	}
	var row dbsqlc.ContextSource
	err = s.saveOnAllowedHost(ctx, by, host, func(q *dbsqlc.Queries) error {
		if err := q.LockContextSourceCount(ctx); err != nil {
			return err
		}
		count, err := q.CountContextSources(ctx)
		if err != nil {
			return err
		}
		if count >= MaxSources {
			return ErrTooMany
		}
		if err := q.LockContextSourceUID(ctx, in.UID); err != nil {
			return err
		}
		if row, err = q.InsertContextSource(ctx, params); err != nil {
			return err
		}
		return recordCreated(ctx, q, row, host)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return Source{}, ErrUIDTaken
	}
	if err != nil {
		return Source{}, err
	}
	s.logger.InfoContext(ctx, "context source added", slog.String("actor_id", by.ID), slog.String("uid", row.Uid))
	return sourceOf(row, ToolCounts{}), nil
}

type Patch struct {
	Name         *string
	Description  *string
	URL          *string
	Audience     *datasource.Audience
	SecretHeader *string
	Secret       *string
	ClearSecret  bool
}

func (s *Service) Update(ctx context.Context, by auth.Principal, uid string, p Patch) (Source, error) {
	row, err := s.row(ctx, uid)
	if err != nil {
		return Source{}, err
	}
	if row.Provisioned {
		return Source{}, ErrProvisioned
	}
	next, err := s.patched(row, p)
	if err != nil {
		return Source{}, err
	}
	return s.apply(ctx, by, row, next)
}

type revision struct {
	params      dbsqlc.UpdateContextSourceParams
	host        string
	credentials bool
}

func (s *Service) patched(row dbsqlc.ContextSource, p Patch) (revision, error) {
	in := Input{UID: row.Uid, Name: row.Name, Description: row.Description.String, URL: row.Url, Audience: datasource.Audience(row.Audience), SecretHeader: row.SecretHeader.String}
	if p.Name != nil {
		in.Name = *p.Name
	}
	if p.Description != nil {
		in.Description = *p.Description
	}
	if p.URL != nil {
		in.URL = *p.URL
	}
	if p.Audience != nil {
		in.Audience = *p.Audience
	}
	if p.SecretHeader != nil {
		in.SecretHeader = *p.SecretHeader
	}
	if p.Secret != nil {
		in.Secret = *p.Secret
	}
	in = in.normalized()
	if p.ClearSecret && p.Secret != nil {
		return revision{}, invalid("secret", "cannot be set and cleared in one change")
	}
	if p.Secret != nil && *p.Secret == "" {
		return revision{}, invalid("secret", "cannot be empty; clear the secret to remove it")
	}
	host, err := in.check()
	if err != nil {
		return revision{}, err
	}
	stored := len(row.Secret) > 0
	if stored && !p.ClearSecret && in.SecretHeader == "" {
		return revision{}, invalid("secret_header", "cannot be empty while a secret is stored; clear the secret to remove both")
	}
	if hostOfURL(row.Url) != host && stored && p.Secret == nil && !p.ClearSecret {
		return revision{}, invalid("secret", "a new host needs the secret again; send it, or clear it")
	}
	out := revision{host: host, params: dbsqlc.UpdateContextSourceParams{
		Uid: row.Uid, Name: in.Name, Description: text(in.Description), Url: in.URL, Audience: string(in.Audience), SecretHeader: text(in.SecretHeader), Secret: row.Secret,
	}}
	switch {
	case p.Secret != nil:
		if out.params.Secret, err = s.seal(row.ID, in.Secret); err != nil {
			return revision{}, err
		}
		out.credentials = true
	case p.ClearSecret:
		out.params.Secret = nil
		out.credentials = stored
	}
	if stored && !p.ClearSecret && in.SecretHeader != row.SecretHeader.String {
		out.credentials = true
	}
	out.params.ResetCheck = in.URL != row.Url || out.credentials
	return out, nil
}

func (s *Service) apply(ctx context.Context, by auth.Principal, row dbsqlc.ContextSource, next revision) (Source, error) {
	before := row
	addressChanged := next.params.Url != row.Url
	hostChanged := next.host != hostOfURL(row.Url)
	var updated dbsqlc.ContextSource
	save := func(q *dbsqlc.Queries) error {
		locked, err := q.LockContextSource(ctx, row.Uid)
		if err != nil {
			return err
		}
		if !locked.UpdatedAt.Time.Equal(before.UpdatedAt.Time) {
			return ErrSourceChanged
		}
		if updated, err = q.UpdateContextSource(ctx, next.params); err != nil {
			return err
		}
		if addressChanged {
			if err := q.DeleteContextSourceTools(ctx, row.ID); err != nil {
				return err
			}
		}
		return recordUpdated(ctx, q, before, updated, next, hostOfURL(before.Url))
	}
	var err error
	if hostChanged {
		err = s.saveOnAllowedHost(ctx, by, next.host, save)
	} else {
		err = s.db.InTx(ctx, save)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, ErrNotFound
	}
	if err != nil {
		return Source{}, err
	}
	if hostChanged {
		if previous := hostOfURL(before.Url); previous != "" {
			s.releaseHost(ctx, by, previous)
		}
	}
	s.logger.InfoContext(ctx, "context source updated", slog.String("actor_id", by.ID), slog.String("uid", row.Uid), slog.Bool("credentials_changed", next.credentials))
	return s.Get(ctx, row.Uid)
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, uid string) error {
	row, err := s.row(ctx, uid)
	if err != nil {
		return err
	}
	if row.Provisioned {
		return ErrProvisioned
	}
	var n int64
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if n, err = q.DeleteContextSource(ctx, uid); err != nil || n == 0 {
			return err
		}
		return recordDeleted(ctx, q, row)
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if host := hostOfURL(row.Url); host != "" {
		s.releaseHost(ctx, by, host)
	}
	s.logger.InfoContext(ctx, "context source removed", slog.String("actor_id", by.ID), slog.String("uid", uid))
	return nil
}
