package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxEventAuthorNameRunes = 256

var errStaleParent = errors.New("parent version is no longer the latest")

type Author struct {
	Kind  string
	ID    string
	Name  string
	Email string
}

type Dashboard struct {
	ID            string
	Title         string
	Tags          []string
	LatestVersion int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	UpdatedBy     Author
}

type VersionMeta struct {
	Number        int
	ParentVersion *int
	RestoredFrom  *int
	Summary       string
	Author        Author
	CreatedAt     time.Time
}

type Version struct {
	VersionMeta
	Spec *spec.Dashboard
}

type newVersion struct {
	dashboardID  pgtype.UUID
	parent       int
	restoredFrom *int
	spec         *spec.Dashboard
	canonical    []byte
	summary      string
	author       Author
}

type store struct {
	db *db.DB
}

func (st store) create(ctx context.Context, in newVersion) (Dashboard, Version, error) {
	var dash Dashboard
	var ver Version
	err := st.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		row, err := q.CreateDashboard(ctx, dbsqlc.CreateDashboardParams{Title: in.spec.Title, Tags: tagsOf(in.spec)})
		if err != nil {
			return fmt.Errorf("create dashboard: %w", err)
		}
		in.dashboardID = row.ID
		meta, err := insertVersion(ctx, q, in, 1)
		if err != nil {
			return err
		}
		dash, ver = dashboardFromRow(row, in.author), Version{VersionMeta: meta, Spec: in.spec}
		return nil
	})
	return dash, ver, err
}

func (st store) addVersion(ctx context.Context, in newVersion) (Dashboard, Version, error) {
	var dash Dashboard
	var ver Version
	err := st.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		row, err := q.BumpDashboardVersion(ctx, dbsqlc.BumpDashboardVersionParams{
			ID:              in.dashboardID,
			Title:           in.spec.Title,
			Tags:            tagsOf(in.spec),
			ExpectedVersion: toInt32(in.parent),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errStaleParent
		}
		if err != nil {
			return fmt.Errorf("bump dashboard version: %w", err)
		}
		meta, err := insertVersion(ctx, q, in, int(row.LatestVersion))
		if err != nil {
			return err
		}
		dash, ver = dashboardFromRow(row, in.author), Version{VersionMeta: meta, Spec: in.spec}
		return nil
	})
	return dash, ver, err
}

func insertVersion(ctx context.Context, q *dbsqlc.Queries, in newVersion, number int) (VersionMeta, error) {
	authorID, err := db.ParseUUID(in.author.ID)
	if err != nil {
		return VersionMeta{}, fmt.Errorf("author id: %w", err)
	}
	params := dbsqlc.InsertDashboardVersionParams{
		DashboardID: in.dashboardID,
		Version:     toInt32(number),
		Spec:        in.canonical,
		Summary:     in.summary,
		AuthorKind:  in.author.Kind,
		AuthorID:    authorID,
		AuthorName:  in.author.Name,
		AuthorEmail: pgtype.Text{String: in.author.Email, Valid: in.author.Email != ""},
	}
	if number > 1 {
		params.ParentVersion = pgtype.Int4{Int32: toInt32(in.parent), Valid: true}
	}
	if in.restoredFrom != nil {
		params.RestoredFrom = pgtype.Int4{Int32: toInt32(*in.restoredFrom), Valid: true}
	}
	row, err := q.InsertDashboardVersion(ctx, params)
	if err != nil {
		return VersionMeta{}, fmt.Errorf("insert dashboard version: %w", err)
	}
	meta := metaFromRow(row.Version, row.ParentVersion, row.RestoredFrom, row.Summary, authorFromRow(row.AuthorKind, row.AuthorID, row.AuthorName, row.AuthorEmail), row.CreatedAt)
	payload, err := versionEventPayload(in.dashboardID, meta)
	if err != nil {
		return VersionMeta{}, fmt.Errorf("marshal event: %w", err)
	}
	if err := q.NotifyEvent(ctx, dbsqlc.NotifyEventParams{Channel: events.Channel, Payload: string(payload)}); err != nil {
		return VersionMeta{}, fmt.Errorf("notify event: %w", err)
	}
	return meta, nil
}

func capEventAuthorName(name string) string {
	if r := []rune(name); len(r) > maxEventAuthorNameRunes {
		return string(r[:maxEventAuthorNameRunes])
	}
	return name
}

// pg_notify refuses payloads of 8000 bytes or more. With the summary capped at 500 characters and the
// name at 256, even all-escaped characters (6 bytes each) leave this payload under 5000 bytes.
func versionEventPayload(dashboardID pgtype.UUID, meta VersionMeta) ([]byte, error) {
	return json.Marshal(events.Event{
		Type:          events.TypeVersionCreated,
		DashboardID:   dashboardID.String(),
		Version:       meta.Number,
		ParentVersion: meta.ParentVersion,
		RestoredFrom:  meta.RestoredFrom,
		Summary:       meta.Summary,
		Author:        &events.Author{Kind: meta.Author.Kind, ID: meta.Author.ID, Name: capEventAuthorName(meta.Author.Name)},
		CreatedAt:     formatTime(meta.CreatedAt),
	})
}

func (st store) latestVersion(ctx context.Context, id pgtype.UUID) (int, error) {
	n, err := st.db.Queries().GetDashboardLatestVersion(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get latest version: %w", err)
	}
	return int(n), nil
}

func (st store) latest(ctx context.Context, id pgtype.UUID) (Dashboard, Version, error) {
	row, err := st.db.Queries().GetDashboard(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Dashboard{}, Version{}, ErrNotFound
	}
	if err != nil {
		return Dashboard{}, Version{}, fmt.Errorf("get dashboard: %w", err)
	}
	v := row.DashboardVersion
	dash := dashboardFromRow(row.Dashboard, authorFromRow(v.AuthorKind, v.AuthorID, v.AuthorName, v.AuthorEmail))
	ver, err := versionFromRow(v)
	return dash, ver, err
}

func (st store) version(ctx context.Context, id pgtype.UUID, number int) (Dashboard, Version, error) {
	row, err := st.db.Queries().GetDashboardVersion(ctx, dbsqlc.GetDashboardVersionParams{ID: id, Version: toInt32(number)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Dashboard{}, Version{}, ErrNotFound
	}
	if err != nil {
		return Dashboard{}, Version{}, fmt.Errorf("get dashboard version: %w", err)
	}
	dash := dashboardFromRow(row.Dashboard, authorFromRow(row.UpdatedByKind, row.UpdatedByID, row.UpdatedByName, row.UpdatedByEmail))
	ver, err := versionFromRow(row.DashboardVersion)
	return dash, ver, err
}

func (st store) specBytes(ctx context.Context, id pgtype.UUID, number int) ([]byte, error) {
	raw, err := st.db.Queries().GetDashboardVersionSpec(ctx, dbsqlc.GetDashboardVersionSpecParams{ID: id, Version: toInt32(number)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get version spec: %w", err)
	}
	return raw, nil
}

func (st store) list(ctx context.Context, after *dashboardCursor, limit int) ([]Dashboard, error) {
	params := dbsqlc.ListDashboardsParams{RowLimit: toInt32(limit)}
	if after != nil {
		params.CursorUpdatedAt = pgtype.Timestamptz{Time: after.updatedAt, Valid: true}
		params.CursorID = after.id
	}
	rows, err := st.db.Queries().ListDashboards(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list dashboards: %w", err)
	}
	items := make([]Dashboard, 0, len(rows))
	for _, r := range rows {
		items = append(items, dashboardFromRow(r.Dashboard, authorFromRow(r.UpdatedByKind, r.UpdatedByID, r.UpdatedByName, r.UpdatedByEmail)))
	}
	return items, nil
}

func (st store) listVersions(ctx context.Context, id pgtype.UUID, before, limit int) ([]VersionMeta, error) {
	params := dbsqlc.ListDashboardVersionsParams{ID: id, RowLimit: toInt32(limit)}
	if before > 0 {
		params.Before = pgtype.Int4{Int32: toInt32(before), Valid: true}
	}
	rows, err := st.db.Queries().ListDashboardVersions(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list dashboard versions: %w", err)
	}
	items := make([]VersionMeta, 0, len(rows))
	for _, r := range rows {
		items = append(items, metaFromRow(r.Version, r.ParentVersion, r.RestoredFrom, r.Summary, authorFromRow(r.AuthorKind, r.AuthorID, r.AuthorName, r.AuthorEmail), r.CreatedAt))
	}
	return items, nil
}

func dashboardFromRow(row dbsqlc.Dashboard, updatedBy Author) Dashboard {
	tags := row.Tags
	if tags == nil {
		tags = []string{}
	}
	return Dashboard{
		ID:            row.ID.String(),
		Title:         row.Title,
		Tags:          tags,
		LatestVersion: int(row.LatestVersion),
		CreatedAt:     row.CreatedAt.Time.UTC(),
		UpdatedAt:     row.UpdatedAt.Time.UTC(),
		UpdatedBy:     updatedBy,
	}
}

func versionFromRow(row dbsqlc.DashboardVersion) (Version, error) {
	d, err := decodeSpec(row.Spec)
	if err != nil {
		return Version{}, err
	}
	meta := metaFromRow(row.Version, row.ParentVersion, row.RestoredFrom, row.Summary, authorFromRow(row.AuthorKind, row.AuthorID, row.AuthorName, row.AuthorEmail), row.CreatedAt)
	return Version{VersionMeta: meta, Spec: d}, nil
}

func metaFromRow(number int32, parent, restored pgtype.Int4, summary string, author Author, createdAt pgtype.Timestamptz) VersionMeta {
	return VersionMeta{
		Number:        int(number),
		ParentVersion: optionalInt(parent),
		RestoredFrom:  optionalInt(restored),
		Summary:       summary,
		Author:        author,
		CreatedAt:     createdAt.Time.UTC(),
	}
}

func authorFromRow(kind string, id pgtype.UUID, name string, email pgtype.Text) Author {
	return Author{Kind: kind, ID: id.String(), Name: name, Email: email.String}
}

func optionalInt(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

func tagsOf(d *spec.Dashboard) []string {
	if d.Tags == nil {
		return []string{}
	}
	return d.Tags
}

func decodeSpec(raw []byte) (*spec.Dashboard, error) {
	var d spec.Dashboard
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("decode stored spec: %w", err)
	}
	return &d, nil
}

func toInt32(n int) int32 {
	if n < math.MinInt32 || n > math.MaxInt32 {
		panic("dashboards: integer outside the int32 range")
	}
	return int32(n)
}
