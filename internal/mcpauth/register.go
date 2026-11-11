package mcpauth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/origin"
)

var (
	registeredGrantTypes    = []string{"authorization_code", "refresh_token"}
	registeredResponseTypes = []string{"code"}
)

type registrationRequest struct {
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
	GrantTypes   []string `json:"grant_types"`
	ResponseType []string `json:"response_types"`
}

type registrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientName              *string  `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if !s.preflightOrMethod(w, r) {
		return
	}
	if !s.registerLimit.allow("") {
		tooManyRequests(w, s.registerLimit)
		return
	}
	req, bad := decodeRegistration(w, r)
	if bad != nil {
		writeOAuthError(w, bad)
		return
	}
	name := origin.CleanClient(req.ClientName)
	client, err := s.db.Queries().CreateOAuthClient(r.Context(), dbsqlc.CreateOAuthClientParams{
		Name:         pgtype.Text{String: name, Valid: name != ""},
		RedirectUris: req.RedirectURIs,
		MaxUnused:    maxUnusedClients,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		tooManyRequests(w, s.registerLimit)
		return
	}
	if err != nil {
		s.fail(w, r, "register oauth client", "oauth_register_failed", err)
		return
	}
	s.logger.InfoContext(r.Context(), "oauth client registered", slog.String("client_id", client.ID))
	body := registrationResponse{
		ClientID:                client.ID,
		ClientIDIssuedAt:        client.RegisteredAt.Time.Unix(),
		RedirectURIs:            client.RedirectUris,
		GrantTypes:              registeredGrantTypes,
		ResponseTypes:           registeredResponseTypes,
		TokenEndpointAuthMethod: "none",
	}
	if client.Name.Valid {
		body.ClientName = &client.Name.String
	}
	writeJSON(w, http.StatusCreated, body)
}

func decodeRegistration(w http.ResponseWriter, r *http.Request) (registrationRequest, *oauthError) {
	var req registrationRequest
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return req, &oauthError{http.StatusBadRequest, "invalid_client_metadata", "Content-Type must be application/json"}
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRegistrationBytes)).Decode(&req); err != nil {
		return req, &oauthError{http.StatusBadRequest, "invalid_client_metadata", "the body must be a JSON object of client metadata"}
	}
	if n := len(req.RedirectURIs); n < 1 || n > maxRedirectURIs {
		return req, &oauthError{http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris must hold 1 to 10 URIs"}
	}
	for _, uri := range req.RedirectURIs {
		if !validRedirect(uri) {
			return req, &oauthError{http.StatusBadRequest, "invalid_redirect_uri", "redirect URIs must be https, or http on localhost, 127.0.0.1 or [::1], with no fragment or user info"}
		}
	}
	for _, grant := range req.GrantTypes {
		if !slices.Contains(registeredGrantTypes, grant) {
			return req, &oauthError{http.StatusBadRequest, "invalid_client_metadata", "grant_types may only hold authorization_code and refresh_token"}
		}
	}
	if req.ResponseType != nil && !slices.Equal(req.ResponseType, registeredResponseTypes) {
		return req, &oauthError{http.StatusBadRequest, "invalid_client_metadata", "response_types must be [\"code\"]"}
	}
	return req, nil
}
