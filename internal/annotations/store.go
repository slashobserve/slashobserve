package annotations

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/placement"
)

type store struct {
	db *db.DB
}

func (s *store) insert(ctx context.Context, in CreateInput, by actor.Actor) (Annotation, error) {
	cols, err := by.Columns()
	if err != nil {
		return Annotation{}, err
	}
	var created Annotation
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := placement.AnswerOnDashboard(ctx, q, in.AnswerOf, in.DashboardID); err != nil {
			return err
		}
		if err := placement.RunWriteOnDashboard(ctx, q, by, in.DashboardID); err != nil {
			return err
		}
		copied, err := placement.CopyOnDashboard(ctx, q, in.CopyOf, in.DashboardID)
		if err != nil {
			return err
		}
		row, err := q.InsertAnnotation(ctx, dbsqlc.InsertAnnotationParams{
			DashboardID:            in.DashboardID,
			PanelID:                in.PanelID,
			TimeFrom:               timestamp(&in.From),
			TimeTo:                 timestamp(in.To),
			Text:                   in.Text,
			Tags:                   in.Tags,
			CreatedBy:              cols.ID,
			CreatedByKind:          pgtype.Text{String: cols.Kind, Valid: true},
			Origin:                 cols.Origin,
			CopiedRestrictedAnswer: copied,
			OriginClient:           cols.Client,
		})
		if err != nil {
			return err
		}
		created = annotationOf(row)
		_, err = activity.Append(ctx, q, activity.Entry{
			Type:     activity.TypeAnnotationCreated,
			TargetID: row.ID,
			Owner:    activityOwner(row.DashboardID),
			Actor:    by,
			Data:     newCreatedData(created),
		}, nil)
		return err
	})
	if db.IsForeignKeyViolation(err) {
		return Annotation{}, ErrNotFound
	}
	if err != nil {
		return Annotation{}, err
	}
	return created, nil
}

const maxEventTitleRunes = 80

type createdData struct {
	Title    string   `json:"title,omitempty"`
	PanelID  string   `json:"panel_id,omitempty"`
	Tags     []string `json:"tags"`
	TimeFrom string   `json:"time_from"`
	TimeTo   string   `json:"time_to,omitempty"`
}

func eventTitle(text string) string {
	line := strings.Join(strings.Fields(text), " ")
	runes := []rune(line)
	if len(runes) <= maxEventTitleRunes {
		return line
	}
	return string(runes[:maxEventTitleRunes-1]) + "…"
}

func newCreatedData(a Annotation) createdData {
	d := createdData{Title: eventTitle(a.Text), PanelID: a.PanelID, Tags: a.Tags, TimeFrom: httpapi.FormatTime(a.From)}
	if a.To != nil {
		d.TimeTo = httpapi.FormatTime(*a.To)
	}
	return d
}

func (s *store) get(ctx context.Context, id pgtype.UUID) (Annotation, error) {
	row, err := s.db.Queries().GetAnnotation(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Annotation{}, ErrNotFound
	}
	if err != nil {
		return Annotation{}, err
	}
	return annotationOf(row), nil
}

func (s *store) named(ctx context.Context, id pgtype.UUID) (Annotation, error) {
	row, err := s.db.Queries().GetAnnotationNamed(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Annotation{}, ErrNotFound
	}
	if err != nil {
		return Annotation{}, err
	}
	return namedAnnotationOf(dbsqlc.ListAnnotationsRow(row)), nil
}

func (s *store) update(ctx context.Context, id pgtype.UUID, text string, tags []string, by actor.Actor) (Annotation, error) {
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		n, err := q.UpdateAnnotation(ctx, dbsqlc.UpdateAnnotationParams{ID: id, Text: text, Tags: tags})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		row, err := q.GetAnnotation(ctx, id)
		if err != nil {
			return err
		}
		if err := placement.RunWriteOnDashboard(ctx, q, by, row.DashboardID); err != nil {
			return err
		}
		_, err = activity.Append(ctx, q, activity.Entry{
			Type:     activity.TypeAnnotationUpdated,
			TargetID: id,
			Owner:    activityOwner(row.DashboardID),
			Actor:    by,
			Data:     updatedData{Tags: tags},
		}, nil)
		return err
	})
	if err != nil {
		return Annotation{}, err
	}
	return s.named(ctx, id)
}

type updatedData struct {
	Tags []string `json:"tags"`
}

func activityOwner(dashboardID pgtype.UUID) activity.Owner {
	if dashboardID.Valid {
		return activity.OnDashboard(dashboardID)
	}
	return activity.General()
}

func (s *store) remove(ctx context.Context, id pgtype.UUID, by actor.Actor) error {
	return s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		row, err := q.GetAnnotation(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := activity.Append(ctx, q, activity.Entry{
			Type:     activity.TypeAnnotationDeleted,
			TargetID: id,
			Owner:    activityOwner(row.DashboardID),
			Actor:    by,
			Data:     newCreatedData(annotationOf(row)),
		}, nil); err != nil {
			return err
		}
		n, err := q.DeleteAnnotation(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *store) list(ctx context.Context, q ListQuery, limit int) ([]Annotation, error) {
	params := dbsqlc.ListAnnotationsParams{
		DashboardID: q.DashboardID,
		WindowFrom:  timestamp(q.From),
		WindowTo:    timestamp(q.To),
		Tags:        q.Tags,
		RowLimit:    httpapi.Int32(limit),
	}
	if params.Tags == nil {
		params.Tags = []string{}
	}
	if q.After != nil {
		params.CursorTime = timestamp(&q.After.Time)
		params.CursorID = q.After.ID
	}
	rows, err := s.db.Queries().ListAnnotations(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]Annotation, 0, len(rows))
	for _, r := range rows {
		out = append(out, namedAnnotationOf(r))
	}
	return out, nil
}

func namedAnnotationOf(r dbsqlc.ListAnnotationsRow) Annotation {
	a := annotationOf(dbsqlc.Annotation{
		ID:            r.ID,
		DashboardID:   r.DashboardID,
		PanelID:       r.PanelID,
		TimeFrom:      r.TimeFrom,
		TimeTo:        r.TimeTo,
		Text:          r.Text,
		Tags:          r.Tags,
		CreatedBy:     r.CreatedBy,
		CreatedAt:     r.CreatedAt,
		CreatedByKind: r.CreatedByKind,
		Origin:        r.Origin,
		OriginClient:  r.OriginClient,
	})
	a.CreatedByName = r.CreatedByName
	return a
}

func timestamp(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func annotationOf(r dbsqlc.Annotation) Annotation {
	a := Annotation{
		ID:            r.ID.String(),
		PanelID:       r.PanelID,
		From:          r.TimeFrom.Time.UTC(),
		Text:          r.Text,
		Tags:          r.Tags,
		CreatedAt:     r.CreatedAt.Time.UTC(),
		CreatedByKind: r.CreatedByKind.String,
		Origin:        origin.Origin{Kind: origin.Kind(r.Origin.String), Client: r.OriginClient.String},
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
	return a
}
