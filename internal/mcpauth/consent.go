package mcpauth

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

const tooManyGrantsMessage = "You have 20 connected agents. Disconnect one in Settings, then try again."

var errTooManyGrants = errors.New("too many connected agents")

type clientJSON struct {
	ID           string  `json:"id"`
	Name         *string `json:"name"`
	RegisteredAt string  `json:"registered_at"`
	RedirectHost string  `json:"redirect_host"`
	Local        bool    `json:"local"`
}

type personJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type requestJSON struct {
	ID            string     `json:"id"`
	Client        clientJSON `json:"client"`
	Person        personJSON `json:"person"`
	ReplacesGrant bool       `json:"replaces_grant"`
	ExpiresAt     string     `json:"expires_at"`
}

type redirectJSON struct {
	RedirectTo string `json:"redirect_to"`
}

func (s *Server) Register(api auth.Routes) {
	api.GET("/oauth/requests/:id", auth.RoleViewer, auth.RequireUser, s.getRequest)
	api.POST("/oauth/requests/:id/approve", auth.RoleViewer, auth.RequireUser, s.approveRequest)
	api.POST("/oauth/requests/:id/deny", auth.RoleViewer, auth.RequireUser, s.denyRequest)
}

func requestID(c *gin.Context) (pgtype.UUID, bool) {
	id, err := db.ParseUUID(c.Param("id"))
	if err != nil {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "connection request not found")
		return pgtype.UUID{}, false
	}
	return id, true
}

func (s *Server) getRequest(c *gin.Context) {
	id, ok := requestID(c)
	if !ok {
		return
	}
	caller := httpapi.Caller(c)
	userID, err := db.ParseUUID(caller.ID)
	if err != nil {
		s.abortInternal(c, "read connection request", err)
		return
	}
	row, err := s.db.Queries().GetOAuthRequest(c.Request.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "connection request not found")
		return
	}
	if err != nil {
		s.abortInternal(c, "read connection request", err)
		return
	}
	replaces, err := s.db.Queries().HasLiveOAuthGrant(c.Request.Context(), dbsqlc.HasLiveOAuthGrantParams{ClientID: row.ClientID, UserID: userID})
	if err != nil {
		s.abortInternal(c, "read connection request", err)
		return
	}
	c.JSON(http.StatusOK, requestJSON{
		ID: id.String(),
		Client: clientJSON{
			ID:           row.ClientID,
			Name:         displayName(row.ClientName),
			RegisteredAt: httpapi.FormatTime(row.ClientRegisteredAt.Time),
			RedirectHost: redirectHost(row.RedirectUri),
			Local:        isLoopback(row.RedirectUri),
		},
		Person:        personJSON{ID: caller.ID, Name: caller.Name, Email: caller.Email, Role: string(caller.Role)},
		ReplacesGrant: replaces,
		ExpiresAt:     httpapi.FormatTime(row.ExpiresAt.Time),
	})
}

func displayName(stored pgtype.Text) *string {
	if !stored.Valid {
		return nil
	}
	name := actor.ClientName(stored.String)
	return &name
}

func (s *Server) approveRequest(c *gin.Context) {
	id, ok := requestID(c)
	if !ok {
		return
	}
	caller := httpapi.Caller(c)
	userID, err := db.ParseUUID(caller.ID)
	if err != nil {
		s.abortInternal(c, "approve connection request", err)
		return
	}
	var redirectTo, clientID string
	err = s.db.InTx(c.Request.Context(), func(q *dbsqlc.Queries) error {
		req, err := q.ConsumeOAuthRequest(c.Request.Context(), id)
		if err != nil {
			return err
		}
		clientID = req.ClientID
		live, err := q.CountLiveOAuthGrants(c.Request.Context(), userID)
		if err != nil {
			return err
		}
		replaces, err := q.HasLiveOAuthGrant(c.Request.Context(), dbsqlc.HasLiveOAuthGrantParams{ClientID: req.ClientID, UserID: userID})
		if err != nil {
			return err
		}
		if live >= maxLiveGrantsPerPerson && !replaces {
			return errTooManyGrants
		}
		code, hash, err := opaquetoken.New(codePrefix)
		if err != nil {
			return err
		}
		err = q.CreateOAuthCode(c.Request.Context(), dbsqlc.CreateOAuthCodeParams{
			CodeHash:      hash,
			ClientID:      req.ClientID,
			UserID:        userID,
			RedirectUri:   req.RedirectUri,
			CodeChallenge: req.CodeChallenge,
			Resource:      req.Resource,
			ExpiresAt:     pgtype.Timestamptz{Time: time.Now().Add(codeTTL), Valid: true},
		})
		if err != nil {
			return err
		}
		values := url.Values{"code": {code}, "iss": {s.issuer}}
		if req.State.Valid {
			values.Set("state", req.State.String)
		}
		redirectTo = appendQuery(req.RedirectUri, values)
		return nil
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "connection request not found")
	case errors.Is(err, errTooManyGrants):
		httpapi.AbortError(c, http.StatusConflict, "too_many_grants", tooManyGrantsMessage)
	case err != nil:
		s.abortInternal(c, "approve connection request", err)
	default:
		s.logger.InfoContext(c.Request.Context(), "oauth request approved", slog.String("user_id", caller.ID), slog.String("client_id", clientID))
		c.JSON(http.StatusOK, redirectJSON{RedirectTo: redirectTo})
	}
}

func (s *Server) denyRequest(c *gin.Context) {
	id, ok := requestID(c)
	if !ok {
		return
	}
	req, err := s.db.Queries().ConsumeOAuthRequest(c.Request.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "connection request not found")
		return
	}
	if err != nil {
		s.abortInternal(c, "deny connection request", err)
		return
	}
	values := url.Values{"error": {"access_denied"}, "iss": {s.issuer}}
	if req.State.Valid {
		values.Set("state", req.State.String)
	}
	c.JSON(http.StatusOK, redirectJSON{RedirectTo: appendQuery(req.RedirectUri, values)})
}

func (s *Server) abortInternal(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, s.logger, action, err) {
		return
	}
	s.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}
