package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type Config struct {
	PublicURL           *url.URL
	IssuerURL           string
	InternalURL         *url.URL
	ClientID            string
	ClientSecret        string
	SessionTTL          time.Duration
	AllowedEmailDomains []string
	AllowAnyEmailDomain bool
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

func (s *Service) Mount(api *gin.RouterGroup) *gin.RouterGroup {
	authRoutes := api.Group("/auth", noStore)
	authRoutes.GET("/login", s.login)
	authRoutes.GET("/callback", s.callback)
	authRoutes.POST("/logout", s.logout)
	authRoutes.GET("/session", s.session)

	authenticated := api.Group("", s.require)
	authenticated.GET("/me", s.me)
	s.mountServiceAccounts(authenticated)
	return authenticated
}

func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Next()
}
