package mcpauth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const (
	defaultGrantLimit = 50
	maxGrantLimit     = 200
	reasonPerson      = "person"
	reasonAdmin       = "admin"
)

type grantClientJSON struct {
	ID           string  `json:"id"`
	Name         *string `json:"name"`
	RedirectHost string  `json:"redirect_host"`
	Local        bool    `json:"local"`
}

type grantPersonJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type grantJSON struct {
	ID         string          `json:"id"`
	Client     grantClientJSON `json:"client"`
	Person     grantPersonJSON `json:"person"`
	CreatedAt  string          `json:"created_at"`
	LastUsedAt *string         `json:"last_used_at"`
	ExpiresAt  string          `json:"expires_at"`
}

type grantListJSON struct {
	Grants     []grantJSON `json:"grants"`
	NextCursor *string     `json:"next_cursor"`
}

type grantPage struct {
	limit    int
	before   pgtype.Timestamptz
	beforeID pgtype.UUID
}

type grantFetcher func(ctx context.Context, page grantPage) ([]dbsqlc.ListOAuthGrantsRow, error)

func newGrantJSON(row dbsqlc.ListOAuthGrantsRow) grantJSON {
	var first string
	if len(row.ClientRedirectUris) > 0 {
		first = row.ClientRedirectUris[0]
	}
	out := grantJSON{
		ID: row.ID.String(),
		Client: grantClientJSON{
			ID:           row.ClientID,
			Name:         displayName(row.ClientName),
			RedirectHost: redirectHost(first),
			Local:        isLoopback(first),
		},
		Person:    grantPersonJSON{ID: row.UserID.String(), Name: row.UserName, Email: row.UserEmail},
		CreatedAt: httpapi.FormatTime(row.CreatedAt.Time),
		ExpiresAt: httpapi.FormatTime(row.ExpiresAt.Time),
	}
	if row.LastUsedAt.Valid {
		used := httpapi.FormatTime(row.LastUsedAt.Time)
		out.LastUsedAt = &used
	}
	return out
}

func (s *Server) listGrants(c *gin.Context, action string, fetch grantFetcher) {
	limit, ok := httpapi.LimitParam(c, defaultGrantLimit, maxGrantLimit)
	if !ok {
		return
	}
	after, ok := httpapi.DecodeTimeCursor(c.Query("cursor"))
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "cursor is not valid")
		return
	}
	page := grantPage{limit: limit + 1}
	if after != nil {
		page.before = pgtype.Timestamptz{Time: after.Time, Valid: true}
		page.beforeID = after.ID
	}
	rows, err := fetch(c.Request.Context(), page)
	if err != nil {
		s.abortInternal(c, action, err)
		return
	}
	body := grantListJSON{Grants: []grantJSON{}}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next := httpapi.EncodeTimeCursor(last.CreatedAt.Time, last.ID.String())
		body.NextCursor = &next
	}
	for _, row := range rows {
		body.Grants = append(body.Grants, newGrantJSON(row))
	}
	c.JSON(http.StatusOK, body)
}

func (s *Server) listMyGrants(c *gin.Context) {
	userID, err := db.ParseUUID(httpapi.Caller(c).ID)
	if err != nil {
		s.abortInternal(c, "list connected agents", err)
		return
	}
	s.listGrants(c, "list connected agents", func(ctx context.Context, page grantPage) ([]dbsqlc.ListOAuthGrantsRow, error) {
		rows, err := s.db.Queries().ListUserOAuthGrants(ctx, dbsqlc.ListUserOAuthGrantsParams{
			UserID:        userID,
			BeforeCreated: page.before,
			BeforeID:      page.beforeID,
			Lim:           httpapi.Int32(page.limit),
		})
		out := make([]dbsqlc.ListOAuthGrantsRow, len(rows))
		for i, row := range rows {
			out[i] = dbsqlc.ListOAuthGrantsRow(row)
		}
		return out, err
	})
}

func (s *Server) listAllGrants(c *gin.Context) {
	s.listGrants(c, "list all connected agents", func(ctx context.Context, page grantPage) ([]dbsqlc.ListOAuthGrantsRow, error) {
		return s.db.Queries().ListOAuthGrants(ctx, dbsqlc.ListOAuthGrantsParams{
			BeforeCreated: page.before,
			BeforeID:      page.beforeID,
			Lim:           httpapi.Int32(page.limit),
		})
	})
}

func (s *Server) disconnectMine(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	caller := httpapi.Caller(c)
	userID, err := db.ParseUUID(caller.ID)
	if err != nil {
		s.abortInternal(c, "disconnect agent", err)
		return
	}
	n, err := s.db.Queries().RevokeUserOAuthGrant(c.Request.Context(), dbsqlc.RevokeUserOAuthGrantParams{UserID: userID, ID: id})
	s.finishDisconnect(c, id, reasonPerson, n, err)
}

func (s *Server) disconnectAny(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	caller := httpapi.Caller(c)
	adminID, err := db.ParseUUID(caller.ID)
	if err != nil {
		s.abortInternal(c, "disconnect agent", err)
		return
	}
	n, err := s.db.Queries().RevokeOAuthGrant(c.Request.Context(), dbsqlc.RevokeOAuthGrantParams{Reason: reasonAdmin, By: adminID, ID: id})
	s.finishDisconnect(c, id, reasonAdmin, n, err)
}

func (s *Server) finishDisconnect(c *gin.Context, id pgtype.UUID, reason string, revoked int64, err error) {
	switch {
	case err != nil:
		s.abortInternal(c, "disconnect agent", err)
	case revoked == 0:
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "connected agent not found")
	default:
		s.logger.InfoContext(c.Request.Context(), "oauth grant revoked",
			slog.String("grant_id", id.String()),
			slog.String("reason", reason),
			slog.String("actor_id", httpapi.Caller(c).ID))
		c.Status(http.StatusNoContent)
	}
}
