package public

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/placement"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxLinksPerDashboard = 20
	maxLabelRunes        = 80
)

const (
	reasonExpired              = "expired"
	reasonAdminsOnlyFolder     = "admins_only_folder"
	reasonUserScoped           = "user_scoped"
	reasonRestrictedDatasource = "restricted_datasource"
)

var (
	ErrNotFound        = errors.New("public link not found")
	ErrPersonalFolder  = errors.New("a dashboard in My Dashboards cannot have a public link; move it to a shared folder first")
	ErrAdminsOnly      = errors.New("a dashboard in a folder only Admins can read cannot have public links; move it to a folder that is not Admins only first")
	ErrUserScoped      = errors.New("the dashboard's queries read the viewer's identity, so it cannot be shared publicly")
	ErrInvalidExpiry   = errors.New("expires_at must be in the future")
	ErrUnknownVariable = errors.New("selections name a variable the dashboard does not declare")
	ErrTooManyLinks    = fmt.Errorf("a dashboard has at most %d public links; revoke one first", maxLinksPerDashboard)
	ErrInvalidMode     = errors.New("variables must be frozen or free")
	ErrInvalidLabel    = fmt.Errorf("label must be at most %d characters without control characters", maxLabelRunes)

	ErrOptionsUnavailable = errors.New("the options of a variable could not be read")
)

type adminsOnlyRefusal struct {
	sentinel   error
	noun       string
	datasource string
}

func (e *adminsOnlyRefusal) Error() string {
	return fmt.Sprintf("this %s reads the datasource %q, which only Admins can query, so it lives in a folder only Admins can read and public links are not available for it", e.noun, e.datasource)
}

func (e *adminsOnlyRefusal) Unwrap() error { return e.sentinel }

func refuseAdminsOnly(sentinel error, noun, datasource string) error {
	if datasource == "" {
		return sentinel
	}
	return &adminsOnlyRefusal{sentinel: sentinel, noun: noun, datasource: datasource}
}

func adminsOnlyDatasource(err error) string {
	var refusal *adminsOnlyRefusal
	if errors.As(err, &refusal) {
		return refusal.datasource
	}
	return ""
}

type VariableOptions interface {
	VariableOptions(ctx context.Context, dashboardID pgtype.UUID, version int, d *spec.Dashboard, name string, earlier map[string][]string) ([]string, error)
}

func WithVariableOptions(o VariableOptions) Option {
	return func(s *Service) { s.options = o }
}

type Creator struct {
	Kind string
	ID   string
	Name string
}

type Link struct {
	ID             string
	DashboardID    string
	Prefix         string
	Label          string
	Mode           Mode
	Selections     map[string][]string
	ExpiresAt      *time.Time
	CreatedBy      Creator
	CreatedAt      time.Time
	LastViewedAt   *time.Time
	Expired        bool
	Usable         bool
	UnusableReason string
}

type CreateInput struct {
	Label      string
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
	guard      *placement.Guard
	publicURL  *url.URL
	logger     *slog.Logger
	now        func() time.Time
	rcas       RCAs
	options    VariableOptions
}

type Option func(*Service)

func New(database *db.DB, access *folders.Access, dashboardService *dashboards.Service, registry *datasource.Registry, publicURL *url.URL, logger *slog.Logger, opts ...Option) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{db: database, access: access, dashboards: dashboardService, registry: registry, guard: placement.New(registry), publicURL: publicURL, logger: logger, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) manage(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	_, err := s.managed(ctx, by, dashboardID)
	return err
}

func (s *Service) managed(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) (folders.Folder, error) {
	access, err := s.access.Dashboard(ctx, by, dashboardID)
	if err != nil {
		return folders.Folder{}, err
	}
	if access.Folder.Personal {
		return folders.Folder{}, ErrPersonalFolder
	}
	if !access.Abilities.Manage {
		return folders.Folder{}, forbidden.Need(folders.ErrForbidden, forbidden.NeedsAccess("manage", access.Folder.Title))
	}
	return access.Folder, nil
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

func (s *Service) checkSelections(ctx context.Context, dashboardID pgtype.UUID, version int, d *spec.Dashboard, in CreateInput) error {
	if in.Mode != ModeFree || s.options == nil || len(in.Selections) == 0 {
		return nil
	}
	options := func(name string, earlier map[string][]string) ([]string, error) {
		values, err := s.options.VariableOptions(ctx, dashboardID, version, d, name, earlier)
		if err != nil {
			return nil, fmt.Errorf("%w: %q: %w", ErrOptionsUnavailable, name, err)
		}
		return values, nil
	}
	_, err := EffectiveSelections(ModeFree, d, in.Selections, in.Selections, options)
	return err
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
	label, ok := cleanLabel(in.Label)
	if !ok {
		return Created{}, ErrInvalidLabel
	}
	if err := s.linkable(ctx, by, dashboardID); err != nil {
		return Created{}, err
	}
	version, d, err := s.latest(ctx, dashboardID)
	if err != nil {
		return Created{}, err
	}
	if err := s.checkInput(d, in); err != nil {
		return Created{}, err
	}
	count, err := s.db.Queries().CountPublicLinks(ctx, dbsqlc.CountPublicLinksParams{DashboardID: dashboardID, Now: pgtype.Timestamptz{Time: s.now(), Valid: true}})
	if err != nil {
		return Created{}, err
	}
	if count >= maxLinksPerDashboard {
		return Created{}, ErrTooManyLinks
	}
	if err := s.checkSelections(ctx, dashboardID, version, d, in); err != nil {
		return Created{}, err
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
	row, err := s.insertLink(ctx, by, dbsqlc.CreatePublicLinkParams{
		DashboardID:   dashboardID,
		TokenHash:     hash,
		DisplayPrefix: displayPrefix(token),
		Label:         label,
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
	return Created{Link: judged(link, false, s.dashboardReason(d), s.now()), Token: token, URL: links.Public(s.publicURL, token)}, nil
}

func (s *Service) insertLink(ctx context.Context, by auth.Principal, params dbsqlc.CreatePublicLinkParams) (dbsqlc.CreatePublicLinkRow, error) {
	var row dbsqlc.CreatePublicLinkRow
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockDashboardForLinks(ctx, params.DashboardID); err != nil {
			return err
		}
		adminsOnly, err := q.ShareDashboardFolderAdminsOnly(ctx, params.DashboardID)
		if errors.Is(err, pgx.ErrNoRows) {
			return folders.ErrNotFound
		}
		if err != nil {
			return err
		}
		if adminsOnly {
			datasource, err := s.guard.DashboardDatasource(ctx, q, params.DashboardID)
			if err != nil {
				return err
			}
			return refuseAdminsOnly(ErrAdminsOnly, "dashboard", datasource)
		}
		count, err := q.CountPublicLinks(ctx, dbsqlc.CountPublicLinksParams{DashboardID: params.DashboardID, Now: pgtype.Timestamptz{Time: s.now(), Valid: true}})
		if err != nil {
			return err
		}
		if count >= maxLinksPerDashboard {
			return ErrTooManyLinks
		}
		if row, err = q.CreatePublicLink(ctx, params); err != nil {
			return err
		}
		return recordLink(ctx, q, activity.TypePublicLinkCreated, by, row.ID)
	})
	return row, err
}

func (s *Service) List(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) ([]Link, error) {
	folder, err := s.managed(ctx, by, dashboardID)
	if err != nil {
		return nil, err
	}
	_, d, err := s.latest(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	hidden := s.dashboardReason(d)
	rows, err := s.db.Queries().ListPublicLinks(ctx, dbsqlc.ListPublicLinksParams{DashboardID: dashboardID, Now: pgtype.Timestamptz{Time: s.now(), Valid: true}})
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
		out = append(out, judged(link, folder.AdminsOnly, hidden, now))
	}
	return out, nil
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, dashboardID, linkID pgtype.UUID) error {
	if err := s.manage(ctx, by, dashboardID); err != nil {
		return err
	}
	return s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := recordLink(ctx, q, activity.TypePublicLinkRevoked, by, linkID); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		n, err := q.DeletePublicLink(ctx, dbsqlc.DeletePublicLinkParams{ID: linkID, DashboardID: dashboardID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
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
	if linkReason(r.ExpiresAt, row.AdminsOnly, s.now()) != "" {
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
	if s.dashboardReason(d) != "" {
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
	for _, in := range s.registry.Visible(datasource.WithAccess(ctx, datasource.Access{})) {
		types[in.UID] = in.Datasource.Type()
	}
	return View{Resolved: r, Title: d.Title, Spec: shown, Datasources: referencedDatasources(d, types)}, nil
}

func linkReason(expiresAt *time.Time, adminsOnlyFolder bool, now time.Time) string {
	switch {
	case expired(expiresAt, now):
		return reasonExpired
	case adminsOnlyFolder:
		return reasonAdminsOnlyFolder
	}
	return ""
}

func (s *Service) dashboardReason(latest *spec.Dashboard) string {
	switch {
	case queryproxy.DashboardUsesUser(latest):
		return reasonUserScoped
	case queryproxy.DashboardUsesRestricted(latest, s.registry):
		return reasonRestrictedDatasource
	}
	return ""
}

func judged(link Link, adminsOnlyFolder bool, dashboardReason string, now time.Time) Link {
	link.UnusableReason = cmp.Or(linkReason(link.ExpiresAt, adminsOnlyFolder, now), dashboardReason)
	link.Usable = link.UnusableReason == ""
	link.Expired = link.UnusableReason == reasonExpired
	return link
}

func expired(expiresAt *time.Time, now time.Time) bool {
	return expiresAt != nil && !now.Before(*expiresAt)
}

func linkOf(row dbsqlc.ListPublicLinksRow) (Link, error) {
	link := Link{
		ID:           row.ID.String(),
		DashboardID:  row.DashboardID.String(),
		Prefix:       row.DisplayPrefix,
		Label:        row.Label,
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

func cleanLabel(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return "", true
	}
	label, err := revisions.CheckTitle(raw)
	if err != nil || utf8.RuneCountInString(label) > maxLabelRunes {
		return "", false
	}
	return label, true
}

func (s *Service) linkable(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	folder, err := s.managed(ctx, by, dashboardID)
	if err != nil {
		return err
	}
	if !folder.AdminsOnly {
		return nil
	}
	datasource, err := s.guard.DashboardDatasource(ctx, s.db.Queries(), dashboardID)
	if err != nil {
		return err
	}
	return refuseAdminsOnly(ErrAdminsOnly, "dashboard", datasource)
}
