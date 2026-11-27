package dashboards

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/placement"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxFolderRetries     = 5
	maxSummaryLength     = 500
	maxInt32             = math.MaxInt32
	defaultCreateSummary = "Create dashboard"
	defaultUpdateSummary = "Update dashboard"
)

var (
	ErrNotFound       = errors.New("dashboard not found")
	ErrFolderNotFound = errors.New("folder not found")
	ErrInvalidCursor  = errors.New("invalid cursor")
	ErrInvalidRange   = errors.New("from must be less than to")
	ErrBeingCreated   = errors.New("the agent is still creating this dashboard")
	errFolderMoved    = errors.New("the dashboard changed folders after its access was checked")
)

type ConflictError struct {
	Latest    int
	Conflicts []spec.Conflict
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("dashboard has a newer version %d", e.Latest)
}

type InvalidSpecError struct {
	Issues []spec.Issue
}

func (e *InvalidSpecError) Error() string {
	return fmt.Sprintf("spec is invalid: %d issue(s)", len(e.Issues))
}

type InvalidInputError struct {
	Message string
}

func (e *InvalidInputError) Error() string {
	return e.Message
}

type Service struct {
	store  store
	runner *queryproxy.Runner
	access *folders.Access
	logger *slog.Logger
	guard  *placement.Guard

	proposer Proposer
}

func New(database *db.DB, runner *queryproxy.Runner, access *folders.Access, logger *slog.Logger) *Service {
	guard := placement.New(runner.Registry())
	return &Service{store: store{db: database, guard: guard}, runner: runner, access: access, logger: logger, guard: guard}
}

func (s *Service) Placement() *placement.Guard {
	return s.guard
}

func (s *Service) Create(ctx context.Context, by auth.Principal, folderID pgtype.UUID, rawSpec []byte, summary string) (Dashboard, Version, error) {
	return s.create(ctx, by, folderID, rawSpec, summary, pgtype.UUID{})
}

func (s *Service) CreateSaved(ctx context.Context, by auth.Principal, folderID pgtype.UUID, rawSpec []byte, summary string, inTx func(ctx context.Context, q *dbsqlc.Queries, dash Dashboard, ver Version) error) (Dashboard, Version, error) {
	return s.createVersion(ctx, by, folderID, rawSpec, summary, newVersion{inTx: func(q *dbsqlc.Queries, dash Dashboard, ver Version) error {
		return inTx(ctx, q, dash, ver)
	}})
}

func (s *Service) CreateFromAnswer(ctx context.Context, by auth.Principal, rawSpec []byte, summary string, run pgtype.UUID) (Dashboard, Version, error) {
	return s.createVersion(ctx, by, pgtype.UUID{}, rawSpec, summary, newVersion{answerOf: run})
}

func (s *Service) CreateCopy(ctx context.Context, by auth.Principal, rawSpec []byte, summary string, from placement.Copied) (Dashboard, Version, error) {
	return s.createVersion(ctx, by, pgtype.UUID{}, rawSpec, summary, newVersion{copyOf: from})
}

func (s *Service) AddVersion(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, rawSpec []byte, summary string) (Dashboard, Version, bool, error) {
	res, err := s.Save(ctx, by, SaveInput{DashboardID: id, Parent: parent, Spec: rawSpec, Summary: summary})
	return res.Dashboard, res.Version, res.Replayed, err
}

func (s *Service) CreateRequestDashboard(ctx context.Context, by auth.Principal, folderID pgtype.UUID, rawSpec []byte, summary string, requestID pgtype.UUID) (Dashboard, Version, bool, error) {
	if !requestID.Valid {
		dash, ver, err := s.Create(ctx, by, folderID, rawSpec, summary)
		return dash, ver, false, err
	}
	if dash, ver, found, err := s.CreatedByRequest(ctx, requestID); err != nil || found {
		return dash, ver, found, err
	}
	dash, ver, err := s.create(ctx, by, folderID, rawSpec, summary, requestID)
	if !errors.Is(err, errRequestSaved) {
		return dash, ver, false, err
	}
	return s.replayedCreate(ctx, requestID)
}

type ImportHook func(ctx context.Context, q *dbsqlc.Queries, dash Dashboard, ver Version, issues []spec.Issue) error

func (s *Service) CreateImported(ctx context.Context, by auth.Principal, folderID pgtype.UUID, rawSpec []byte, summary string, requestID pgtype.UUID, hook ImportHook) (Dashboard, Version, []spec.Issue, bool, error) {
	if requestID.Valid {
		if dash, ver, found, err := s.CreatedByRequest(ctx, requestID); err != nil || found {
			return dash, ver, nil, found, err
		}
	}
	target, err := s.creationTarget(ctx, by, folderID)
	if err != nil {
		return Dashboard{}, Version{}, nil, false, err
	}
	summary, parsed, canonical, err := validateSave(rawSpec, summary, defaultCreateSummary, nil)
	if err != nil {
		return Dashboard{}, Version{}, nil, false, err
	}
	issues, err := s.runner.CheckSpec(ctx, parsed, time.Now(), queryproxy.UserFor(by))
	if err != nil {
		return Dashboard{}, Version{}, nil, false, err
	}
	failing, _ := UnverifiedOf(parsed, issues)
	in := newVersion{spec: parsed, canonical: canonical, summary: summary, author: authorOf(by), requestID: requestID, origin: origin.From(ctx), unverified: failing.because(ReasonImported), folder: target.folder}
	var inTx func(q *dbsqlc.Queries, dash Dashboard, ver Version) error
	if hook != nil {
		inTx = func(q *dbsqlc.Queries, dash Dashboard, ver Version) error {
			return hook(ctx, q, dash, ver, issues)
		}
	}
	dash, ver, err := s.store.createWith(ctx, in, inTx)
	if errors.Is(err, errRequestSaved) && requestID.Valid {
		dash, ver, replayed, err := s.replayedCreate(ctx, requestID)
		return dash, ver, nil, replayed, err
	}
	if err != nil {
		return Dashboard{}, Version{}, nil, false, err
	}
	dash.Abilities = target.abilities
	return dash, ver, issues, false, nil
}

func (s *Service) replayedCreate(ctx context.Context, requestID pgtype.UUID) (Dashboard, Version, bool, error) {
	dash, ver, found, err := s.CreatedByRequest(ctx, requestID)
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	if !found {
		return Dashboard{}, Version{}, false, fmt.Errorf("create dashboard: %w", errRequestSaved)
	}
	return dash, ver, true, nil
}

func (s *Service) CreatedByRequest(ctx context.Context, requestID pgtype.UUID) (Dashboard, Version, bool, error) {
	dash, ver, found, err := s.store.savedByRequest(ctx, requestID, pgtype.UUID{})
	if err != nil || !found {
		return Dashboard{}, Version{}, false, err
	}
	if ver.Number != 1 {
		return Dashboard{}, Version{}, false, fmt.Errorf("request %s already saved version %d, not a new dashboard", requestID.String(), ver.Number)
	}
	return dash, ver, true, nil
}

func (s *Service) create(ctx context.Context, by auth.Principal, folderID pgtype.UUID, rawSpec []byte, summary string, requestID pgtype.UUID) (Dashboard, Version, error) {
	return s.createVersion(ctx, by, folderID, rawSpec, summary, newVersion{requestID: requestID})
}

func (s *Service) createVersion(ctx context.Context, by auth.Principal, folderID pgtype.UUID, rawSpec []byte, summary string, in newVersion) (Dashboard, Version, error) {
	target, err := s.creationTarget(ctx, by, folderID)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	summary, parsed, canonical, err := validateSave(rawSpec, summary, defaultCreateSummary, nil)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	if err := s.checkQueries(ctx, by, parsed); err != nil {
		return Dashboard{}, Version{}, err
	}
	in.spec, in.canonical, in.summary, in.author, in.origin, in.folder = parsed, canonical, summary, authorOf(by), origin.From(ctx), target.folder
	dash, ver, err := s.store.create(ctx, in)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	dash.Abilities = target.abilities
	return dash, ver, nil
}

func (s *Service) CreateStarter(ctx context.Context, by auth.Principal, folderID pgtype.UUID, rawSpec []byte, summary string, inTx func(q *dbsqlc.Queries, dash Dashboard) error) (Dashboard, error) {
	target, err := s.creationTarget(ctx, by, folderID)
	if err != nil {
		return Dashboard{}, err
	}
	summary, parsed, canonical, err := validateSave(rawSpec, summary, defaultCreateSummary, nil)
	if err != nil {
		return Dashboard{}, err
	}
	unverified, err := s.verify(withAcceptedEmpty(ctx), by, nil, Unverified{}, parsed, false)
	if err != nil {
		return Dashboard{}, err
	}
	in := newVersion{spec: parsed, canonical: canonical, summary: summary, author: authorOf(by), origin: origin.From(ctx), unverified: unverified, folder: target.folder}
	dash, _, err := s.store.createWith(ctx, in, func(q *dbsqlc.Queries, dash Dashboard, _ Version) error { return inTx(q, dash) })
	if err != nil {
		return Dashboard{}, err
	}
	dash.Abilities = target.abilities
	return dash, nil
}

func (s *Service) Restore(ctx context.Context, by auth.Principal, id pgtype.UUID, from, parent int, summary string) (Dashboard, Version, error) {
	access, err := s.need(ctx, by, id, folders.NeedContent)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	dash, ver, err := s.restore(ctx, by, id, from, parent, summary)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	return withAccess(dash, access), ver, nil
}

func (s *Service) restore(ctx context.Context, by auth.Principal, id pgtype.UUID, from, parent int, summary string) (Dashboard, Version, error) {
	summary, issues := checkSummary(summary, "Restore version "+strconv.Itoa(from))
	if len(issues) > 0 {
		return Dashboard{}, Version{}, &InvalidSpecError{Issues: issues}
	}
	stored, err := s.store.specBytes(ctx, id, from)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	latest, err := s.store.latestVersion(ctx, id)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	if latest != parent {
		return Dashboard{}, Version{}, &ConflictError{Latest: latest}
	}
	parsed, err := spec.Parse(stored)
	if err != nil {
		return Dashboard{}, Version{}, asInvalidSpec(err)
	}
	issues, err = s.runner.CheckDatasources(ctx, parsed)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	if len(issues) > 0 {
		return Dashboard{}, Version{}, &InvalidSpecError{Issues: issues}
	}
	unverified, err := s.store.unverified(ctx, id, from)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	canonical, err := json.Marshal(parsed)
	if err != nil {
		return Dashboard{}, Version{}, fmt.Errorf("marshal spec: %w", err)
	}
	dash, ver, err := s.store.addVersion(ctx, newVersion{dashboardID: id, parent: parent, restoredFrom: &from, spec: parsed, canonical: canonical, summary: summary, author: authorOf(by), origin: origin.From(ctx), unverified: unverified.because(ReasonRestored)})
	if errors.Is(err, errStaleParent) {
		return Dashboard{}, Version{}, s.conflict(ctx, id)
	}
	return dash, ver, err
}

func (s *Service) Get(ctx context.Context, id pgtype.UUID) (Dashboard, Version, error) {
	return s.store.latest(ctx, id)
}

func (s *Service) GetVersion(ctx context.Context, id pgtype.UUID, version int) (Dashboard, Version, error) {
	return s.store.version(ctx, id, version)
}

func (s *Service) VersionSpec(ctx context.Context, id pgtype.UUID, version int) (*spec.Dashboard, error) {
	raw, err := s.store.specBytes(ctx, id, version)
	if err != nil {
		return nil, err
	}
	return decodeSpec(raw)
}

func (s *Service) HasContent(ctx context.Context, id pgtype.UUID, version int, raw []byte) (bool, error) {
	parsed, err := spec.Parse(raw)
	if err != nil {
		return false, nil
	}
	stored, err := s.VersionSpec(ctx, id, version)
	if err != nil {
		return false, err
	}
	next, err := json.Marshal(parsed)
	if err != nil {
		return false, err
	}
	prev, err := json.Marshal(stored)
	if err != nil {
		return false, err
	}
	return bytes.Equal(next, prev), nil
}

func (s *Service) HasPanel(ctx context.Context, id pgtype.UUID, version int, panelID string) (bool, error) {
	d, err := s.VersionSpec(ctx, id, version)
	if errors.Is(err, ErrNotFound) {
		return false, revisions.ErrDashboardNotFound
	}
	if err != nil {
		return false, err
	}
	for _, entry := range d.AllPanels() {
		if entry.Panel.ID == panelID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) LatestVersion(ctx context.Context, id pgtype.UUID) (int, error) {
	return s.store.latestVersion(ctx, id)
}

type MadeBy struct {
	CreatorKind string
	CreatorID   string
	AgentOnly   bool
	MCPOnly     bool
	MCPClient   string
}

type ListParams struct {
	Limit        int
	Cursor       string
	Offset       int
	MadeBy       MadeBy
	Datasource   string
	Query        string
	Tags         []string
	Problem      string
	ExcludeAudit bool
	SkipTotal    bool
	Folder       pgtype.UUID
	ByFolder     bool
	ByViews      bool
	ByName       bool
}

type ListItem struct {
	Dashboard
	Description    string
	DatasourceUIDs []string
	OpenComments   int
	ViewCount      int
	sortTitle      string
	titleSort      string
}

type ListPage struct {
	Items      []ListItem
	NextCursor string
	Total      int
}

func (s *Service) markProvisioned(ctx context.Context, items []ListItem, levels folders.Levels) error {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	provisioned, err := s.store.provisionedAmong(ctx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		access := folders.DashboardAccess{Abilities: levels.Of(items[i].Folder.ID), Provisioned: provisioned[items[i].ID]}
		items[i].Abilities = access.Shown()
		items[i].Provisioned = access.Provisioned
	}
	return nil
}

const madeByMessage = "made_by must be me, agent, mcp, mcp:<client>, user:<id> or service_account:<id>"

func ParseMadeBy(raw string, caller auth.Principal) (MadeBy, error) {
	invalid := &InvalidInputError{Message: madeByMessage}
	switch raw {
	case "me":
		return MadeBy{CreatorKind: string(caller.Kind), CreatorID: caller.ID}, nil
	case "agent":
		return MadeBy{AgentOnly: true}, nil
	case "mcp":
		return MadeBy{MCPOnly: true}, nil
	}
	kind, value, found := strings.Cut(raw, ":")
	if !found {
		return MadeBy{}, invalid
	}
	switch kind {
	case "mcp":
		if n := utf8.RuneCountInString(value); n < 1 || n > origin.MaxClientRunes || origin.CleanClient(value) != value {
			return MadeBy{}, invalid
		}
		return MadeBy{MCPOnly: true, MCPClient: value}, nil
	case string(auth.KindUser), string(auth.KindServiceAccount):
		id, err := db.ParseUUID(value)
		if err != nil {
			return MadeBy{}, invalid
		}
		return MadeBy{CreatorKind: kind, CreatorID: id.String()}, nil
	}
	return MadeBy{}, invalid
}

func (s *Service) List(ctx context.Context, by auth.Principal, p ListParams) (ListPage, error) {
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return ListPage{}, err
	}
	visible, err := s.visibleFolders(ctx, levels, p.ExcludeAudit)
	if err != nil {
		return ListPage{}, err
	}
	filter, err := filterOf(p, visible)
	if err != nil {
		return ListPage{}, err
	}
	var items []ListItem
	switch {
	case p.ByFolder:
		items, err = s.listByFolder(ctx, p, filter)
	case p.ByViews:
		items, err = s.listByViews(ctx, p, filter)
	case p.ByName:
		items, err = s.listByName(ctx, p, filter)
	default:
		items, err = s.listByRecency(ctx, p, filter)
	}
	if err != nil {
		return ListPage{}, err
	}
	if err := s.markProvisioned(ctx, items, levels); err != nil {
		return ListPage{}, err
	}
	page := ListPage{Items: items}
	if !p.SkipTotal {
		if page.Total, err = s.store.count(ctx, filter); err != nil {
			return ListPage{}, err
		}
	}
	if len(items) > p.Limit {
		page.Items = items[:p.Limit]
		page.NextCursor = nextCursor(page.Items[len(page.Items)-1], p)
	}
	return page, nil
}

func (s *Service) visibleFolders(ctx context.Context, levels folders.Levels, excludeAudit bool) ([]pgtype.UUID, error) {
	visible := levels.Visible()
	if !excludeAudit {
		return visible, nil
	}
	audit, err := s.access.AuditFolder(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(visible, func(id pgtype.UUID) bool { return id == audit }), nil
}

func (s *Service) listByRecency(ctx context.Context, p ListParams, filter dbsqlc.CountDashboardsParams) ([]ListItem, error) {
	after, ok := httpapi.DecodeTimeCursor(p.Cursor)
	if !ok {
		return nil, ErrInvalidCursor
	}
	return s.store.list(ctx, after, p.Limit+1, p.Offset, filter)
}

func (s *Service) listByFolder(ctx context.Context, p ListParams, filter dbsqlc.CountDashboardsParams) ([]ListItem, error) {
	var after *folderCursor
	if p.Cursor != "" {
		decoded, err := decodeFolderCursor(p.Cursor)
		if err != nil {
			return nil, err
		}
		after = &decoded
	}
	return s.store.listByFolder(ctx, after, p.Limit+1, p.Offset, filter)
}

func (s *Service) listByViews(ctx context.Context, p ListParams, filter dbsqlc.CountDashboardsParams) ([]ListItem, error) {
	var after *viewsCursor
	if p.Cursor != "" {
		decoded, err := decodeViewsCursor(p.Cursor)
		if err != nil {
			return nil, err
		}
		after = &decoded
	}
	return s.store.listByViews(ctx, after, p.Limit+1, p.Offset, filter)
}

func (s *Service) listByName(ctx context.Context, p ListParams, filter dbsqlc.CountDashboardsParams) ([]ListItem, error) {
	var after *nameCursor
	if p.Cursor != "" {
		decoded, err := decodeNameCursor(p.Cursor)
		if err != nil {
			return nil, err
		}
		after = &decoded
	}
	return s.store.listByName(ctx, after, p.Limit+1, p.Offset, filter)
}

func nextCursor(last ListItem, p ListParams) string {
	switch {
	case p.ByFolder:
		return encodeFolderCursor(last.Folder.Personal, last.Folder.General, last.sortTitle, last.Folder.ID, last.UpdatedAt, last.ID)
	case p.ByViews:
		return encodeViewsCursor(last.ViewCount, last.UpdatedAt, last.ID)
	case p.ByName:
		return encodeNameCursor(last.titleSort, last.ID)
	}
	return httpapi.EncodeTimeCursor(last.UpdatedAt, last.ID)
}

func (s *Service) DatasourceUsage(ctx context.Context, by auth.Principal) (map[string]int, error) {
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.db.Queries().CountDashboardsByDatasource(ctx, levels.Visible())
	if err != nil {
		return nil, fmt.Errorf("count dashboards by datasource: %w", err)
	}
	usage := make(map[string]int, len(rows))
	for _, row := range rows {
		usage[row.Uid] = int(row.Dashboards)
	}
	return usage, nil
}

type TagCount struct {
	Tag        string
	Dashboards int
}

func (s *Service) Tags(ctx context.Context, by auth.Principal, excludeAudit bool) ([]TagCount, error) {
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return nil, err
	}
	visible, err := s.visibleFolders(ctx, levels, excludeAudit)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.db.Queries().ListDashboardTags(ctx, visible)
	if err != nil {
		return nil, fmt.Errorf("list dashboard tags: %w", err)
	}
	tags := make([]TagCount, 0, len(rows))
	for _, row := range rows {
		tags = append(tags, TagCount{Tag: row.Tag, Dashboards: int(row.Dashboards)})
	}
	return tags, nil
}

func (s *Service) Move(ctx context.Context, by auth.Principal, id, folderID pgtype.UUID) (Dashboard, Version, error) {
	for range maxFolderRetries {
		dash, ver, err := s.moveOnce(ctx, by, id, folderID)
		if !errors.Is(err, errFolderMoved) {
			return dash, ver, err
		}
	}
	return Dashboard{}, Version{}, fmt.Errorf("dashboard %s changed folders %d times in a row: %w", id, maxFolderRetries, errFolderMoved)
}

func (s *Service) moveOnce(ctx context.Context, by auth.Principal, id, folderID pgtype.UUID) (Dashboard, Version, error) {
	source, err := s.need(ctx, by, id, folders.NeedContent)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	target, abilities, err := s.access.Folder(ctx, by, folderID)
	if errors.Is(err, folders.ErrNotFound) {
		return Dashboard{}, Version{}, ErrFolderNotFound
	}
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	if !abilities.Edit {
		return Dashboard{}, Version{}, folders.ErrForbidden
	}
	if !folders.MayMoveInto(target) {
		return Dashboard{}, Version{}, folders.ErrMoveIntoPersonal
	}
	if err := s.store.move(ctx, id, source.Folder.ID, folderID, actor.In(ctx, by, "")); err != nil {
		return Dashboard{}, Version{}, err
	}
	dash, ver, err := s.store.latest(ctx, id)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	dash.Abilities = abilities
	s.logger.InfoContext(ctx, "dashboard moved",
		slog.String("actor_id", by.ID), slog.String("dashboard_id", dash.ID), slog.String("folder_id", target.ID.String()))
	return dash, ver, nil
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error {
	access, err := s.need(ctx, by, id, folders.NeedEdit)
	if err != nil {
		return err
	}
	if err := s.store.delete(ctx, id, actor.In(ctx, by, "")); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "dashboard deleted",
		slog.String("actor_id", by.ID), slog.String("dashboard_id", id.String()), slog.String("folder_id", access.Folder.ID.String()))
	return nil
}

func (s *Service) ListVersions(ctx context.Context, id pgtype.UUID, limit int, cursor string) ([]VersionMeta, string, error) {
	before, err := decodeVersionCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	if _, err := s.store.latestVersion(ctx, id); err != nil {
		return nil, "", err
	}
	items, err := s.store.listVersions(ctx, id, before, limit+1)
	if err != nil {
		return nil, "", err
	}
	if len(items) <= limit {
		return items, "", nil
	}
	items = items[:limit]
	return items, encodeCursor(strconv.Itoa(items[len(items)-1].Number)), nil
}

func (s *Service) Diff(ctx context.Context, id pgtype.UUID, from, to *int) ([]spec.Change, int, int, error) {
	var toN, fromN int
	if to != nil {
		toN = *to
	} else {
		latest, err := s.store.latestVersion(ctx, id)
		if err != nil {
			return nil, 0, 0, err
		}
		toN = latest
	}
	if from != nil {
		fromN = *from
	} else {
		fromN = toN - 1
	}
	if fromN < 0 || toN < 1 || fromN >= toN {
		return nil, 0, 0, ErrInvalidRange
	}
	var before *spec.Dashboard
	if fromN > 0 {
		d, err := s.VersionSpec(ctx, id, fromN)
		if err != nil {
			return nil, 0, 0, err
		}
		before = d
	}
	after, err := s.VersionSpec(ctx, id, toN)
	if err != nil {
		return nil, 0, 0, err
	}
	changes := spec.Diff(before, after)
	if before == nil && len(changes) == 1 && changes[0].Op == spec.OpReplace && changes[0].Path == "" {
		changes[0] = spec.Change{Op: spec.OpAdd, Path: "", To: changes[0].To}
	}
	return changes, fromN, toN, nil
}

func (s *Service) checkQueries(ctx context.Context, by auth.Principal, d *spec.Dashboard) error {
	issues, err := s.runner.CheckSpec(ctx, d, time.Now(), queryproxy.UserFor(by))
	if err != nil {
		return err
	}
	if len(issues) > 0 {
		return &InvalidSpecError{Issues: issues}
	}
	return nil
}

func (s *Service) checkSave(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, parsed *spec.Dashboard) (Unverified, error) {
	prevUnverified, err := s.store.unverified(ctx, id, parent)
	if err != nil {
		return Unverified{}, err
	}
	prev, err := s.VersionSpec(ctx, id, parent)
	if err != nil {
		return Unverified{}, err
	}
	return s.verify(ctx, by, prev, prevUnverified, parsed, false)
}

func (s *Service) checkChangedSave(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, parsed *spec.Dashboard) (Unverified, error) {
	prev, err := s.VersionSpec(ctx, id, parent)
	if err != nil {
		return Unverified{}, err
	}
	prevUnverified, err := s.store.unverified(ctx, id, parent)
	if err != nil {
		return Unverified{}, err
	}
	return s.verify(ctx, by, prev, prevUnverified, parsed, true)
}

func (s *Service) verify(ctx context.Context, by auth.Principal, prev *spec.Dashboard, prevUnverified Unverified, next *spec.Dashboard, partial bool) (Unverified, error) {
	if partial && prev != nil && !needsFullCheck(prev, next) {
		panels, variables := ChangedSince(prev, next)
		issues, err := s.runner.CheckChanged(ctx, next, time.Now(), queryproxy.UserFor(by), panels, variables)
		if err != nil {
			return Unverified{}, err
		}
		blocking, accepted := acceptEmpty(ctx, next, issues)
		if len(blocking) > 0 {
			return Unverified{}, &InvalidSpecError{Issues: blocking}
		}
		return CarryUnchanged(prev, prevUnverified, next, panels, variables).with(accepted), nil
	}
	issues, err := s.runner.CheckSpec(ctx, next, time.Now(), queryproxy.UserFor(by))
	if err != nil {
		return Unverified{}, err
	}
	blocking, accepted := acceptEmpty(ctx, next, issues)
	blocking, timedOut := acceptUnchangedFailures(prev, next, blocking)
	accepted = accepted.with(timedOut)
	if prev == nil || prevUnverified.Empty() {
		if len(blocking) > 0 {
			return Unverified{}, &InvalidSpecError{Issues: blocking}
		}
		return accepted, nil
	}
	carried, blocking := CarryUnverified(prev, prevUnverified, next, blocking)
	if len(blocking) > 0 {
		return Unverified{}, &InvalidSpecError{Issues: blocking}
	}
	return carried.with(accepted), nil
}

func (s *Service) replayOrConflict(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, canonical []byte, requestID pgtype.UUID) (Dashboard, Version, bool, error) {
	if requestID.Valid {
		dash, ver, found, err := s.store.savedByRequest(ctx, requestID, id)
		if err != nil || found {
			return dash, ver, found, err
		}
		return Dashboard{}, Version{}, false, s.conflict(ctx, id)
	}
	dash, ver, err := s.store.latest(ctx, id)
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	if ver.ParentVersion != nil && *ver.ParentVersion == parent && ver.Author.ID == by.ID {
		stored, err := json.Marshal(ver.Spec)
		if err != nil {
			return Dashboard{}, Version{}, false, fmt.Errorf("marshal stored spec: %w", err)
		}
		if bytes.Equal(stored, canonical) {
			return dash, ver, true, nil
		}
	}
	return Dashboard{}, Version{}, false, &ConflictError{Latest: ver.Number}
}

func (s *Service) conflict(ctx context.Context, id pgtype.UUID) error {
	latest, err := s.store.latestVersion(ctx, id)
	if err != nil {
		return err
	}
	return &ConflictError{Latest: latest}
}

func validateSave(rawSpec []byte, summary, fallback string, previous *spec.Dashboard) (string, *spec.Dashboard, []byte, error) {
	summary, issues := checkSummary(summary, fallback)
	parsed, canonical, err := parseSpec(rawSpec, previous)
	if err != nil {
		var invalid *InvalidSpecError
		if !errors.As(err, &invalid) {
			return "", nil, nil, err
		}
		issues = append(issues, invalid.Issues...)
	}
	if len(issues) > 0 {
		return "", nil, nil, &InvalidSpecError{Issues: issues}
	}
	return summary, parsed, canonical, nil
}

func parseSpec(raw []byte, previous *spec.Dashboard) (*spec.Dashboard, []byte, error) {
	parsed, err := spec.Parse(raw)
	if err != nil {
		return nil, nil, asInvalidSpec(err)
	}
	if issues := cleanTitles(parsed, previous); len(issues) > 0 {
		return nil, nil, &InvalidSpecError{Issues: issues}
	}
	canonical, err := json.Marshal(parsed)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal spec: %w", err)
	}
	return parsed, canonical, nil
}

func asInvalidSpec(err error) error {
	var validation *spec.ValidationError
	if errors.As(err, &validation) {
		return &InvalidSpecError{Issues: validation.Issues}
	}
	return err
}

func checkSummary(summary, fallback string) (string, []spec.Issue) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = fallback
	}
	switch n := utf8.RuneCountInString(summary); {
	case n == 0:
		return "", summaryIssue("summary must not be empty")
	case n > maxSummaryLength:
		return "", summaryIssue(fmt.Sprintf("summary must be at most %d characters", maxSummaryLength))
	case strings.ContainsRune(summary, 0):
		return "", summaryIssue("summary must not contain the NUL character")
	}
	return summary, nil
}

func summaryIssue(message string) []spec.Issue {
	return []spec.Issue{{Path: "/summary", Code: spec.CodeSchema, Message: message}}
}

func authorOf(p auth.Principal) Author {
	return Author{Kind: string(p.Kind), ID: p.ID, Name: p.Name, Email: p.Email}
}

func encodeCursor(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeCursorValue(cursor string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", ErrInvalidCursor
	}
	return string(raw), nil
}

func decodeVersionCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	value, err := decodeCursorValue(cursor)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > maxInt32 {
		return 0, ErrInvalidCursor
	}
	return n, nil
}
