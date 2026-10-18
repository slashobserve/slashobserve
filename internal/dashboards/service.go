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

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxSummaryLength     = 500
	maxNulIssues         = 50
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
	summary, parsed, canonical, err := validateSave(rawSpec, summary, defaultCreateSummary)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	if err := s.checkQueries(ctx, parsed); err != nil {
		return Dashboard{}, Version{}, err
	}
	return s.store.create(ctx, newVersion{spec: parsed, canonical: canonical, summary: summary, author: authorOf(by)})
}

func (s *Service) AddVersion(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, rawSpec []byte, summary string) (Dashboard, Version, bool, error) {
	return s.saveVersion(ctx, by, id, parent, rawSpec, summary, pgtype.UUID{})
}

// A request id makes the save exactly-once: a second save for the same id returns the version the first one
// created, whatever spec or parent it carries.
func (s *Service) AddRequestVersion(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, rawSpec []byte, summary string, requestID pgtype.UUID) (Dashboard, Version, bool, error) {
	return s.saveVersion(ctx, by, id, parent, rawSpec, summary, requestID)
}

// A second save for the same request id returns the dashboard the first one created.
func (s *Service) CreateRequestDashboard(ctx context.Context, by auth.Principal, rawSpec []byte, summary string, requestID pgtype.UUID) (Dashboard, Version, bool, error) {
	if !requestID.Valid {
		dash, ver, err := s.Create(ctx, by, rawSpec, summary)
		return dash, ver, false, err
	}
	if dash, ver, found, err := s.store.createdByRequest(ctx, requestID); err != nil || found {
		return dash, ver, found, err
	}
	summary, parsed, canonical, err := validateSave(rawSpec, summary, defaultCreateSummary)
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	if err := s.checkQueries(ctx, parsed); err != nil {
		return Dashboard{}, Version{}, false, err
	}
	dash, ver, err := s.store.create(ctx, newVersion{spec: parsed, canonical: canonical, summary: summary, author: authorOf(by), requestID: requestID})
	if !errors.Is(err, errRequestSaved) {
		return dash, ver, false, err
	}
	dash, ver, found, err := s.store.createdByRequest(ctx, requestID)
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	if !found {
		return Dashboard{}, Version{}, false, fmt.Errorf("create dashboard: %w", errRequestSaved)
	}
	return dash, ver, true, nil
}

func (s *Service) saveVersion(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, rawSpec []byte, summary string, requestID pgtype.UUID) (Dashboard, Version, bool, error) {
	if requestID.Valid {
		if dash, ver, found, err := s.store.versionOfRequest(ctx, id, requestID); err != nil || found {
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
	if err := s.checkQueries(ctx, parsed); err != nil {
		return Dashboard{}, Version{}, false, err
	}
	dash, ver, err := s.store.addVersion(ctx, newVersion{dashboardID: id, parent: parent, spec: parsed, canonical: canonical, summary: summary, author: authorOf(by), requestID: requestID})
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
	dash, ver, err := s.store.addVersion(ctx, newVersion{dashboardID: id, parent: parent, restoredFrom: &from, spec: parsed, canonical: canonical, summary: summary, author: authorOf(by)})
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

func (s *Service) LatestVersion(ctx context.Context, id pgtype.UUID) (int, error) {
	return s.store.latestVersion(ctx, id)
}

func (s *Service) List(ctx context.Context, limit int, cursor string) ([]Dashboard, string, error) {
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	items, err := s.store.list(ctx, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	if len(items) <= limit {
		return items, "", nil
	}
	items = items[:limit]
	last := items[len(items)-1]
	return items, httpapi.EncodeTimeCursor(last.UpdatedAt, last.ID), nil
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

func (s *Service) checkQueries(ctx context.Context, d *spec.Dashboard) error {
	issues, err := s.runner.CheckSpec(ctx, d, time.Now())
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
		dash, ver, found, err := s.store.versionOfRequest(ctx, id, requestID)
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
	if issues := nulIssues(canonical); len(issues) > 0 {
		return nil, nil, &InvalidSpecError{Issues: issues}
	}
	return parsed, canonical, nil
}

func nulIssues(canonical []byte) []spec.Issue {
	dec := json.NewDecoder(bytes.NewReader(canonical))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil
	}
	var issues []spec.Issue
	walkStrings(tree, "", func(pointer, kind string) bool {
		issues = append(issues, spec.Issue{Path: pointer, Code: spec.CodeSchema, Message: kind + " must not contain the NUL character"})
		return len(issues) < maxNulIssues
	})
	return issues
}

func walkStrings(node any, pointer string, report func(pointer, kind string) bool) bool {
	switch v := node.(type) {
	case string:
		if strings.ContainsRune(v, 0) {
			return report(pointer, "value")
		}
	case []any:
		for i, child := range v {
			if !walkStrings(child, pointer+"/"+strconv.Itoa(i), report) {
				return false
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			childPointer := pointer + "/" + escapePointer(k)
			if strings.ContainsRune(k, 0) && !report(childPointer, "property name") {
				return false
			}
			if !walkStrings(v[k], childPointer, report) {
				return false
			}
		}
	}
	return true
}

var pointerEscaper = strings.NewReplacer("~", "~0", "/", "~1")

func escapePointer(s string) string {
	return pointerEscaper.Replace(s)
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
