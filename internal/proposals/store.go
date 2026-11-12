package proposals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
	openAuthorKey       = "proposals_open_author_key"
	openRunKey          = "proposals_open_run_key"
	maxTitleRunes       = 200
	maxProposeTrials    = 3
)

type Author struct {
	Kind string
	ID   string
	Name string
}

type Item struct {
	ID             string
	DashboardID    string
	State          State
	Title          string
	Revision       int
	BaseVersion    int
	Summary        string
	Author         Author
	Origin         string
	OriginClient   string
	RunID          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ClosedAt       *time.Time
	ClosedBy       *Author
	MergedVersion  *int
	Behind         bool
	LatestVersion  int
	AllowedActions []Action
}

type head struct {
	row         dbsqlc.Proposal
	baseVersion int
	summary     string
	spec        []byte
	merged      pgtype.Int4
}

type openProposal struct {
	id       pgtype.UUID
	revision int32
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && (pgErr.ConstraintName == openAuthorKey || pgErr.ConstraintName == openRunKey)
}

func isForeignKey(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation && pgErr.ConstraintName == constraint
}

func (s *Service) head(ctx context.Context, id pgtype.UUID) (head, error) {
	row, err := s.db.Queries().GetProposal(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return head{}, ErrNotFound
	}
	if err != nil {
		return head{}, fmt.Errorf("get proposal: %w", err)
	}
	return head{row: row.Proposal, baseVersion: int(row.BaseVersion), summary: row.Summary, spec: row.Spec, merged: row.MergedVersion}, nil
}

func (s *Service) findOpen(ctx context.Context, dashboardID pgtype.UUID, cols actor.Columns) (openProposal, bool, error) {
	q := s.db.Queries()
	if cols.RunID.Valid {
		row, err := q.FindOpenProposalForRun(ctx, dbsqlc.FindOpenProposalForRunParams{DashboardID: dashboardID, RunID: cols.RunID})
		return openFrom(row.ID, row.Revision, err)
	}
	row, err := q.FindOpenProposalForAuthor(ctx, dbsqlc.FindOpenProposalForAuthorParams{DashboardID: dashboardID, AuthorKind: cols.Kind, AuthorID: cols.ID})
	return openFrom(row.ID, row.Revision, err)
}

func lockOpen(ctx context.Context, q *dbsqlc.Queries, dashboardID pgtype.UUID, cols actor.Columns) (openProposal, bool, error) {
	if cols.RunID.Valid {
		row, err := q.LockOpenProposalForRun(ctx, dbsqlc.LockOpenProposalForRunParams{DashboardID: dashboardID, RunID: cols.RunID})
		return openFrom(row.ID, row.Revision, err)
	}
	row, err := q.LockOpenProposalForAuthor(ctx, dbsqlc.LockOpenProposalForAuthorParams{DashboardID: dashboardID, AuthorKind: cols.Kind, AuthorID: cols.ID})
	return openFrom(row.ID, row.Revision, err)
}

func openFrom(id pgtype.UUID, revision int32, err error) (openProposal, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return openProposal{}, false, nil
	}
	if err != nil {
		return openProposal{}, false, fmt.Errorf("find open proposal: %w", err)
	}
	return openProposal{id: id, revision: revision}, true, nil
}

type eventData struct {
	Title       string `json:"title"`
	Revision    int    `json:"revision"`
	BaseVersion *int   `json:"base_version,omitempty"`
}

type mergedEventData struct {
	Title    string `json:"title"`
	Revision int    `json:"revision"`
	Version  *int   `json:"version"`
}

func appendEvent(ctx context.Context, q *dbsqlc.Queries, typ string, who actor.Actor, id, dashboardID pgtype.UUID, data any) error {
	_, err := activity.Append(ctx, q, activity.Entry{
		Type:     typ,
		TargetID: id,
		Owner:    activity.OnDashboard(dashboardID),
		Actor:    who,
		Data:     data,
	}, nil)
	return err
}

func closeProposal(ctx context.Context, q *dbsqlc.Queries, h head, state State, by actor.Columns) error {
	rows, err := q.CloseProposal(ctx, dbsqlc.CloseProposalParams{
		State:        string(state),
		ClosedByKind: pgtype.Text{String: by.Kind, Valid: true},
		ClosedByID:   by.ID,
		ClosedByName: pgtype.Text{String: by.Name, Valid: true},
		ID:           h.row.ID,
		Revision:     h.row.Revision,
	})
	if err != nil {
		return fmt.Errorf("close proposal: %w", err)
	}
	if rows == 0 {
		return errLostClose
	}
	return nil
}

func authorOf(kind string, id pgtype.UUID, name string) Author {
	return Author{Kind: kind, ID: id.String(), Name: name}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	u := t.Time.UTC()
	return &u
}

func itemOf(p dbsqlc.Proposal, baseVersion int, summary string, merged pgtype.Int4) Item {
	item := Item{
		ID:           p.ID.String(),
		DashboardID:  p.DashboardID.String(),
		State:        State(p.State),
		Title:        p.Title,
		Revision:     int(p.Revision),
		BaseVersion:  baseVersion,
		Summary:      summary,
		Author:       authorOf(p.AuthorKind, p.AuthorID, p.AuthorName),
		Origin:       p.Origin.String,
		OriginClient: p.OriginClient.String,
		RunID:        p.RunID.String(),
		CreatedAt:    p.CreatedAt.Time.UTC(),
		UpdatedAt:    p.UpdatedAt.Time.UTC(),
		ClosedAt:     timePtr(p.ClosedAt),
	}
	if p.ClosedByKind.Valid {
		closer := authorOf(p.ClosedByKind.String, p.ClosedByID, p.ClosedByName.String)
		item.ClosedBy = &closer
	}
	if merged.Valid {
		version := int(merged.Int32)
		item.MergedVersion = &version
	}
	return item
}

func truncate(s string, runes int) string {
	if r := []rune(s); len(r) > runes {
		return string(r[:runes])
	}
	return s
}
