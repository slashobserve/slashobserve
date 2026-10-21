package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxEventAuthorNameRunes = 256

const (
	viaDirect = "direct"
	viaAgent  = "agent"

	uniqueViolation          = "23505"
	requestVersionConstraint = "dashboard_versions_agent_request_id_key"
)

var (
	errStaleParent  = errors.New("parent version is no longer the latest")
	errRequestSaved = errors.New("a version already exists for the request id")
)

type Author struct {
	Kind  string
	ID    string
	Name  string
	Email string
}

type Creator struct {
	Author       Author
	Origin       string
	OriginClient string
	Via          string
}

type Dashboard struct {
	ID            string
	Title         string
	Tags          []string
	LatestVersion int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	UpdatedBy     Author
	Creator       Creator
}

type VersionMeta struct {
	Number         int
	ParentVersion  *int
	RestoredFrom   *int
	Summary        string
	Author         Author
	CreatedAt      time.Time
	Origin         string
	OriginClient   string
	AgentRequestID string
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
	requestID    pgtype.UUID
	origin       origin.Origin
}

type store struct {
	db *db.DB
}

func (st store) create(ctx context.Context, in newVersion) (Dashboard, Version, error) {
	return st.createWith(ctx, in, nil)
}

func (st store) createWith(ctx context.Context, in newVersion, inTx func(q *dbsqlc.Queries, dash Dashboard, ver Version) error) (Dashboard, Version, error) {
	var dash Dashboard
	var ver Version
	err := st.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		authorID, err := db.ParseUUID(in.author.ID)
		if err != nil {
			return fmt.Errorf("author id: %w", err)
		}
		originKind, originClient := in.origin.Columns()
		via := viaDirect
		if in.requestID.Valid {
			via = viaAgent
		}
		row, err := q.CreateDashboard(ctx, dbsqlc.CreateDashboardParams{
			Title:               in.spec.Title,
			Tags:                tagsOf(in.spec),
			CreatedByKind:       in.author.Kind,
			CreatedByID:         authorID,
			CreatedByName:       in.author.Name,
			CreatedByEmail:      pgtype.Text{String: in.author.Email, Valid: in.author.Email != ""},
			CreatedOrigin:       originKind,
			CreatedOriginClient: originClient,
			CreatedVia:          via,
		})
		if err != nil {
			return fmt.Errorf("create dashboard: %w", err)
		}
		in.dashboardID = row.ID
		meta, err := insertVersion(ctx, q, in, 1)
		if err != nil {
			return err
		}
		dash, ver = dashboardFromRow(row, in.author), Version{VersionMeta: meta, Spec: in.spec}
		if inTx == nil {
			return nil
		}
		return inTx(q, dash, ver)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == requestVersionConstraint {
		return Dashboard{}, Version{}, errRequestSaved
	}
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
			ExpectedVersion: httpapi.Int32(in.parent),
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
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == requestVersionConstraint {
		return Dashboard{}, Version{}, errRequestSaved
	}
	return dash, ver, err
}

func (st store) savedByRequest(ctx context.Context, requestID, dashboardID pgtype.UUID) (Dashboard, Version, bool, error) {
	row, err := st.db.Queries().GetAgentRequestSavedVersion(ctx, dbsqlc.GetAgentRequestSavedVersionParams{AgentRequestID: requestID, DashboardID: dashboardID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Dashboard{}, Version{}, false, nil
	}
	if err != nil {
		return Dashboard{}, Version{}, false, fmt.Errorf("get version of request: %w", err)
	}
	dash, ver, err := st.version(ctx, row.DashboardID, int(row.Version))
	return dash, ver, err == nil, err
}

func insertVersion(ctx context.Context, q *dbsqlc.Queries, in newVersion, number int) (VersionMeta, error) {
	authorID, err := db.ParseUUID(in.author.ID)
	if err != nil {
		return VersionMeta{}, fmt.Errorf("author id: %w", err)
	}
	originKind, originClient := in.origin.Columns()
	params := dbsqlc.InsertDashboardVersionParams{
		DashboardID:    in.dashboardID,
		Version:        httpapi.Int32(number),
		Spec:           in.canonical,
		Summary:        in.summary,
		AuthorKind:     in.author.Kind,
		AuthorID:       authorID,
		AuthorName:     in.author.Name,
		AuthorEmail:    pgtype.Text{String: in.author.Email, Valid: in.author.Email != ""},
		AgentRequestID: in.requestID,
		Origin:         originKind,
		OriginClient:   originClient,
	}
	if number > 1 {
		params.ParentVersion = pgtype.Int4{Int32: httpapi.Int32(in.parent), Valid: true}
	}
	if in.restoredFrom != nil {
		params.RestoredFrom = pgtype.Int4{Int32: httpapi.Int32(*in.restoredFrom), Valid: true}
	}
	row, err := q.InsertDashboardVersion(ctx, params)
	if err != nil {
		return VersionMeta{}, fmt.Errorf("insert dashboard version: %w", err)
	}
	meta := metaFromRow(row.Version, row.ParentVersion, row.RestoredFrom, row.Summary, authorFromRow(row.AuthorKind, row.AuthorID, row.AuthorName, row.AuthorEmail), row.CreatedAt, row.Origin, row.OriginClient, row.AgentRequestID)
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
		CreatedAt:     httpapi.FormatTime(meta.CreatedAt),
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
	row, err := st.db.Queries().GetDashboardVersion(ctx, dbsqlc.GetDashboardVersionParams{ID: id, Version: httpapi.Int32(number)})
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
	raw, err := st.db.Queries().GetDashboardVersionSpec(ctx, dbsqlc.GetDashboardVersionSpecParams{ID: id, Version: httpapi.Int32(number)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get version spec: %w", err)
	}
	return raw, nil
}

func (st store) list(ctx context.Context, after *httpapi.TimeCursor, limit int, f dbsqlc.CountDashboardsParams) ([]ListItem, error) {
	params := dbsqlc.ListDashboardsParams{
		CreatorKind: f.CreatorKind,
		CreatorID:   f.CreatorID,
		OnlyAgent:   f.OnlyAgent,
		OnlyMcp:     f.OnlyMcp,
		McpClient:   f.McpClient,
		Datasource:  f.Datasource,
		Q:           f.Q,
		RowLimit:    httpapi.Int32(limit),
	}
	if after != nil {
		params.CursorUpdatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	rows, err := st.db.Queries().ListDashboards(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list dashboards: %w", err)
	}
	items := make([]ListItem, 0, len(rows))
	for _, r := range rows {
		uids := r.DatasourceUids
		if uids == nil {
			uids = []string{}
		}
		items = append(items, ListItem{
			Dashboard:      dashboardFromRow(r.Dashboard, authorFromRow(r.UpdatedByKind, r.UpdatedByID, r.UpdatedByName, r.UpdatedByEmail)),
			Description:    r.Description,
			DatasourceUIDs: uids,
			OpenComments:   int(r.OpenCommentCount),
		})
	}
	return items, nil
}

func (st store) count(ctx context.Context, f dbsqlc.CountDashboardsParams) (int, error) {
	n, err := st.db.Queries().CountDashboards(ctx, f)
	if err != nil {
		return 0, fmt.Errorf("count dashboards: %w", err)
	}
	return int(n), nil
}

func filterOf(p ListParams) (dbsqlc.CountDashboardsParams, error) {
	f := dbsqlc.CountDashboardsParams{
		OnlyAgent:  p.MadeBy.AgentOnly,
		OnlyMcp:    p.MadeBy.MCPOnly,
		McpClient:  optionalText(p.MadeBy.MCPClient),
		Datasource: optionalText(p.Datasource),
		Q:          optionalText(p.Query),
	}
	if p.MadeBy.CreatorKind != "" {
		id, err := db.ParseUUID(p.MadeBy.CreatorID)
		if err != nil {
			return f, fmt.Errorf("creator id: %w", err)
		}
		f.CreatorKind = pgtype.Text{String: p.MadeBy.CreatorKind, Valid: true}
		f.CreatorID = id
	}
	return f, nil
}

func optionalText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func (st store) listVersions(ctx context.Context, id pgtype.UUID, before, limit int) ([]VersionMeta, error) {
	params := dbsqlc.ListDashboardVersionsParams{ID: id, RowLimit: httpapi.Int32(limit)}
	if before > 0 {
		params.Before = pgtype.Int4{Int32: httpapi.Int32(before), Valid: true}
	}
	rows, err := st.db.Queries().ListDashboardVersions(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list dashboard versions: %w", err)
	}
	items := make([]VersionMeta, 0, len(rows))
	for _, r := range rows {
		items = append(items, metaFromRow(r.Version, r.ParentVersion, r.RestoredFrom, r.Summary, authorFromRow(r.AuthorKind, r.AuthorID, r.AuthorName, r.AuthorEmail), r.CreatedAt, r.Origin, r.OriginClient, r.AgentRequestID))
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
		Creator: Creator{
			Author:       authorFromRow(row.CreatedByKind, row.CreatedByID, row.CreatedByName, row.CreatedByEmail),
			Origin:       row.CreatedOrigin.String,
			OriginClient: row.CreatedOriginClient.String,
			Via:          row.CreatedVia,
		},
	}
}

func versionFromRow(row dbsqlc.DashboardVersion) (Version, error) {
	d, err := decodeSpec(row.Spec)
	if err != nil {
		return Version{}, err
	}
	meta := metaFromRow(row.Version, row.ParentVersion, row.RestoredFrom, row.Summary, authorFromRow(row.AuthorKind, row.AuthorID, row.AuthorName, row.AuthorEmail), row.CreatedAt, row.Origin, row.OriginClient, row.AgentRequestID)
	return Version{VersionMeta: meta, Spec: d}, nil
}

func metaFromRow(number int32, parent, restored pgtype.Int4, summary string, author Author, createdAt pgtype.Timestamptz, originKind, originClient pgtype.Text, agentRequestID pgtype.UUID) VersionMeta {
	meta := VersionMeta{
		Number:        int(number),
		ParentVersion: optionalInt(parent),
		RestoredFrom:  optionalInt(restored),
		Summary:       summary,
		Author:        author,
		CreatedAt:     createdAt.Time.UTC(),
		Origin:        originKind.String,
		OriginClient:  originClient.String,
	}
	if agentRequestID.Valid {
		meta.AgentRequestID = agentRequestID.String()
	}
	return meta
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
