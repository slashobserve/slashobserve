package observations_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/placement"
)

type unrecordableDashboards struct {
	*fakeDashboards
	deleted []string
}

func (u *unrecordableDashboards) CreateCopy(ctx context.Context, by auth.Principal, raw []byte, summary string, from placement.Copied) (dashboards.Dashboard, dashboards.Version, error) {
	d, v, err := u.fakeDashboards.CreateCopy(ctx, by, raw, summary, from)
	d.ID = uuid.NewString()
	return d, v, err
}

func (u *unrecordableDashboards) Delete(ctx context.Context, _ auth.Principal, id pgtype.UUID) error {
	u.deleted = append(u.deleted, id.String())
	return nil
}

type unrecordableAnnotations struct {
	deleted []string
}

func (u *unrecordableAnnotations) Create(context.Context, auth.Principal, annotations.CreateInput) (annotations.Annotation, error) {
	return annotations.Annotation{ID: uuid.NewString()}, nil
}

func (u *unrecordableAnnotations) Delete(_ context.Context, _ auth.Principal, id pgtype.UUID) error {
	u.deleted = append(u.deleted, id.String())
	return nil
}

func TestAPinThatCannotBeRecordedDeletesItsAnnotation(t *testing.T) {
	r := newRig(t)
	notes := &unrecordableAnnotations{}
	service := observations.New(r.db.DB, folders.NewAccess(r.db.DB), r.freezer, nil, slog.New(slog.DiscardHandler), observations.WithOutputs(r.made, notes))
	about := r.dashboard(t)
	o := r.create(t, editor, r.shared)

	if _, _, err := service.Pin(reading(t.Context()), editor, mustUUID(t, o.ID), about); err == nil {
		t.Fatal("pin: want the failed record to be an error")
	}
	if len(notes.deleted) != 1 {
		t.Fatalf("annotations deleted after the failed record: %v, want the one created", notes.deleted)
	}
}

func TestADashboardThatCannotBeRecordedIsDeleted(t *testing.T) {
	r := newRig(t)
	made := &unrecordableDashboards{fakeDashboards: r.made}
	service := observations.New(r.db.DB, folders.NewAccess(r.db.DB), r.freezer, nil, slog.New(slog.DiscardHandler), observations.WithOutputs(made, annotations.New(r.db.DB, folders.NewAccess(r.db.DB))))
	o := r.create(t, editor, r.shared)
	r.addPanel(t, editor, o)

	ctx, cancel := context.WithTimeout(reading(t.Context()), time.Minute)
	defer cancel()
	if _, _, _, err := service.MakeDashboard(ctx, editor, mustUUID(t, o.ID), pgtype.UUID{}, false); err == nil {
		t.Fatal("make: want the failed record to be an error")
	}
	if len(made.deleted) != 1 {
		t.Fatalf("dashboards deleted after the failed record: %v, want the one created", made.deleted)
	}
}
