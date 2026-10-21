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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxSummaryLength     = 500
	maxInt32             = math.MaxInt32
	defaultCreateSummary = "Create dashboard"
)

var (
	ErrNotFound      = errors.New("dashboard not found")
	ErrInvalidCursor = errors.New("invalid cursor")
	ErrInvalidRange  = errors.New("from must be less than to")
)

type ConflictError struct {
	Latest int
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

type Service struct {
	store  store
	runner *queryproxy.Runner
	logger *slog.Logger
}

func New(database *db.DB, runner *queryproxy.Runner, logger *slog.Logger) *Service {
	return &Service{store: store{db: database}, runner: runner, logger: logger}
}

func (s *Service) Create(ctx context.Context, by auth.Principal, rawSpec []byte, summary string) (Dashboard, Version, error) {
	return s.create(ctx, by, rawSpec, summary, pgtype.UUID{})
}

func (s *Service) AddVersion(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, rawSpec []byte, summary string) (Dashboard, Version, bool, error) {
	return s.saveVersion(ctx, by, id, parent, rawSpec, summary, pgtype.UUID{})
}

// A request id makes the save exactly-once: a second save for the same id returns the version the first one
// created, whatever spec or parent it carries.
func (s *Service) AddRequestVersion(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, rawSpec []byte, summary string, requestID pgtype.UUID) (Dashboard, Version, bool, error) {
	return s.saveVersion(ctx, by, id, parent, rawSpec, summary, requestID)
}

func (s *Service) CreateRequestDashboard(ctx context.Context, by auth.Principal, rawSpec []byte, summary string, requestID pgtype.UUID) (Dashboard, Version, bool, error) {
	if !requestID.Valid {
		dash, ver, err := s.Create(ctx, by, rawSpec, summary)
		return dash, ver, false, err
	}
	if dash, ver, found, err := s.CreatedByRequest(ctx, requestID); err != nil || found {
		return dash, ver, found, err
	}
	dash, ver, err := s.create(ctx, by, rawSpec, summary, requestID)
	if !errors.Is(err, errRequestSaved) {
		return dash, ver, false, err
	}
	return s.replayedCreate(ctx, requestID)
}

type ImportHook func(ctx context.Context, q *dbsqlc.Queries, dash Dashboard, ver Version, issues []spec.Issue) error

func (s *Service) CreateImported(ctx context.Context, by auth.Principal, rawSpec []byte, summary string, requestID pgtype.UUID, hook ImportHook) (Dashboard, Version, []spec.Issue, bool, error) {
	if requestID.Valid {
		if dash, ver, found, err := s.CreatedByRequest(ctx, requestID); err != nil || found {
			return dash, ver, nil, found, err
		}
	}
	summary, parsed, canonical, err := validateSave(rawSpec, summary, defaultCreateSummary)
	if err != nil {
		return Dashboard{}, Version{}, nil, false, err
	}
	issues, err := s.runner.CheckSpec(ctx, parsed, time.Now(), queryproxy.UserFor(by))
	if err != nil {
		return Dashboard{}, Version{}, nil, false, err
	}
	in := newVersion{spec: parsed, canonical: canonical, summary: summary, author: authorOf(by), requestID: requestID, origin: origin.From(ctx)}
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
	return dash, ver, issues, false, err
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

func (s *Service) create(ctx context.Context, by auth.Principal, rawSpec []byte, summary string, requestID pgtype.UUID) (Dashboard, Version, error) {
	summary, parsed, canonical, err := validateSave(rawSpec, summary, defaultCreateSummary)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	if err := s.checkQueries(ctx, by, parsed); err != nil {
		return Dashboard{}, Version{}, err
	}
	return s.store.create(ctx, newVersion{spec: parsed, canonical: canonical, summary: summary, author: authorOf(by), requestID: requestID, origin: origin.From(ctx)})
}

func (s *Service) saveVersion(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, rawSpec []byte, summary string, requestID pgtype.UUID) (Dashboard, Version, bool, error) {
	if requestID.Valid {
		if dash, ver, found, err := s.store.savedByRequest(ctx, requestID, id); err != nil || found {
			return dash, ver, found, err
		}
	}
	summary, parsed, canonical, err := validateSave(rawSpec, summary, "")
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	latest, err := s.store.latestVersion(ctx, id)
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	if latest != parent {
		return s.replayOrConflict(ctx, by, id, parent, canonical, requestID)
	}
	if err := s.checkQueries(ctx, by, parsed); err != nil {
		return Dashboard{}, Version{}, false, err
	}
	dash, ver, err := s.store.addVersion(ctx, newVersion{dashboardID: id, parent: parent, spec: parsed, canonical: canonical, summary: summary, author: authorOf(by), requestID: requestID, origin: origin.From(ctx)})
	if errors.Is(err, errStaleParent) || errors.Is(err, errRequestSaved) {
		return s.replayOrConflict(ctx, by, id, parent, canonical, requestID)
	}
	return dash, ver, false, err
}

func (s *Service) Restore(ctx context.Context, by auth.Principal, id pgtype.UUID, from, parent int, summary string) (Dashboard, Version, error) {
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
	if issues := s.runner.CheckDatasources(parsed); len(issues) > 0 {
		return Dashboard{}, Version{}, &InvalidSpecError{Issues: issues}
	}
	canonical, err := json.Marshal(parsed)
	if err != nil {
		return Dashboard{}, Version{}, fmt.Errorf("marshal spec: %w", err)
	}
	dash, ver, err := s.store.addVersion(ctx, newVersion{dashboardID: id, parent: parent, restoredFrom: &from, spec: parsed, canonical: canonical, summary: summary, author: authorOf(by), origin: origin.From(ctx)})
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

func (s *Service) HasPanel(ctx context.Context, id pgtype.UUID, version int, panelID string) (bool, error) {
	d, err := s.VersionSpec(ctx, id, version)
	if errors.Is(err, ErrNotFound) {
		return false, revisions.ErrDashboardNotFound
	}
	if err != nil {
		return false, err
	}
	_, ok := panelSet(d)[panelID]
	return ok, nil
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
	Limit      int
	Cursor     string
	MadeBy     MadeBy
	Datasource string
	Query      string
	SkipTotal  bool
}

type ListItem struct {
	Dashboard
	Description    string
	DatasourceUIDs []string
	OpenComments   int
}

type ListPage struct {
	Items      []ListItem
	NextCursor string
	Total      int
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

func (s *Service) List(ctx context.Context, p ListParams) (ListPage, error) {
	after, ok := httpapi.DecodeTimeCursor(p.Cursor)
	if !ok {
		return ListPage{}, ErrInvalidCursor
	}
	filter, err := filterOf(p)
	if err != nil {
		return ListPage{}, err
	}
	items, err := s.store.list(ctx, after, p.Limit+1, filter)
	if err != nil {
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
		last := page.Items[len(page.Items)-1]
		page.NextCursor = httpapi.EncodeTimeCursor(last.UpdatedAt, last.ID)
	}
	return page, nil
}

func (s *Service) DatasourceUsage(ctx context.Context) (map[string]int, error) {
	rows, err := s.store.db.Queries().CountDashboardsByDatasource(ctx)
	if err != nil {
		return nil, fmt.Errorf("count dashboards by datasource: %w", err)
	}
	usage := make(map[string]int, len(rows))
	for _, row := range rows {
		usage[row.Uid] = int(row.Dashboards)
	}
	return usage, nil
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

func validateSave(rawSpec []byte, summary, fallback string) (string, *spec.Dashboard, []byte, error) {
	summary, issues := checkSummary(summary, fallback)
	parsed, canonical, err := parseSpec(rawSpec)
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

func parseSpec(raw []byte) (*spec.Dashboard, []byte, error) {
	parsed, err := spec.Parse(raw)
	if err != nil {
		return nil, nil, asInvalidSpec(err)
	}
	canonical, err := json.Marshal(parsed)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal spec: %w", err)
	}
	if issues := spec.NULIssues(canonical); len(issues) > 0 {
		return nil, nil, &InvalidSpecError{Issues: issues}
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
