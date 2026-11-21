package auditfolder

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	Title     = "Audit"
	lockName  = "audit_folder"
	FolderKey = "audit/folder"
	summary   = "Starter dashboard added by the server to the Audit folder"
)

//go:embed dashboards/*.json
var embedded embed.FS

type Starter struct {
	Key   string
	Title string
	Spec  []byte
}

func Starters() ([]Starter, error) {
	names, err := fs.Glob(embedded, "dashboards/*.json")
	if err != nil {
		return nil, err
	}
	out := make([]Starter, 0, len(names))
	for _, name := range names {
		raw, err := embedded.ReadFile(name)
		if err != nil {
			return nil, err
		}
		var head struct{ Title string }
		if err := json.Unmarshal(raw, &head); err != nil {
			return nil, fmt.Errorf("starter dashboard %q: %w", name, err)
		}
		out = append(out, Starter{Key: "audit/dashboards/" + strings.TrimSuffix(path.Base(name), ".json"), Title: head.Title, Spec: raw})
	}
	return out, nil
}

type Seeder struct {
	db         *db.DB
	dashboards *dashboards.Service
	folders    *folders.Access
	logger     *slog.Logger
	by         auth.Principal
}

func New(database *db.DB, dash *dashboards.Service, access *folders.Access, by auth.Principal, logger *slog.Logger) *Seeder {
	return &Seeder{db: database, dashboards: dash, folders: access, by: by, logger: logger}
}

func (s *Seeder) Seed(ctx context.Context) error {
	starters, err := Starters()
	if err != nil {
		return err
	}
	ctx = datasource.WithAccess(ctx, datasource.Access{System: true})
	return s.db.WithSessionLock(ctx, lockName, func(ctx context.Context) error {
		folderID, ok, err := s.folder(ctx)
		if err != nil || !ok {
			return err
		}
		var errs []error
		for _, st := range starters {
			if err := s.dashboard(ctx, folderID, st); err != nil {
				errs = append(errs, fmt.Errorf("starter dashboard %q: %w", st.Title, err))
			}
		}
		return errors.Join(errs...)
	})
}

func (s *Seeder) folder(ctx context.Context) (pgtype.UUID, bool, error) {
	q := s.db.Queries()
	seeded, err := q.StarterContent(ctx, FolderKey)
	switch {
	case err == nil:
		return s.seededFolder(ctx, q, seeded)
	case !errors.Is(err, pgx.ErrNoRows):
		return pgtype.UUID{}, false, err
	}
	existing, err := q.FolderByTitle(ctx, Title)
	switch {
	case err == nil:
		return s.adoptFolder(ctx, q, existing)
	case !errors.Is(err, pgx.ErrNoRows):
		return pgtype.UUID{}, false, err
	}
	id, err := s.folders.CreateAdminsOnlyFolder(ctx, s.by, Title)
	if err != nil {
		return pgtype.UUID{}, false, err
	}
	return id, true, q.RecordStarterContent(ctx, dbsqlc.RecordStarterContentParams{Key: FolderKey, Ref: id})
}

func (s *Seeder) seededFolder(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID) (pgtype.UUID, bool, error) {
	row, err := q.GetFolder(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		s.logger.DebugContext(ctx, "the Audit folder was deleted, so its starter dashboards are not added again")
		return pgtype.UUID{}, false, nil
	}
	if err != nil {
		return pgtype.UUID{}, false, err
	}
	return s.restricted(ctx, row)
}

func (s *Seeder) adoptFolder(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID) (pgtype.UUID, bool, error) {
	row, err := q.GetFolder(ctx, id)
	if err != nil {
		return pgtype.UUID{}, false, err
	}
	if _, ok, err := s.restricted(ctx, row); err != nil || !ok {
		return pgtype.UUID{}, false, err
	}
	return row.ID, true, q.RecordStarterContent(ctx, dbsqlc.RecordStarterContentParams{Key: FolderKey, Ref: row.ID})
}

func (s *Seeder) restricted(ctx context.Context, row dbsqlc.Folder) (pgtype.UUID, bool, error) {
	if !folders.AdminsOnly(row) {
		s.logger.WarnContext(ctx, "the folder named Audit is not Admins only, so the starter audit dashboards are not added; make it Admins only to get them on the next start", slog.String("folder_id", row.ID.String()))
		return pgtype.UUID{}, false, nil
	}
	return row.ID, true, nil
}

func (s *Seeder) dashboard(ctx context.Context, folderID pgtype.UUID, st Starter) error {
	q := s.db.Queries()
	if _, err := q.StarterContent(ctx, st.Key); !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	same, err := q.ApplyCandidates(ctx, dbsqlc.ApplyCandidatesParams{FolderID: folderID, Title: st.Title})
	if err != nil {
		return err
	}
	if len(same) > 0 {
		return q.RecordStarterContent(ctx, dbsqlc.RecordStarterContentParams{Key: st.Key, Ref: same[0]})
	}
	dash, err := s.dashboards.CreateStarter(ctx, s.by, folderID, st.Spec, summary, func(q *dbsqlc.Queries, dash dashboards.Dashboard) error {
		id, err := db.ParseUUID(dash.ID)
		if err != nil {
			return err
		}
		return q.RecordStarterContent(ctx, dbsqlc.RecordStarterContentParams{Key: st.Key, Ref: id})
	})
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "added a starter audit dashboard", slog.String("dashboard_id", dash.ID))
	return nil
}

func (s *Seeder) Start(ctx context.Context) {
	if err := s.Seed(ctx); err != nil && ctx.Err() == nil {
		s.logger.WarnContext(ctx, "could not add the Audit folder's starter dashboards; the next start tries again", logattrs.Failure("audit_folder_failed", err)...)
	}
}
