package mcpauth

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type authorizeParams struct {
	state         string
	codeChallenge string
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		refusePage(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	if !s.authorizeLimit.allow("") {
		w.Header().Set("Retry-After", strconv.Itoa(s.authorizeLimit.retryAfterSeconds()))
		refusePage(w, http.StatusTooManyRequests, "too many connection attempts, wait a minute")
		return
	}
	q := r.URL.Query()
	client, redirectURI, reason := s.resolveClient(r, q)
	if reason != "" {
		if reason == internalReason {
			refusePage(w, http.StatusInternalServerError, "the server could not read the connection request")
			return
		}
		refusePage(w, http.StatusBadRequest, "This connection link is not valid: "+reason+". Start connecting again from your agent.")
		return
	}
	params, bad := s.checkAuthorizeParams(q)
	if bad != nil {
		s.redirectError(w, redirectURI, params.state, bad)
		return
	}
	ctx := r.Context()
	if err := s.db.Queries().DeleteExpiredOAuthRequests(ctx); err != nil {
		s.redirectFailure(w, r, redirectURI, params.state, err)
		return
	}
	id, err := s.db.Queries().CreateOAuthRequest(ctx, dbsqlc.CreateOAuthRequestParams{
		ClientID:       client.ID,
		RedirectUri:    redirectURI,
		State:          pgtype.Text{String: params.state, Valid: params.state != ""},
		CodeChallenge:  params.codeChallenge,
		Resource:       s.resource,
		ExpiresAt:      pgtype.Timestamptz{Time: time.Now().Add(requestTTL), Valid: true},
		MaxOutstanding: maxPendingRequests,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		s.redirectError(w, redirectURI, params.state, &oauthError{code: "temporarily_unavailable", description: "too many connection requests are waiting"})
		return
	}
	if err != nil {
		s.redirectFailure(w, r, redirectURI, params.state, err)
		return
	}
	noStore(w)
	w.Header().Set("Location", "/connect/"+id.String())
	w.WriteHeader(http.StatusFound)
}

const internalReason = "internal"

func (s *Server) resolveClient(r *http.Request, q url.Values) (dbsqlc.OauthClient, string, string) {
	clientID, ok := single(q, "client_id")
	redirect, okRedirect := single(q, "redirect_uri")
	if !ok || !okRedirect || clientID == "" {
		return dbsqlc.OauthClient{}, "", "client_id and redirect_uri must each be given once"
	}
	client, err := s.db.Queries().GetOAuthClient(r.Context(), clientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.OauthClient{}, "", "the client is not registered"
	}
	if err != nil {
		if !clientgone.Is(r.Context(), err) {
			s.logger.ErrorContext(r.Context(), "read oauth client", logattrs.Failure("oauth_client_read_failed", err)...)
		}
		return dbsqlc.OauthClient{}, "", internalReason
	}
	if redirect == "" && len(client.RedirectUris) == 1 {
		return client, client.RedirectUris[0], ""
	}
	matched, ok := matchRedirect(client.RedirectUris, redirect)
	if !ok {
		return dbsqlc.OauthClient{}, "", "the redirect URI is not registered for this client"
	}
	return client, matched, ""
}

func (s *Server) checkAuthorizeParams(q url.Values) (authorizeParams, *oauthError) {
	var p authorizeParams
	state, okState := single(q, "state")
	if okState && len(state) <= maxStateBytes {
		p.state = state
	}
	responseType, okType := single(q, "response_type")
	if !okType || responseType != "code" {
		return p, &oauthError{code: "unsupported_response_type", description: "response_type must be code"}
	}
	if !okState || len(state) > maxStateBytes {
		return p, invalidRequest("state must be given once and be at most 1024 bytes")
	}
	challenge, okChallenge := single(q, "code_challenge")
	method, okMethod := single(q, "code_challenge_method")
	if !okChallenge || !okMethod || !validChallenge(challenge) || method != "S256" {
		return p, invalidRequest("a PKCE code_challenge with code_challenge_method S256 is required")
	}
	p.codeChallenge = challenge
	for _, resource := range q["resource"] {
		if !canonicalEqual(resource, s.resource) {
			return p, errInvalidTarget
		}
	}
	return p, nil
}

func (s *Server) redirectFailure(w http.ResponseWriter, r *http.Request, redirectURI, state string, err error) {
	if !clientgone.Is(r.Context(), err) {
		s.logger.ErrorContext(r.Context(), "create oauth request", logattrs.Failure("oauth_request_failed", err)...)
	}
	s.redirectError(w, redirectURI, state, &oauthError{code: "server_error", description: "internal error"})
}

func (s *Server) redirectError(w http.ResponseWriter, redirectURI, state string, e *oauthError) {
	values := url.Values{"error": {e.code}, "error_description": {e.description}, "iss": {s.issuer}}
	if state != "" {
		values.Set("state", state)
	}
	noStore(w)
	w.Header().Set("Location", appendQuery(redirectURI, values))
	w.WriteHeader(http.StatusFound)
}

func refusePage(w http.ResponseWriter, status int, message string) {
	noStore(w)
	http.Error(w, message, status)
}
