package mcpauth

import (
	"context"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type People interface {
	Principal(ctx context.Context, kind auth.PrincipalKind, id string) (auth.Principal, error)
}

type Config struct {
	PublicURL *url.URL
	Logger    *slog.Logger
}

type route struct {
	path    string
	handler http.HandlerFunc
}

type Server struct {
	db       *db.DB
	people   People
	logger   *slog.Logger
	issuer   string
	resource string
	routes   []route
	mux      *http.ServeMux

	registerLimit  *limiter
	authorizeLimit *limiter
	tokenLimit     *limiter
	tokenClient    *limiter
	revokeLimit    *limiter
}

func New(cfg Config, database *db.DB, people People) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		db:             database,
		people:         people,
		logger:         logger,
		issuer:         strings.TrimSuffix(cfg.PublicURL.String(), "/"),
		resource:       cfg.PublicURL.JoinPath("mcp").String(),
		mux:            http.NewServeMux(),
		registerLimit:  newLimiter(registerPerMinute, limitWindow),
		authorizeLimit: newLimiter(authorizePerMinute, limitWindow),
		tokenLimit:     newLimiter(tokenPerMinute, limitWindow),
		tokenClient:    newLimiter(tokenPerClientPerMinute, limitWindow),
		revokeLimit:    newLimiter(revokePerMinute, limitWindow),
	}
	resourceMetadata := s.protectedResourceMetadata()
	s.routes = []route{
		{"/.well-known/oauth-protected-resource/mcp", resourceMetadata},
		{"/.well-known/oauth-protected-resource", resourceMetadata},
		{"/.well-known/oauth-authorization-server", s.authorizationServerMetadata},
		{"/oauth/authorize", s.authorize},
		{"/oauth/token", s.token},
		{"/oauth/register", s.register},
		{"/oauth/revoke", s.revoke},
	}
	for _, r := range s.routes {
		s.mux.HandleFunc(r.path, r.handler)
	}
	return s
}

func (s *Server) Paths() []string {
	paths := make([]string, len(s.routes))
	for i, r := range s.routes {
		paths[i] = r.path
	}
	return paths
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) ResourceMetadataURL() string {
	return s.issuer + "/.well-known/oauth-protected-resource/mcp"
}

type oauthError struct {
	status      int
	code        string
	description string
}

func (e *oauthError) Error() string { return e.code }

var (
	errInvalidGrant  = &oauthError{http.StatusBadRequest, "invalid_grant", "the grant is invalid, expired or already used"}
	errInvalidTarget = &oauthError{http.StatusBadRequest, "invalid_target", "resource must be this server's MCP URL"}
)

func invalidRequest(description string) *oauthError {
	return &oauthError{http.StatusBadRequest, "invalid_request", description}
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, action, code string, err error) {
	if clientgone.Is(r.Context(), err) {
		clientgone.WriteHeader(w, r, s.logger, action)
		return
	}
	s.logger.ErrorContext(r.Context(), action, logattrs.Failure(code, err)...)
	writeOAuthError(w, &oauthError{http.StatusInternalServerError, "server_error", "internal error"})
}

func writeOAuthError(w http.ResponseWriter, e *oauthError) {
	noStore(w)
	if e.status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="slashobserve"`)
	}
	writeJSON(w, e.status, map[string]string{"error": e.code, "error_description": e.description})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func setCORS(w http.ResponseWriter, methods string) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", methods)
	h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, MCP-Protocol-Version")
}

func (s *Server) preflightOrMethod(w http.ResponseWriter, r *http.Request) bool {
	setCORS(w, "POST, OPTIONS")
	noStore(w)
	switch r.Method {
	case http.MethodPost:
		return true
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "POST, OPTIONS")
		writeOAuthError(w, &oauthError{http.StatusMethodNotAllowed, "invalid_request", "use POST"})
	}
	return false
}

func tooManyRequests(w http.ResponseWriter, l *limiter) {
	w.Header().Set("Retry-After", strconv.Itoa(l.retryAfterSeconds()))
	writeOAuthError(w, &oauthError{http.StatusTooManyRequests, "temporarily_unavailable", "too many requests"})
}

func readForm(w http.ResponseWriter, r *http.Request) (url.Values, *oauthError) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		return nil, invalidRequest("Content-Type must be application/x-www-form-urlencoded")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		return nil, invalidRequest("the form body is invalid or too large")
	}
	return r.PostForm, nil
}

func single(values url.Values, key string) (string, bool) {
	vs := values[key]
	switch len(vs) {
	case 0:
		return "", true
	case 1:
		return vs[0], true
	}
	return "", false
}

func canonicalEqual(a, b string) bool {
	return canonicalResource(a) == canonicalResource(b)
}
