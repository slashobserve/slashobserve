package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/singleflight"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

var ErrPrincipalGone = errors.New("principal no longer exists")

type Config struct {
	PublicURL           *url.URL
	IssuerURL           string
	InternalURL         *url.URL
	ClientID            string
	ClientSecret        string
	SessionTTL          time.Duration
	AllowedEmailDomains []string
	AllowAnyEmailDomain bool
	ProviderName        string
	RolePolicy          RolePolicy
}

type Service struct {
	cfg           Config
	queries       *dbsqlc.Queries
	logger        *slog.Logger
	client        *http.Client
	csrf          *http.CrossOriginProtection
	secureCookies bool
	sessionCookie string
	loginCookie   string

	providerMu   sync.Mutex
	discovery    singleflight.Group
	oidcProvider *oidc.Provider
	groupsScope  bool
}

func New(cfg Config, queries *dbsqlc.Queries, logger *slog.Logger) (*Service, error) {
	issuer, err := url.Parse(cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("parse issuer url: %w", err)
	}
	if cfg.PublicURL == nil {
		return nil, errors.New("public url is required")
	}
	csrf := http.NewCrossOriginProtection()
	if err := csrf.AddTrustedOrigin(cfg.PublicURL.Scheme + "://" + cfg.PublicURL.Host); err != nil {
		return nil, fmt.Errorf("trust public origin: %w", err)
	}
	s := &Service{
		cfg:           cfg,
		queries:       queries,
		logger:        logger,
		client:        newBackChannelClient(issuer, cfg.InternalURL),
		csrf:          csrf,
		secureCookies: cfg.PublicURL.Scheme == "https",
		sessionCookie: "so_session",
		loginCookie:   "so_login",
	}
	if s.secureCookies {
		s.sessionCookie = "__Host-" + s.sessionCookie
		s.loginCookie = "__Host-" + s.loginCookie
	}
	return s, nil
}

func (s *Service) Mount(api *gin.RouterGroup) Routes {
	authRoutes := api.Group("/auth", noStore)
	authRoutes.GET("/login", s.login)
	authRoutes.GET("/callback", s.callback)
	authRoutes.POST("/logout", s.logout)
	authRoutes.GET("/session", s.session)

	authenticated := NewRoutes(api.Group("", s.require))
	authenticated.GET("/me", RoleViewer, s.me)
	authenticated.GET("/instance", RoleViewer, s.instance)
	s.mountServiceAccounts(authenticated)
	return authenticated
}

func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Next()
}

func manualRole(stored pgtype.Text) *Role {
	if !stored.Valid {
		return nil
	}
	role, ok := ParseRole(stored.String)
	if !ok {
		return nil
	}
	return &role
}

func setUserRoleParams(id pgtype.UUID, d RoleDecision) dbsqlc.SetUserRoleParams {
	return dbsqlc.SetUserRoleParams{
		ID:         id,
		Role:       string(d.Role),
		RoleSource: string(d.Source),
		RoleGroup:  pgtype.Text{String: d.Group, Valid: d.Group != ""},
	}
}

func (s *Service) assignRole(ctx context.Context, user dbsqlc.UpsertUserRow, groups []string) error {
	first, err := s.queries.IsFirstUser(ctx, user.ID)
	if err != nil {
		return err
	}
	decision := ResolveRole(RoleInputs{Email: user.Email, Groups: groups, FirstUser: first, Manual: manualRole(user.ManualRole)}, s.cfg.RolePolicy)
	return s.queries.SetUserRole(ctx, setUserRoleParams(user.ID, decision))
}

func (s *Service) ReconcileRoles(ctx context.Context) error {
	users, err := s.queries.ListUsersForRoles(ctx)
	if err != nil {
		return fmt.Errorf("list users for roles: %w", err)
	}
	changed := 0
	for i, u := range users {
		decision := ResolveRole(RoleInputs{Email: u.Email, Groups: u.Groups, FirstUser: i == 0, Manual: manualRole(u.ManualRole)}, s.cfg.RolePolicy)
		want := setUserRoleParams(u.ID, decision)
		if u.Role == want.Role && u.RoleSource == want.RoleSource && u.RoleGroup == want.RoleGroup {
			continue
		}
		if err := s.queries.SetUserRole(ctx, want); err != nil {
			return fmt.Errorf("set user role: %w", err)
		}
		changed++
	}
	s.logger.InfoContext(ctx, "roles reconciled", slog.Int("changed", changed))
	return nil
}

func (s *Service) Principal(ctx context.Context, kind PrincipalKind, id string) (Principal, error) {
	var uid pgtype.UUID
	if err := uid.Scan(id); err != nil {
		return Principal{}, ErrPrincipalGone
	}
	switch kind {
	case KindUser:
		row, err := s.queries.GetUserPrincipal(ctx, uid)
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, ErrPrincipalGone
		}
		if err != nil {
			return Principal{}, err
		}
		return Principal{Kind: KindUser, ID: uid.String(), Email: row.Email, Name: row.Name, Role: s.storedRole(ctx, "user_id", id, row.Role), Groups: row.Groups}, nil
	case KindServiceAccount:
		row, err := s.queries.GetServiceAccountPrincipal(ctx, uid)
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, ErrPrincipalGone
		}
		if err != nil {
			return Principal{}, err
		}
		return Principal{Kind: KindServiceAccount, ID: uid.String(), Name: row.Name, Role: s.storedRole(ctx, "service_account_id", id, row.Role)}, nil
	}
	return Principal{}, fmt.Errorf("unknown principal kind %q", kind)
}
