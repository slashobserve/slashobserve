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

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	viaDirect = "direct"
	viaAgent  = "agent"

	uniqueViolation          = "23505"
	requestVersionConstraint = "dashboard_versions_agent_request_id_key"
	foreignKeyViolation      = "23503"
	dashboardFolderFK        = "dashboards_folder_id_fkey"
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
	RunID        string
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
	Folder        folders.Folder
	Abilities     folders.Abilities
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

	MergedProposalID string
}

type Version struct {
	VersionMeta
	Spec       *spec.Dashboard
	Unverified Unverified
}

type newVersion struct {
	dashboardID    pgtype.UUID
	parent         int
	restoredFrom   *int
	spec           *spec.Dashboard
	canonical      []byte
	summary        string
	author         Author
	requestID      pgtype.UUID
	origin         origin.Origin
	unverified     Unverified
	folder         folders.Folder
	mergedProposal pgtype.UUID
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
			CreatedRunID:        in.requestID,
			FolderID:            in.folder.ID,
		})
		if err != nil {
			return fmt.Errorf("create dashboard: %w", err)
		}
		in.dashboardID = row.ID
		meta, err := insertVersion(ctx, q, in, 1)
		if err != nil {
			return err
		}
		dash, ver = dashboardFromRow(row, in.author), Version{VersionMeta: meta, Spec: in.spec, Unverified: in.unverified}
		dash.Folder = in.folder
		if inTx == nil {
			return nil
		}
		return inTx(q, dash, ver)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == requestVersionConstraint {
		return Dashboard{}, Version{}, errRequestSaved
	}
	if isFolderGone(err) {
		return Dashboard{}, Version{}, ErrFolderNotFound
	}
	return dash, ver, err
}

func isFolderGone(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation && pgErr.ConstraintName == dashboardFolderFK
}

func (st store) move(ctx context.Context, id, from, to pgtype.UUID, by actor.Actor) error {
	return st.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		moved, err := q.MoveDashboard(ctx, dbsqlc.MoveDashboardParams{ID: id, FolderID: to})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrNotFound
		case isFolderGone(err):
			return ErrFolderNotFound
		case err != nil:
			return fmt.Errorf("move dashboard: %w", err)
		}
		if err := activity.Move(ctx, q, activity.OnDashboard(id), to); err != nil {
			return err
		}
		entry := activity.Entry{
			Type:     activity.TypeDashboardMoved,
			TargetID: id,
			Owner:    activity.OnDashboard(id),
			Actor:    by,
			Data:     movedActivityFields{Title: moved.Title, FromFolderID: from.String(), ToFolderID: to.String()},
		}
		_, err = activity.Append(ctx, q, entry, nil)
		return err
	})
}

type movedActivityFields struct {
	Title        string `json:"title"`
	FromFolderID string `json:"from_folder_id"`
	ToFolderID   string `json:"to_folder_id"`
}

func deletedEvent(dashboardID pgtype.UUID, by Author) events.Event {
	return events.Event{
		Type:        events.TypeDashboardDeleted,
		DashboardID: dashboardID.String(),
		Author:      events.NewAuthor(by.Kind, by.ID, by.Name),
	}
}

func (st store) delete(ctx context.Context, id pgtype.UUID, by actor.Actor) error {
	return st.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		title, err := q.LockDashboardForDelete(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock dashboard: %w", err)
		}
		ev := deletedEvent(id, Author{Kind: string(by.Kind), ID: by.ID, Name: by.Name})
		entry := activity.Entry{
			Type:     activity.TypeDashboardDeleted,
			TargetID: id,
			Owner:    activity.OnDashboard(id),
			Actor:    by,
			Data:     deletedActivityFields{Title: title},
		}
		if _, err := activity.Append(ctx, q, entry, &ev); err != nil {
			return err
		}
		n, err := q.DeleteDashboard(ctx, id)
		if err != nil {
			return fmt.Errorf("delete dashboard: %w", err)
		}
		if n == 0 {
			return ErrBeingCreated
		}
		return nil
	})
}

type deletedActivityFields struct {
	Title string `json:"title"`
}

func (st store) addVersion(ctx context.Context, in newVersion) (Dashboard, Version, error) {
	return st.addVersionWith(ctx, in, nil)
}

func (st store) addVersionWith(ctx context.Context, in newVersion, inTx func(q *dbsqlc.Queries, dash Dashboard, ver Version) error) (Dashboard, Version, error) {
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
		dash, ver = dashboardFromRow(row, in.author), Version{VersionMeta: meta, Spec: in.spec, Unverified: in.unverified}
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
		DashboardID:      in.dashboardID,
		Version:          httpapi.Int32(number),
		Spec:             in.canonical,
		Summary:          in.summary,
		AuthorKind:       in.author.Kind,
		AuthorID:         authorID,
		AuthorName:       in.author.Name,
		AuthorEmail:      pgtype.Text{String: in.author.Email, Valid: in.author.Email != ""},
		AgentRequestID:   in.requestID,
		Origin:           originKind,
		OriginClient:     originClient,
		MergedProposalID: in.mergedProposal,
	}
	if !in.unverified.Empty() {
		params.Unverified, err = json.Marshal(in.unverified)
		if err != nil {
			return VersionMeta{}, fmt.Errorf("marshal unverified: %w", err)
		}
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
	meta := metaFromRow(row.Version, row.ParentVersion, row.RestoredFrom, row.Summary, authorFromRow(row.AuthorKind, row.AuthorID, row.AuthorName, row.AuthorEmail), row.CreatedAt, row.Origin, row.OriginClient, row.AgentRequestID, row.MergedProposalID)
	ev := versionEvent(in.dashboardID, meta)
	entry := activity.Entry{
		Type:     versionActivityType(meta),
		TargetID: in.dashboardID,
		Owner:    activity.OnDashboard(in.dashboardID),
		Actor:    versionActor(meta),
		Data:     versionActivityData(in.spec.Title, meta),
	}
	if _, err := activity.Append(ctx, q, entry, &ev); err != nil {
		return VersionMeta{}, err
	}
	return meta, nil
}

func versionActivityType(meta VersionMeta) string {
	switch {
	case meta.RestoredFrom != nil:
		return activity.TypeDashboardRestored
	case meta.Number == 1:
		return activity.TypeDashboardCreated
	default:
		return activity.TypeDashboardVersionCreated
	}
}

type versionActivityFields struct {
	Version       int    `json:"version"`
	ParentVersion *int   `json:"parent_version,omitempty"`
	RestoredFrom  *int   `json:"restored_from,omitempty"`
	Title         string `json:"title"`
	Summary       string `json:"summary"`
}

func versionActivityData(title string, meta VersionMeta) versionActivityFields {
	return versionActivityFields{
		Version:       meta.Number,
		ParentVersion: meta.ParentVersion,
		RestoredFrom:  meta.RestoredFrom,
		Title:         title,
		Summary:       meta.Summary,
	}
}

// pg_notify refuses payloads of 8000 bytes or more. With the summary capped at 500 characters and the
// name at 256, even all-escaped characters (6 bytes each) leave this payload under 5000 bytes.
func versionEvent(dashboardID pgtype.UUID, meta VersionMeta) events.Event {
	return events.Event{
		Type:          events.TypeVersionCreated,
		DashboardID:   dashboardID.String(),
		Version:       meta.Number,
		ParentVersion: meta.ParentVersion,
		RestoredFrom:  meta.RestoredFrom,
		Summary:       meta.Summary,
		Author:        events.NewAuthor(meta.Author.Kind, meta.Author.ID, meta.Author.Name),
		CreatedAt:     httpapi.FormatTime(meta.CreatedAt),
	}
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
	dash.Folder = folderOf(row.Dashboard.FolderID, row.FolderTitle, row.FolderGeneral, row.FolderPersonal)
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
	dash.Folder = folderOf(row.Dashboard.FolderID, row.FolderTitle, row.FolderGeneral, row.FolderPersonal)
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

func (st store) unverified(ctx context.Context, id pgtype.UUID, number int) (Unverified, error) {
	raw, err := st.db.Queries().GetDashboardVersionUnverified(ctx, dbsqlc.GetDashboardVersionUnverifiedParams{ID: id, Version: httpapi.Int32(number)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Unverified{}, ErrNotFound
	}
	if err != nil {
		return Unverified{}, fmt.Errorf("get version unverified: %w", err)
	}
	return decodeUnverified(raw)
}

func (st store) list(ctx context.Context, after *httpapi.TimeCursor, limit, offset int, f dbsqlc.CountDashboardsParams) ([]ListItem, error) {
	params := dbsqlc.ListDashboardsParams{
		Folders:     f.Folders,
		Folder:      f.Folder,
		CreatorKind: f.CreatorKind,
		CreatorID:   f.CreatorID,
		OnlyAgent:   f.OnlyAgent,
		OnlyMcp:     f.OnlyMcp,
		McpClient:   f.McpClient,
		Datasource:  f.Datasource,
		Q:           f.Q,
		Tags:        f.Tags,
		RowLimit:    httpapi.Int32(limit),
		RowOffset:   httpapi.Int32(offset),
	}
	if after != nil {
		params.CursorUpdatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	rows, err := st.db.Queries().ListDashboards(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list dashboards: %w", err)
	}
	return listItemsOf(rows), nil
}

func (st store) listByFolder(ctx context.Context, after *folderCursor, limit, offset int, f dbsqlc.CountDashboardsParams) ([]ListItem, error) {
	params := dbsqlc.ListDashboardsByFolderParams{
		Folders:     f.Folders,
		Folder:      f.Folder,
		CreatorKind: f.CreatorKind,
		CreatorID:   f.CreatorID,
		OnlyAgent:   f.OnlyAgent,
		OnlyMcp:     f.OnlyMcp,
		McpClient:   f.McpClient,
		Datasource:  f.Datasource,
		Q:           f.Q,
		Tags:        f.Tags,
		RowLimit:    httpapi.Int32(limit),
		RowOffset:   httpapi.Int32(offset),
	}
	if after != nil {
		params.CFolder = after.FolderID
		params.CPersonal = pgtype.Bool{Bool: after.Personal, Valid: true}
		params.CGeneral = pgtype.Bool{Bool: after.General, Valid: true}
		params.CTitle = pgtype.Text{String: after.Title, Valid: true}
		params.CUpdatedAt = pgtype.Timestamptz{Time: after.UpdatedAt, Valid: true}
		params.CID = after.ID
	}
	rows, err := st.db.Queries().ListDashboardsByFolder(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list dashboards by folder: %w", err)
	}
	same := make([]dbsqlc.ListDashboardsRow, len(rows))
	for i, r := range rows {
		same[i] = dbsqlc.ListDashboardsRow(r)
	}
	return listItemsOf(same), nil
}

func listItemsOf(rows []dbsqlc.ListDashboardsRow) []ListItem {
	items := make([]ListItem, 0, len(rows))
	for _, r := range rows {
		uids := r.DatasourceUids
		if uids == nil {
			uids = []string{}
		}
		dash := dashboardFromRow(r.Dashboard, authorFromRow(r.UpdatedByKind, r.UpdatedByID, r.UpdatedByName, r.UpdatedByEmail))
		dash.Folder = folderOf(r.Dashboard.FolderID, r.FolderTitle, r.FolderGeneral, r.FolderPersonal)
		items = append(items, ListItem{
			Dashboard:      dash,
			Description:    r.Description,
			DatasourceUIDs: uids,
			OpenComments:   int(r.OpenCommentCount),
			sortTitle:      r.FolderSortTitle,
		})
	}
	return items
}

func folderOf(id pgtype.UUID, title string, general, personal bool) folders.Folder {
	return folders.Folder{ID: id, Title: title, General: general, Personal: personal}
}

func (st store) count(ctx context.Context, f dbsqlc.CountDashboardsParams) (int, error) {
	n, err := st.db.Queries().CountDashboards(ctx, f)
	if err != nil {
		return 0, fmt.Errorf("count dashboards: %w", err)
	}
	return int(n), nil
}

func filterOf(p ListParams, visible []pgtype.UUID) (dbsqlc.CountDashboardsParams, error) {
	f := dbsqlc.CountDashboardsParams{
		Folders:    visible,
		Folder:     p.Folder,
		OnlyAgent:  p.MadeBy.AgentOnly,
		OnlyMcp:    p.MadeBy.MCPOnly,
		McpClient:  optionalText(p.MadeBy.MCPClient),
		Datasource: optionalText(p.Datasource),
		Q:          optionalText(p.Query),
		Tags:       p.Tags,
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
		items = append(items, metaFromRow(r.Version, r.ParentVersion, r.RestoredFrom, r.Summary, authorFromRow(r.AuthorKind, r.AuthorID, r.AuthorName, r.AuthorEmail), r.CreatedAt, r.Origin, r.OriginClient, r.AgentRequestID, r.MergedProposalID))
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
		Folder:        folders.Folder{ID: row.FolderID},
		LatestVersion: int(row.LatestVersion),
		CreatedAt:     row.CreatedAt.Time.UTC(),
		UpdatedAt:     row.UpdatedAt.Time.UTC(),
		UpdatedBy:     updatedBy,
		Creator: Creator{
			Author:       authorFromRow(row.CreatedByKind, row.CreatedByID, row.CreatedByName, row.CreatedByEmail),
			Origin:       row.CreatedOrigin.String,
			OriginClient: row.CreatedOriginClient.String,
			Via:          row.CreatedVia,
			RunID:        row.CreatedRunID.String(),
		},
	}
}

func versionFromRow(row dbsqlc.DashboardVersion) (Version, error) {
	d, err := decodeSpec(row.Spec)
	if err != nil {
		return Version{}, err
	}
	meta := metaFromRow(row.Version, row.ParentVersion, row.RestoredFrom, row.Summary, authorFromRow(row.AuthorKind, row.AuthorID, row.AuthorName, row.AuthorEmail), row.CreatedAt, row.Origin, row.OriginClient, row.AgentRequestID, row.MergedProposalID)
	unverified, err := decodeUnverified(row.Unverified)
	if err != nil {
		return Version{}, err
	}
	return Version{VersionMeta: meta, Spec: d, Unverified: unverified}, nil
}

func decodeUnverified(raw []byte) (Unverified, error) {
	var u Unverified
	if raw == nil {
		return u, nil
	}
	if err := json.Unmarshal(raw, &u); err != nil {
		return Unverified{}, fmt.Errorf("decode stored unverified: %w", err)
	}
	return u, nil
}

func metaFromRow(number int32, parent, restored pgtype.Int4, summary string, author Author, createdAt pgtype.Timestamptz, originKind, originClient pgtype.Text, agentRequestID, mergedProposalID pgtype.UUID) VersionMeta {
	return VersionMeta{
		Number:         int(number),
		ParentVersion:  optionalInt(parent),
		RestoredFrom:   optionalInt(restored),
		Summary:        summary,
		Author:         author,
		CreatedAt:      createdAt.Time.UTC(),
		Origin:         originKind.String,
		OriginClient:   originClient.String,
		AgentRequestID: agentRequestID.String(),

		MergedProposalID: mergedProposalID.String(),
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
