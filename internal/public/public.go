package public

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxLinksPerDashboard = 20

const (
	reasonExpired    = "expired"
	reasonUserScoped = "user_scoped"
)

var (
	ErrNotFound        = errors.New("public link not found")
	ErrPersonalFolder  = errors.New("a dashboard in My Dashboards cannot have a public link; move it to a shared folder first")
	ErrUserScoped      = errors.New("the dashboard's queries read the viewer's identity, so it cannot be shared publicly")
	ErrInvalidExpiry   = errors.New("expires_at must be in the future")
	ErrUnknownVariable = errors.New("selections name a variable the dashboard does not declare")
	ErrTooManyLinks    = fmt.Errorf("a dashboard has at most %d public links; revoke one first", maxLinksPerDashboard)
	ErrInvalidMode     = errors.New("variables must be frozen or free")
)

type Creator struct {
	Kind string
	ID   string
	Name string
}

type Link struct {
	ID           string
	DashboardID  string
	Prefix       string
	Mode         Mode
	Selections   map[string][]string
	ExpiresAt    *time.Time
	CreatedBy    Creator
	CreatedAt    time.Time
	LastViewedAt *time.Time
	Expired      bool
	Usable       bool
}

type CreateInput struct {
	Mode       Mode
	Selections map[string][]string
	ExpiresAt  *time.Time
}

type Created struct {
	Link  Link
	Token string
	URL   string
}

type Resolved struct {
	LinkID      pgtype.UUID
	DashboardID pgtype.UUID
	Version     int
	Mode        Mode
	Selections  map[string][]string
	ExpiresAt   *time.Time
}

type View struct {
	Resolved
	Title       string
	Spec        *spec.Dashboard
	Datasources []Datasource
}

type Service struct {
	db         *db.DB
	access     *folders.Access
	dashboards *dashboards.Service
	registry   *datasource.Registry
	publicURL  *url.URL
	logger     *slog.Logger
	now        func() time.Time
	rcas       RCAs
}

type Option func(*Service)

func New(database *db.DB, access *folders.Access, dashboardService *dashboards.Service, registry *datasource.Registry, publicURL *url.URL, logger *slog.Logger, opts ...Option) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{db: database, access: access, dashboards: dashboardService, registry: registry, publicURL: publicURL, logger: logger, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) manage(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	access, err := s.access.Dashboard(ctx, by, dashboardID)
	if err != nil {
		return err
	}
	if access.Folder.Personal {
		return ErrPersonalFolder
	}
	if !access.Abilities.Manage {
		return folders.ErrForbidden
	}
	return nil
}

func (s *Service) latest(ctx context.Context, dashboardID pgtype.UUID) (int, *spec.Dashboard, error) {
	version, err := s.dashboards.LatestVersion(ctx, dashboardID)
	if err != nil {
		return 0, nil, err
	}
	d, err := s.dashboards.VersionSpec(ctx, dashboardID, version)
	return version, d, err
}

func (s *Service) checkInput(d *spec.Dashboard, in CreateInput) error {
	if queryproxy.DashboardUsesUser(d) {
		return ErrUserScoped
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(s.now()) {
		return ErrInvalidExpiry
	}
	for name := range in.Selections {
		if !slices.ContainsFunc(d.Variables, func(v spec.Variable) bool { return v.Name == name }) {
			return fmt.Errorf("%w: %q", ErrUnknownVariable, name)
		}
	}
	return nil
}

func storedSelections(selections map[string][]string) ([]byte, error) {
	if selections == nil {
		selections = map[string][]string{}
	}
	return json.Marshal(selections)
}

func (s *Service) Create(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID, in CreateInput) (Created, error) {
	if !in.Mode.valid() {
		return Created{}, ErrInvalidMode
	}
	if err := s.manage(ctx, by, dashboardID); err != nil {
		return Created{}, err
	}
	_, d, err := s.latest(ctx, dashboardID)
	if err != nil {
		return Created{}, err
	}
	if err := s.checkInput(d, in); err != nil {
		return Created{}, err
	}
	count, err := s.db.Queries().CountPublicLinks(ctx, dashboardID)
	if err != nil {
		return Created{}, err
	}
	if count >= maxLinksPerDashboard {
		return Created{}, ErrTooManyLinks
	}
	creatorID, err := db.ParseUUID(by.ID)
	if err != nil {
		return Created{}, folders.ErrForbidden
	}
	stored, err := storedSelections(in.Selections)
	if err != nil {
		return Created{}, err
	}
	token, hash, err := opaquetoken.New(tokenPrefix)
	if err != nil {
		return Created{}, err
	}
	row, err := s.db.Queries().CreatePublicLink(ctx, dbsqlc.CreatePublicLinkParams{
		DashboardID:   dashboardID,
		TokenHash:     hash,
		DisplayPrefix: displayPrefix(token),
		Variables:     string(in.Mode),
		Selections:    stored,
		ExpiresAt:     timestamp(in.ExpiresAt),
		CreatedByKind: string(by.Kind),
		CreatedByID:   creatorID,
		CreatedByName: creatorName(by),
	})
	if db.IsForeignKeyViolation(err) {
		return Created{}, folders.ErrNotFound
	}
	if err != nil {
		return Created{}, err
	}
	link, err := linkOf(dbsqlc.ListPublicLinksRow(row))
	if err != nil {
		return Created{}, err
	}
	link.Usable = true
	return Created{Link: link, Token: token, URL: links.Public(s.publicURL, token)}, nil
}

func (s *Service) List(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) ([]Link, error) {
	if err := s.manage(ctx, by, dashboardID); err != nil {
		return nil, err
	}
	_, d, err := s.latest(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	usesUser := queryproxy.DashboardUsesUser(d)
	rows, err := s.db.Queries().ListPublicLinks(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := make([]Link, 0, len(rows))
	for _, row := range rows {
		link, err := linkOf(row)
		if err != nil {
			return nil, err
		}
		var reason string
		link.Usable, reason = usable(link, usesUser, now)
		link.Expired = reason == reasonExpired
		out = append(out, link)
	}
	return out, nil
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, dashboardID, linkID pgtype.UUID) error {
	if err := s.manage(ctx, by, dashboardID); err != nil {
		return err
	}
	n, err := s.db.Queries().DeletePublicLink(ctx, dbsqlc.DeletePublicLinkParams{ID: linkID, DashboardID: dashboardID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Resolve(ctx context.Context, token string) (Resolved, error) {
	if !validTokenShape(token) {
		return Resolved{}, ErrNotFound
	}
	row, err := s.db.Queries().PublicLinkByTokenHash(ctx, opaquetoken.Hash(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Resolved{}, ErrNotFound
	}
	if err != nil {
		return Resolved{}, err
	}
	r := Resolved{LinkID: row.ID, DashboardID: row.DashboardID, Version: int(row.LatestVersion), Mode: Mode(row.Variables), ExpiresAt: timeOf(row.ExpiresAt)}
	if expired(r.ExpiresAt, s.now()) {
		return Resolved{}, ErrNotFound
	}
	if err := json.Unmarshal(row.Selections, &r.Selections); err != nil {
		return Resolved{}, fmt.Errorf("decode public link selections: %w", err)
	}
	return r, nil
}

func (s *Service) View(ctx context.Context, token string) (View, error) {
	r, err := s.Resolve(ctx, token)
	if err != nil {
		return View{}, err
	}
	d, err := s.dashboards.VersionSpec(ctx, r.DashboardID, r.Version)
	if errors.Is(err, dashboards.ErrNotFound) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, err
	}
	if queryproxy.DashboardUsesUser(d) {
		return View{}, ErrNotFound
	}
	shown, err := publicSpec(d)
	if err != nil {
		return View{}, err
	}
	if err := s.db.Queries().TouchPublicLink(ctx, r.LinkID); err != nil {
		s.logger.WarnContext(ctx, "record a public link view", logattrs.Failure("internal", err, slog.String("public_link_id", r.LinkID.String()))...)
	}
	types := map[string]string{}
	for _, in := range s.registry.List() {
		types[in.UID] = in.Datasource.Type()
	}
	return View{Resolved: r, Title: d.Title, Spec: shown, Datasources: referencedDatasources(d, types)}, nil
}

func usable(link Link, latestUsesUser bool, now time.Time) (bool, string) {
	switch {
	case expired(link.ExpiresAt, now):
		return false, reasonExpired
	case latestUsesUser:
		return false, reasonUserScoped
	}
	return true, ""
}

func expired(expiresAt *time.Time, now time.Time) bool {
	return expiresAt != nil && !now.Before(*expiresAt)
}

func linkOf(row dbsqlc.ListPublicLinksRow) (Link, error) {
	link := Link{
		ID:           row.ID.String(),
		DashboardID:  row.DashboardID.String(),
		Prefix:       row.DisplayPrefix,
		Mode:         Mode(row.Variables),
		ExpiresAt:    timeOf(row.ExpiresAt),
		CreatedBy:    Creator{Kind: row.CreatedByKind, ID: row.CreatedByID.String(), Name: row.CreatedByName},
		CreatedAt:    row.CreatedAt.Time,
		LastViewedAt: timeOf(row.LastViewedAt),
	}
	if err := json.Unmarshal(row.Selections, &link.Selections); err != nil {
		return Link{}, fmt.Errorf("decode public link selections: %w", err)
	}
	return link, nil
}

func creatorName(by auth.Principal) string {
	if by.Name != "" {
		return by.Name
	}
	return by.Email
}

func timestamp(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func timeOf(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
