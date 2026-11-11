package findings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

type store struct {
	db *db.DB
}

func (s *store) insert(ctx context.Context, by actor.Actor, in SaveInput) (Finding, bool, error) {
	cols, err := by.Columns()
	if err != nil {
		return Finding{}, false, err
	}
	evidence, err := json.Marshal(in.Evidence)
	if err != nil {
		return Finding{}, false, fmt.Errorf("encode evidence: %w", err)
	}
	var row dbsqlc.Finding
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		row, err = q.InsertFinding(ctx, dbsqlc.InsertFindingParams{
			AgentRequestID: in.AgentRequestID,
			DashboardID:    in.DashboardID,
			FolderID:       in.FolderID,
			Title:          in.Title,
			Markdown:       in.Markdown,
			Evidence:       evidence,
			TimeFrom:       pgtype.Timestamptz{Time: in.From, Valid: true},
			TimeTo:         pgtype.Timestamptz{Time: in.To, Valid: true},
			CreatedByKind:  cols.Kind,
			CreatedByID:    cols.ID,
			CreatedByName:  cols.Name,
			Origin:         cols.Origin,
			OriginClient:   cols.Client,
		})
		if err != nil {
			return err
		}
		data := map[string]string{"title": row.Title}
		if row.AgentRequestID.Valid {
			data["agent_request_id"] = row.AgentRequestID.String()
		}
		_, err = activity.Append(ctx, q, activity.Entry{
			Type:     activity.TypeFindingCreated,
			TargetID: row.ID,
			Owner:    activityOwner(row.DashboardID, row.ID),
			Actor:    by,
			Data:     data,
		}, nil)
		return err
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
		ID: row.ID, AgentRequestID: row.AgentRequestID, DashboardID: row.DashboardID, FolderID: row.FolderID, Title: row.Title, Markdown: row.Markdown,
		Evidence: row.Evidence, TimeFrom: row.TimeFrom, TimeTo: row.TimeTo, CreatedByKind: row.CreatedByKind,
		CreatedByID: row.CreatedByID, CreatedByName: row.CreatedByName, AnnotationID: row.AnnotationID, CreatedAt: row.CreatedAt,
		Origin: row.Origin, OriginClient: row.OriginClient,
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

func (s *store) listInFolder(ctx context.Context, folderID pgtype.UUID, after *httpapi.TimeCursor, rows int) ([]Finding, error) {
	params := dbsqlc.ListFindingsInFolderParams{FolderID: folderID, RowLimit: httpapi.Int32(rows)}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	found, err := s.db.Queries().ListFindingsInFolder(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list findings of folder: %w", err)
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

func activityOwner(dashboardID, findingID pgtype.UUID) activity.Owner {
	if dashboardID.Valid {
		return activity.OnDashboard(dashboardID)
	}
	return activity.OnFinding(findingID)
}

func (s *store) setAnnotation(ctx context.Context, by actor.Actor, id, dashboardID, annotationID pgtype.UUID) (bool, error) {
	won := false
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		n, err := q.SetFindingAnnotation(ctx, dbsqlc.SetFindingAnnotationParams{ID: id, AnnotationID: annotationID})
		if err != nil || n == 0 {
			return err
		}
		won = true
		_, err = activity.Append(ctx, q, activity.Entry{
			Type:     activity.TypeFindingPinned,
			TargetID: id,
			Owner:    activityOwner(dashboardID, id),
			Actor:    by,
			Data:     map[string]string{"annotation_id": annotationID.String()},
		}, nil)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("set finding annotation: %w", err)
	}
	return won, nil
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
		Actor: actor.FromColumns(actor.Columns{
			Kind: r.CreatedByKind, ID: r.CreatedByID, Name: r.CreatedByName,
			Origin: r.Origin, Client: r.OriginClient, RunID: r.AgentRequestID,
		}),
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
	if r.FolderID.Valid {
		f.FolderID = r.FolderID.String()
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
