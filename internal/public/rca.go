package public

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

const maxLinksPerObservation = 20

var (
	ErrRCAPersonalFolder = errors.New("an observation in My Dashboards cannot have a public link")
	ErrRCAUserScoped     = errors.New("a snapshot in this RCA shows data that depends on who is viewing, so it cannot be shared publicly")
	ErrTooManyRCALinks   = fmt.Errorf("an observation has at most %d public links; revoke one first", maxLinksPerObservation)
)

type RCAs interface {
	Access(ctx context.Context, by auth.Principal, id pgtype.UUID) (observations.Placement, error)
	GetRCA(ctx context.Context, by auth.Principal, observationID pgtype.UUID, revision int) (observations.RCA, error)
	RCASnapshotFrames(ctx context.Context, observationID pgtype.UUID, revision int, snapshotID pgtype.UUID) (json.RawMessage, error)
}

func WithRCAs(r RCAs) Option {
	return func(s *Service) { s.rcas = r }
}

type RCALink struct {
	ID           string
	Revision     int
	Prefix       string
	ExpiresAt    *time.Time
	CreatedBy    actor.Actor
	CreatedAt    time.Time
	LastViewedAt *time.Time
	Expired      bool
}

type CreateRCAInput struct {
	ExpiresAt *time.Time
}

type CreatedRCA struct {
	Link  RCALink
	Token string
	URL   string
}

type PublicRCA struct {
	Revision    int
	Title       string
	Summary     string
	State       string
	From, To    *time.Time
	Blocks      json.RawMessage
	PublishedBy string
	CreatedAt   time.Time
}

type ResolvedRCA struct {
	LinkID        pgtype.UUID
	ObservationID pgtype.UUID
	Revision      int
	ExpiresAt     *time.Time
	row           dbsqlc.RCAPublicLinkByTokenHashRow
}

func (s *Service) manageRCA(ctx context.Context, by auth.Principal, observationID pgtype.UUID) error {
	placement, err := s.rcas.Access(ctx, by, observationID)
	switch {
	case err != nil:
		return err
	case placement.Personal:
		return ErrRCAPersonalFolder
	case !placement.Abilities.Manage:
		return folders.ErrForbidden
	}
	return nil
}

func rcaLinkOf(row dbsqlc.ListRCAPublicLinksRow, revision int, now time.Time) RCALink {
	return RCALink{
		ID:           row.ID.String(),
		Revision:     revision,
		Prefix:       row.DisplayPrefix,
		ExpiresAt:    timeOf(row.ExpiresAt),
		CreatedBy:    actor.Actor{Kind: auth.PrincipalKind(row.CreatedByKind), ID: row.CreatedByID.String(), Name: row.CreatedByName},
		CreatedAt:    row.CreatedAt.Time,
		LastViewedAt: timeOf(row.LastViewedAt),
		Expired:      expired(timeOf(row.ExpiresAt), now),
	}
}

func (s *Service) CreateRCALink(ctx context.Context, by auth.Principal, observationID pgtype.UUID, revision int, in CreateRCAInput) (CreatedRCA, error) {
	if err := s.manageRCA(ctx, by, observationID); err != nil {
		return CreatedRCA{}, err
	}
	rca, err := s.rcas.GetRCA(ctx, by, observationID, revision)
	if err != nil {
		return CreatedRCA{}, err
	}
	rcaID, err := db.ParseUUID(rca.ID)
	if err != nil {
		return CreatedRCA{}, err
	}
	q := s.db.Queries()
	scoped, err := q.RCAHasUserScopedSnapshot(ctx, rcaID)
	if err != nil {
		return CreatedRCA{}, err
	}
	if scoped {
		return CreatedRCA{}, ErrRCAUserScoped
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(s.now()) {
		return CreatedRCA{}, ErrInvalidExpiry
	}
	count, err := q.CountObservationPublicLinks(ctx, observationID)
	if err != nil {
		return CreatedRCA{}, err
	}
	if count >= maxLinksPerObservation {
		return CreatedRCA{}, ErrTooManyRCALinks
	}
	creatorID, err := db.ParseUUID(by.ID)
	if err != nil {
		return CreatedRCA{}, folders.ErrForbidden
	}
	token, hash, err := opaquetoken.New(tokenPrefix)
	if err != nil {
		return CreatedRCA{}, err
	}
	row, err := q.CreateRCAPublicLink(ctx, dbsqlc.CreateRCAPublicLinkParams{
		RcaID:         rcaID,
		TokenHash:     hash,
		DisplayPrefix: displayPrefix(token),
		ExpiresAt:     timestamp(in.ExpiresAt),
		CreatedByKind: string(by.Kind),
		CreatedByID:   creatorID,
		CreatedByName: creatorName(by),
	})
	if db.IsForeignKeyViolation(err) {
		return CreatedRCA{}, observations.ErrRevisionNotFound
	}
	if err != nil {
		return CreatedRCA{}, err
	}
	link := rcaLinkOf(dbsqlc.ListRCAPublicLinksRow(row), revision, s.now())
	return CreatedRCA{Link: link, Token: token, URL: links.RCA(s.publicURL, token)}, nil
}

func (s *Service) ListRCALinks(ctx context.Context, by auth.Principal, observationID pgtype.UUID, revision int) ([]RCALink, error) {
	if err := s.manageRCA(ctx, by, observationID); err != nil {
		return nil, err
	}
	rca, err := s.rcas.GetRCA(ctx, by, observationID, revision)
	if err != nil {
		return nil, err
	}
	rcaID, err := db.ParseUUID(rca.ID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Queries().ListRCAPublicLinks(ctx, rcaID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := make([]RCALink, len(rows))
	for i, row := range rows {
		out[i] = rcaLinkOf(row, revision, now)
	}
	return out, nil
}

func (s *Service) DeleteRCALink(ctx context.Context, by auth.Principal, observationID, linkID pgtype.UUID) error {
	if err := s.manageRCA(ctx, by, observationID); err != nil {
		return err
	}
	n, err := s.db.Queries().DeleteRCAPublicLink(ctx, dbsqlc.DeleteRCAPublicLinkParams{LinkID: linkID, ObservationID: observationID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ResolveRCA(ctx context.Context, token string) (ResolvedRCA, error) {
	if !validTokenShape(token) {
		return ResolvedRCA{}, ErrNotFound
	}
	row, err := s.db.Queries().RCAPublicLinkByTokenHash(ctx, opaquetoken.Hash(token))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ResolvedRCA{}, ErrNotFound
	case err != nil:
		return ResolvedRCA{}, err
	case row.Personal, expired(timeOf(row.ExpiresAt), s.now()):
		return ResolvedRCA{}, ErrNotFound
	}
	return ResolvedRCA{LinkID: row.LinkID, ObservationID: row.ObservationID, Revision: int(row.Revision), ExpiresAt: timeOf(row.ExpiresAt), row: row}, nil
}

func (s *Service) ViewRCA(ctx context.Context, token string) (PublicRCA, error) {
	r, err := s.ResolveRCA(ctx, token)
	if err != nil {
		return PublicRCA{}, err
	}
	var content struct {
		Blocks json.RawMessage `json:"blocks"`
	}
	if err := json.Unmarshal(r.row.Content, &content); err != nil {
		return PublicRCA{}, fmt.Errorf("decode RCA content: %w", err)
	}
	if len(content.Blocks) == 0 {
		content.Blocks = json.RawMessage("[]")
	}
	if err := s.db.Queries().TouchPublicLink(ctx, r.LinkID); err != nil {
		s.logger.WarnContext(ctx, "record a public link view", logattrs.Failure("internal", err, slog.String("public_link_id", r.LinkID.String()))...)
	}
	return PublicRCA{
		Revision:    r.Revision,
		Title:       r.row.Title,
		Summary:     r.row.Summary.String,
		State:       r.row.ObservationState,
		From:        timeOf(r.row.TimeFrom),
		To:          timeOf(r.row.TimeTo),
		Blocks:      content.Blocks,
		PublishedBy: r.row.CreatedByName,
		CreatedAt:   r.row.CreatedAt.Time,
	}, nil
}

func (s *Service) ViewRCASnapshot(ctx context.Context, token string, snapshotID pgtype.UUID) (json.RawMessage, error) {
	r, err := s.ResolveRCA(ctx, token)
	if err != nil {
		return nil, err
	}
	frames, err := s.rcas.RCASnapshotFrames(ctx, r.ObservationID, r.Revision, snapshotID)
	if errors.Is(err, observations.ErrNotFound) || errors.Is(err, observations.ErrRevisionNotFound) {
		return nil, ErrNotFound
	}
	return frames, err
}
