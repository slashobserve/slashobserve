package imports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

	maxPerPrincipal = 1
	maxPerInstance  = 4
	defaultLimit    = 50
	maxLimit        = 200
	maxSummaryBytes = 500
	sourceGrafana   = "grafana"
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

	mu           sync.Mutex
	perPrincipal map[string]int
	inFlight     int
}

func New(database *db.DB, dash *dashboards.Service, registry *datasource.Registry, logger *slog.Logger) *Service {
	return &Service{db: database, dashboard: dash, registry: registry, logger: logger, perPrincipal: map[string]int{}}
}

func (s *Service) Preview(_ context.Context, req Request) (*grafana.Result, error) {
	return grafana.Convert(req.JSON, s.options(req))
}

func (s *Service) Import(ctx context.Context, by auth.Principal, req Request) (Imported, error) {
	release, ok := s.acquire(string(by.Kind) + ":" + by.ID)
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
	imported := Imported{Dashboard: dash, Version: ver, Replayed: replayed}
	if replayed {
		id, err := db.ParseUUID(dash.ID)
		if err != nil {
			return Imported{}, fmt.Errorf("dashboard id: %w", err)
		}
		if imported.Record, err = s.ForDashboard(ctx, id); err != nil {
			return Imported{}, err
		}
	} else {
		imported.Record = Record{
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
		}
	}
	s.logger.InfoContext(ctx, "dashboard imported",
		slog.String("dashboard_id", dash.ID),
		slog.String("import_id", imported.Record.ID),
		slog.Bool("replayed", replayed),
		slog.Int("panels", imported.Record.Report.Counts.Total),
		slog.Int("unverified", imported.Record.Report.Counts.Unverified),
	)
	return imported, nil
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

func (s *Service) acquire(key string) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.perPrincipal[key] >= maxPerPrincipal || s.inFlight >= maxPerInstance {
		return nil, false
	}
	s.perPrincipal[key]++
	s.inFlight++
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.perPrincipal[key]--; s.perPrincipal[key] == 0 {
			delete(s.perPrincipal, key)
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
		{"datasources", datasourcesOf(result), &cols.datasources},
		{"report", result.Report, &report},
	} {
		raw, err := json.Marshal(column.from)
		if err != nil {
			return columns{}, fmt.Errorf("marshal import %s: %w", column.name, err)
		}
		*column.into = raw
		for _, issue := range dashboards.NULIssues(raw) {
			issue.Path = "/" + column.name + issue.Path
			issues = append(issues, issue)
		}
	}
	if len(issues) > 0 {
		return columns{}, &dashboards.InvalidSpecError{Issues: issues}
	}
	return cols, nil
}

func datasourcesOf(result *grafana.Result) []grafana.SourceDatasource {
	if result.Datasources == nil {
		return []grafana.SourceDatasource{}
	}
	return result.Datasources
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

func mappingOf(req Request) map[string]string {
	if req.Mapping == nil {
		return map[string]string{}
	}
	return req.Mapping
}

func importSummary(title string) string {
	if title == "" {
		return "Imported from Grafana"
	}
	summary := `Imported "` + title + `" from Grafana`
	if len(summary) <= maxSummaryBytes {
		return summary
	}
	cut := maxSummaryBytes
	for cut > 0 && !utf8.RuneStart(summary[cut]) {
		cut--
	}
	return summary[:cut]
}

func author(kind string, id pgtype.UUID, name string, email pgtype.Text) dashboards.Author {
	return dashboards.Author{Kind: kind, ID: id.String(), Name: name, Email: email.String}
}
