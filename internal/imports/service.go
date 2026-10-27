package imports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/pkg/grafana"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	MaxBodyBytes      = 8 << 20
	MaxMappingEntries = 50

	importsPerPrincipal  = 1
	importsPerInstance   = 4
	previewsPerPrincipal = 2
	previewsPerInstance  = 4
	defaultLimit         = 50
	maxLimit             = 200
	maxSummaryBytes      = 500
	sourceGrafana        = "grafana"
)

var (
	ErrNotFound      = errors.New("import not found")
	ErrBusy          = errors.New("too many imports in flight")
	ErrInvalidCursor = errors.New("invalid cursor")
)

type UnresolvedError struct {
	Datasources []grafana.SourceDatasource
}

func (e *UnresolvedError) Error() string {
	return "every datasource needs a match before importing"
}

type Request struct {
	JSON      []byte
	Mapping   map[string]string
	RequestID pgtype.UUID
}

type Record struct {
	ID              string
	DashboardID     string
	Version         int
	ImporterVersion int
	Title           string
	Mapping         map[string]string
	Summary         grafana.Summary
	Datasources     []grafana.SourceDatasource
	Report          grafana.Report
	CreatedBy       dashboards.Author
	CreatedAt       time.Time
}

type Meta struct {
	ID              string
	DashboardID     string
	Version         int
	ImporterVersion int
	Title           string
	Counts          grafana.Counts
	CreatedBy       dashboards.Author
	CreatedAt       time.Time
}

type Imported struct {
	Record    Record
	Dashboard dashboards.Dashboard
	Version   dashboards.Version
	Replayed  bool
}

type Service struct {
	db        *db.DB
	dashboard *dashboards.Service
	registry  *datasource.Registry
	logger    *slog.Logger

	importSlots  *slots
	previewSlots *slots
}

func New(database *db.DB, dash *dashboards.Service, registry *datasource.Registry, logger *slog.Logger) *Service {
	return &Service{
		db: database, dashboard: dash, registry: registry, logger: logger,
		importSlots:  newSlots(importsPerPrincipal, importsPerInstance),
		previewSlots: newSlots(previewsPerPrincipal, previewsPerInstance),
	}
}

func (s *Service) Preview(_ context.Context, by auth.Principal, req Request) (*grafana.Result, error) {
	if err := checkLimits(req); err != nil {
		return nil, err
	}
	release, ok := s.previewSlots.acquire(principalKey(by))
	if !ok {
		return nil, ErrBusy
	}
	defer release()
	return grafana.Convert(req.JSON, s.options(req))
}

func (s *Service) Import(ctx context.Context, by auth.Principal, req Request) (Imported, error) {
	if err := checkLimits(req); err != nil {
		return Imported{}, err
	}
	if req.RequestID.Valid {
		dash, ver, found, err := s.dashboard.CreatedByRequest(ctx, req.RequestID)
		if err != nil {
			return Imported{}, err
		}
		if found {
			return s.replay(ctx, dash, ver)
		}
	}
	release, ok := s.importSlots.acquire(principalKey(by))
	if !ok {
		return Imported{}, ErrBusy
	}
	defer release()

	result, err := grafana.Convert(req.JSON, s.options(req))
	if err != nil {
		return Imported{}, err
	}
	if !result.Ready() {
		return Imported{}, &UnresolvedError{Datasources: result.Datasources}
	}
	rawSpec, err := json.Marshal(result.Spec)
	if err != nil {
		return Imported{}, fmt.Errorf("marshal imported spec: %w", err)
	}
	cols, err := encodeColumns(req, result)
	if err != nil {
		return Imported{}, err
	}

	var inserted dbsqlc.InsertImportRow
	hook := func(ctx context.Context, q *dbsqlc.Queries, dash dashboards.Dashboard, ver dashboards.Version, issues []spec.Issue) error {
		result.Verify(issues)
		replaceNULInReasons(&result.Report)
		params, err := insertParams(req, cols, result.Report, dash, ver, by)
		if err != nil {
			return err
		}
		inserted, err = q.InsertImport(ctx, params)
		if err != nil {
			return fmt.Errorf("insert import: %w", err)
		}
		return nil
	}
	dash, ver, _, replayed, err := s.dashboard.CreateImported(ctx, by, rawSpec, importSummary(result.Summary.Title), req.RequestID, hook)
	if err != nil {
		return Imported{}, err
	}
	if replayed {
		return s.replay(ctx, dash, ver)
	}
	imported := Imported{
		Dashboard: dash,
		Version:   ver,
		Record: Record{
			ID:              inserted.ID.String(),
			DashboardID:     dash.ID,
			Version:         ver.Number,
			ImporterVersion: grafana.ImporterVersion,
			Title:           dash.Title,
			Mapping:         mappingOf(req),
			Summary:         result.Summary,
			Datasources:     result.Datasources,
			Report:          result.Report,
			CreatedBy:       dashboards.Author{Kind: string(by.Kind), ID: by.ID, Name: by.Name, Email: by.Email},
			CreatedAt:       inserted.CreatedAt.Time.UTC(),
		},
	}
	s.logImported(ctx, imported)
	return imported, nil
}

func (s *Service) replay(ctx context.Context, dash dashboards.Dashboard, ver dashboards.Version) (Imported, error) {
	imported := Imported{Dashboard: dash, Version: ver, Replayed: true}
	id, err := db.ParseUUID(dash.ID)
	if err != nil {
		return Imported{}, fmt.Errorf("dashboard id: %w", err)
	}
	record, err := s.ForDashboard(ctx, id)
	switch {
	case err == nil:
		imported.Record = record
	case !errors.Is(err, ErrNotFound):
		return Imported{}, err
	}
	s.logImported(ctx, imported)
	return imported, nil
}

func (s *Service) logImported(ctx context.Context, imported Imported) {
	s.logger.InfoContext(ctx, "dashboard imported",
		slog.String("dashboard_id", imported.Dashboard.ID),
		slog.String("import_id", imported.Record.ID),
		slog.Bool("replayed", imported.Replayed),
		slog.Int("panels", imported.Record.Report.Counts.Total),
		slog.Int("unverified", imported.Record.Report.Counts.Unverified),
	)
}

func (s *Service) List(ctx context.Context, limit int, cursor string) ([]Meta, string, error) {
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	limit = min(limit, maxLimit)
	params := dbsqlc.ListImportsParams{RowLimit: httpapi.Int32(limit + 1)}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	rows, err := s.db.Queries().ListImports(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("list imports: %w", err)
	}
	items := make([]Meta, 0, min(len(rows), limit))
	for _, r := range rows {
		var counts grafana.Counts
		if err := json.Unmarshal([]byte(r.Counts), &counts); err != nil {
			return nil, "", fmt.Errorf("decode import counts: %w", err)
		}
		items = append(items, Meta{
			ID:              r.ID.String(),
			DashboardID:     r.DashboardID.String(),
			Version:         int(r.Version),
			ImporterVersion: int(r.ImporterVersion),
			Title:           r.Title,
			Counts:          counts,
			CreatedBy:       author(r.CreatedByKind, r.CreatedByID, r.CreatedByName, r.CreatedByEmail),
			CreatedAt:       r.CreatedAt.Time.UTC(),
		})
	}
	if len(items) <= limit {
		return items, "", nil
	}
	items = items[:limit]
	last := items[len(items)-1]
	return items, httpapi.EncodeTimeCursor(last.CreatedAt, last.ID), nil
}

func (s *Service) ForDashboard(ctx context.Context, dashboardID pgtype.UUID) (Record, error) {
	row, err := s.db.Queries().GetImportByDashboard(ctx, dashboardID)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, _, err := s.dashboard.Get(ctx, dashboardID); err != nil {
			return Record{}, err
		}
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("get import: %w", err)
	}
	record := Record{
		ID:              row.ID.String(),
		DashboardID:     row.DashboardID.String(),
		Version:         int(row.Version),
		ImporterVersion: int(row.ImporterVersion),
		Title:           row.Title,
		CreatedBy:       author(row.CreatedByKind, row.CreatedByID, row.CreatedByName, row.CreatedByEmail),
		CreatedAt:       row.CreatedAt.Time.UTC(),
	}
	for _, column := range []struct {
		name string
		raw  []byte
		into any
	}{
		{"mapping", row.Mapping, &record.Mapping},
		{"summary", row.Summary, &record.Summary},
		{"datasources", row.Datasources, &record.Datasources},
		{"report", row.Report, &record.Report},
	} {
		if err := json.Unmarshal(column.raw, column.into); err != nil {
			return Record{}, fmt.Errorf("decode import %s: %w", column.name, err)
		}
	}
	return record, nil
}

func (s *Service) options(req Request) grafana.Options {
	list := s.registry.List()
	ours := make([]grafana.Datasource, len(list))
	for i, in := range list {
		ours[i] = grafana.Datasource{UID: in.UID, Type: in.Datasource.Type(), Aliases: in.GrafanaUIDs}
	}
	return grafana.Options{Datasources: ours, Mapping: req.Mapping}
}

func principalKey(by auth.Principal) string {
	return string(by.Kind) + ":" + by.ID
}

type slots struct {
	perPrincipal int
	perInstance  int

	mu       sync.Mutex
	byKey    map[string]int
	inFlight int
}

func newSlots(perPrincipal, perInstance int) *slots {
	return &slots{perPrincipal: perPrincipal, perInstance: perInstance, byKey: map[string]int{}}
}

func (s *slots) acquire(key string) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byKey[key] >= s.perPrincipal || s.inFlight >= s.perInstance {
		return nil, false
	}
	s.byKey[key]++
	s.inFlight++
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.byKey[key]--; s.byKey[key] == 0 {
			delete(s.byKey, key)
		}
		s.inFlight--
	}, true
}

type columns struct {
	mapping, summary, datasources []byte
}

func encodeColumns(req Request, result *grafana.Result) (columns, error) {
	var cols columns
	var report []byte
	var issues []spec.Issue
	for _, column := range []struct {
		name string
		from any
		into *[]byte
	}{
		{"mapping", mappingOf(req), &cols.mapping},
		{"summary", result.Summary, &cols.summary},
		{"datasources", result.Datasources, &cols.datasources},
		{"report", result.Report, &report},
	} {
		raw, err := json.Marshal(column.from)
		if err != nil {
			return columns{}, fmt.Errorf("marshal import %s: %w", column.name, err)
		}
		*column.into = raw
		for _, issue := range spec.NULIssues(raw) {
			issue.Path = "/" + column.name + issue.Path
			issues = append(issues, issue)
		}
	}
	if len(issues) > 0 {
		return columns{}, &dashboards.InvalidSpecError{Issues: issues}
	}
	return cols, nil
}

func replaceNULInReasons(report *grafana.Report) {
	clean := func(s string) string { return strings.ReplaceAll(s, "\x00", "\uFFFD") }
	for i := range report.Panels {
		for j, reason := range report.Panels[i].Reasons {
			report.Panels[i].Reasons[j] = clean(reason)
		}
	}
	for i := range report.Notes {
		report.Notes[i].Reason = clean(report.Notes[i].Reason)
	}
}

func insertParams(req Request, cols columns, report grafana.Report, dash dashboards.Dashboard, ver dashboards.Version, by auth.Principal) (dbsqlc.InsertImportParams, error) {
	dashboardID, err := db.ParseUUID(dash.ID)
	if err != nil {
		return dbsqlc.InsertImportParams{}, fmt.Errorf("dashboard id: %w", err)
	}
	authorID, err := db.ParseUUID(by.ID)
	if err != nil {
		return dbsqlc.InsertImportParams{}, fmt.Errorf("author id: %w", err)
	}
	rawReport, err := json.Marshal(report)
	if err != nil {
		return dbsqlc.InsertImportParams{}, fmt.Errorf("marshal import report: %w", err)
	}
	return dbsqlc.InsertImportParams{
		DashboardID:     dashboardID,
		Version:         httpapi.Int32(ver.Number),
		Source:          sourceGrafana,
		ImporterVersion: httpapi.Int32(grafana.ImporterVersion),
		Original:        req.JSON,
		Mapping:         cols.mapping,
		Summary:         cols.summary,
		Datasources:     cols.datasources,
		Report:          rawReport,
		Title:           dash.Title,
		CreatedByKind:   string(by.Kind),
		CreatedByID:     authorID,
		CreatedByName:   by.Name,
		CreatedByEmail:  pgtype.Text{String: by.Email, Valid: by.Email != ""},
	}, nil
}

func checkLimits(req Request) error {
	if len(req.JSON) > MaxBodyBytes {
		return &grafana.InputError{Code: "too_large", Message: fmt.Sprintf("the dashboard JSON is %d bytes; the limit is %d", len(req.JSON), MaxBodyBytes)}
	}
	if len(req.Mapping) > MaxMappingEntries {
		return &grafana.InputError{Code: "invalid_mapping", Message: fmt.Sprintf("the mapping has %d entries; the limit is %d", len(req.Mapping), MaxMappingEntries)}
	}
	return nil
}

func mappingOf(req Request) map[string]string {
	if req.Mapping == nil {
		return map[string]string{}
	}
	return req.Mapping
}

func importSummary(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "Imported from Grafana"
	}
	const prefix, suffix = `Imported "`, `" from Grafana`
	if room := maxSummaryBytes - len(prefix) - len(suffix); len(title) > room {
		cut := room
		for cut > 0 && !utf8.RuneStart(title[cut]) {
			cut--
		}
		title = title[:cut]
	}
	return prefix + title + suffix
}

func author(kind string, id pgtype.UUID, name string, email pgtype.Text) dashboards.Author {
	return dashboards.Author{Kind: kind, ID: id.String(), Name: name, Email: email.String}
}
