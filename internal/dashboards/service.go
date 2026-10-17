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
	"github.com/slashobserve/slashobserve/internal/queryproxy"
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
	summary, err := summaryOrDefault(summary, defaultCreateSummary)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	parsed, canonical, err := parseSpec(rawSpec)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	if err := s.checkQueries(ctx, parsed); err != nil {
		return Dashboard{}, Version{}, err
	}
	return s.store.create(ctx, newVersion{spec: parsed, canonical: canonical, summary: summary, author: authorOf(by)})
}

func (s *Service) AddVersion(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, rawSpec []byte, summary string) (Dashboard, Version, bool, error) {
	summary, err := summaryOrDefault(summary, "")
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	parsed, canonical, err := parseSpec(rawSpec)
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	latest, err := s.store.latestVersion(ctx, id)
	if err != nil {
		return Dashboard{}, Version{}, false, err
	}
	if latest != parent {
		return s.replayOrConflict(ctx, by, id, parent, canonical)
	}
	if err := s.checkQueries(ctx, parsed); err != nil {
		return Dashboard{}, Version{}, false, err
	}
	dash, ver, err := s.store.addVersion(ctx, newVersion{dashboardID: id, parent: parent, spec: parsed, canonical: canonical, summary: summary, author: authorOf(by)})
	if errors.Is(err, errStaleParent) {
		return s.replayOrConflict(ctx, by, id, parent, canonical)
	}
	return dash, ver, false, err
}

func (s *Service) Restore(ctx context.Context, by auth.Principal, id pgtype.UUID, from, parent int, summary string) (Dashboard, Version, error) {
	summary, err := summaryOrDefault(summary, "Restore version "+strconv.Itoa(from))
	if err != nil {
		return Dashboard{}, Version{}, err
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
	after, err := decodeDashboardCursor(cursor)
	if err != nil {
		return nil, "", err
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
	return items, encodeDashboardCursor(last), nil
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
	return spec.Diff(before, after), fromN, toN, nil
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

func (s *Service) replayOrConflict(ctx context.Context, by auth.Principal, id pgtype.UUID, parent int, canonical []byte) (Dashboard, Version, bool, error) {
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

func parseSpec(raw []byte) (*spec.Dashboard, []byte, error) {
	parsed, err := spec.Parse(raw)
	if err != nil {
		return nil, nil, asInvalidSpec(err)
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

func summaryOrDefault(summary, fallback string) (string, error) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = fallback
	}
	switch n := utf8.RuneCountInString(summary); {
	case n == 0:
		return "", &InvalidSpecError{Issues: []spec.Issue{{Path: "/summary", Code: spec.CodeSchema, Message: "summary must not be empty"}}}
	case n > maxSummaryLength:
		return "", &InvalidSpecError{Issues: []spec.Issue{{Path: "/summary", Code: spec.CodeSchema, Message: fmt.Sprintf("summary must be at most %d characters", maxSummaryLength)}}}
	}
	return summary, nil
}

func authorOf(p auth.Principal) Author {
	return Author{Kind: string(p.Kind), ID: p.ID, Name: p.Name, Email: p.Email}
}

type dashboardCursor struct {
	updatedAt time.Time
	id        pgtype.UUID
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

func encodeDashboardCursor(d Dashboard) string {
	return encodeCursor(strconv.FormatInt(d.UpdatedAt.UnixMicro(), 10) + ":" + d.ID)
}

func decodeDashboardCursor(cursor string) (*dashboardCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	value, err := decodeCursorValue(cursor)
	if err != nil {
		return nil, err
	}
	micros, idText, ok := strings.Cut(value, ":")
	if !ok {
		return nil, ErrInvalidCursor
	}
	n, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	id, err := parseUUID(idText)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	return &dashboardCursor{updatedAt: time.UnixMicro(n).UTC(), id: id}, nil
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
