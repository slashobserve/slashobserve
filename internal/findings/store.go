package findings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

type store struct {
	db *db.DB
}

func (s *store) insert(ctx context.Context, by auth.Principal, in SaveInput) (Finding, bool, error) {
	creator, err := db.ParseUUID(by.ID)
	if err != nil {
		return Finding{}, false, fmt.Errorf("creator id: %w", err)
	}
	evidence, err := json.Marshal(in.Evidence)
	if err != nil {
		return Finding{}, false, fmt.Errorf("encode evidence: %w", err)
	}
	row, err := s.db.Queries().InsertFinding(ctx, dbsqlc.InsertFindingParams{
		AgentRequestID: in.AgentRequestID,
		DashboardID:    in.DashboardID,
		Title:          in.Title,
		Markdown:       in.Markdown,
		Evidence:       evidence,
		TimeFrom:       pgtype.Timestamptz{Time: in.From, Valid: true},
		TimeTo:         pgtype.Timestamptz{Time: in.To, Valid: true},
		CreatedByKind:  string(by.Kind),
		CreatedByID:    creator,
		CreatedByName:  by.Name,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		existing, found, err := s.forRequest(ctx, in.AgentRequestID)
		if err == nil && !found {
			err = fmt.Errorf("read the finding saved first for agent request %s: %w", in.AgentRequestID.String(), ErrNotFound)
		}
		return existing, true, err
	case db.IsForeignKeyViolation(err):
		return Finding{}, false, ErrNotFound
	case err != nil:
		return Finding{}, false, fmt.Errorf("insert finding: %w", err)
	}
	f, err := findingOf(row, pgtype.Timestamptz{})
	return f, false, err
}

func (s *store) get(ctx context.Context, id pgtype.UUID) (Finding, error) {
	row, err := s.db.Queries().GetFinding(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Finding{}, ErrNotFound
	case err != nil:
		return Finding{}, fmt.Errorf("get finding: %w", err)
	}
	return findingOf(dbsqlc.Finding{
		ID: row.ID, AgentRequestID: row.AgentRequestID, DashboardID: row.DashboardID, Title: row.Title, Markdown: row.Markdown,
		Evidence: row.Evidence, TimeFrom: row.TimeFrom, TimeTo: row.TimeTo, CreatedByKind: row.CreatedByKind,
		CreatedByID: row.CreatedByID, CreatedByName: row.CreatedByName, AnnotationID: row.AnnotationID, CreatedAt: row.CreatedAt,
	}, row.RequestTo)
}

func (s *store) forRequest(ctx context.Context, requestID pgtype.UUID) (Finding, bool, error) {
	row, err := s.db.Queries().GetFindingByAgentRequest(ctx, requestID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Finding{}, false, nil
	case err != nil:
		return Finding{}, false, fmt.Errorf("get finding of agent request: %w", err)
	}
	f, err := findingOf(row, pgtype.Timestamptz{})
	return f, err == nil, err
}

func (s *store) listByDashboard(ctx context.Context, dashboardID pgtype.UUID, after *httpapi.TimeCursor, rows int) ([]Finding, error) {
	params := dbsqlc.ListFindingsByDashboardParams{DashboardID: dashboardID, RowLimit: httpapi.Int32(rows)}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	found, err := s.db.Queries().ListFindingsByDashboard(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list findings of dashboard: %w", err)
	}
	return findingsOf(found)
}

func (s *store) listBy(ctx context.Context, by auth.Principal, visible []pgtype.UUID, after *httpapi.TimeCursor, rows int) ([]Finding, error) {
	creator, err := db.ParseUUID(by.ID)
	if err != nil {
		return nil, fmt.Errorf("creator id: %w", err)
	}
	params := dbsqlc.ListFindingsByParams{CreatedByKind: string(by.Kind), CreatedByID: creator, Folders: visible, RowLimit: httpapi.Int32(rows)}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	found, err := s.db.Queries().ListFindingsBy(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list findings of creator: %w", err)
	}
	return findingsOf(found)
}

func (s *store) setAnnotation(ctx context.Context, id, annotationID pgtype.UUID) (bool, error) {
	n, err := s.db.Queries().SetFindingAnnotation(ctx, dbsqlc.SetFindingAnnotationParams{ID: id, AnnotationID: annotationID})
	if err != nil {
		return false, fmt.Errorf("set finding annotation: %w", err)
	}
	return n == 1, nil
}

func (s *store) annotation(ctx context.Context, id pgtype.UUID) (annotations.Annotation, error) {
	r, err := s.db.Queries().GetAnnotation(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return annotations.Annotation{}, ErrNotFound
	case err != nil:
		return annotations.Annotation{}, fmt.Errorf("get annotation of finding: %w", err)
	}
	a := annotations.Annotation{
		ID:        r.ID.String(),
		PanelID:   r.PanelID,
		From:      r.TimeFrom.Time.UTC(),
		Text:      r.Text,
		Tags:      r.Tags,
		CreatedAt: r.CreatedAt.Time.UTC(),
	}
	if r.DashboardID.Valid {
		a.DashboardID = r.DashboardID.String()
	}
	if r.TimeTo.Valid {
		to := r.TimeTo.Time.UTC()
		a.To = &to
	}
	if r.CreatedBy.Valid {
		a.CreatedBy = r.CreatedBy.String()
	}
	if a.Tags == nil {
		a.Tags = []string{}
	}
	return a, nil
}

func findingsOf(rows []dbsqlc.Finding) ([]Finding, error) {
	out := make([]Finding, len(rows))
	for i, r := range rows {
		f, err := findingOf(r, pgtype.Timestamptz{})
		if err != nil {
			return nil, err
		}
		out[i] = f
	}
	return out, nil
}

func findingOf(r dbsqlc.Finding, requestTo pgtype.Timestamptz) (Finding, error) {
	f := Finding{
		ID:        r.ID.String(),
		Title:     r.Title,
		Markdown:  r.Markdown,
		From:      r.TimeFrom.Time.UTC(),
		To:        r.TimeTo.Time.UTC(),
		CreatedAt: r.CreatedAt.Time.UTC(),
		CreatedBy: Author{Kind: r.CreatedByKind, ID: r.CreatedByID.String(), Name: r.CreatedByName},
	}
	if err := json.Unmarshal(r.Evidence, &f.Evidence); err != nil {
		return Finding{}, fmt.Errorf("decode evidence of finding %s: %w", f.ID, err)
	}
	if r.AgentRequestID.Valid {
		f.AgentRequestID = r.AgentRequestID.String()
	}
	if r.DashboardID.Valid {
		f.DashboardID = r.DashboardID.String()
	}
	if r.AnnotationID.Valid {
		f.AnnotationID = r.AnnotationID.String()
	}
	if requestTo.Valid {
		t := requestTo.Time.UTC()
		f.RequestTo = &t
	}
	return f, nil
}
