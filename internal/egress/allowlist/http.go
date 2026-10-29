package allowlist

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxBodyBytes = 4 << 10

	managedBySettings = "settings"
	managedByConfig   = "config"
)

var (
	errTooMany = errors.New("too many outbound domains")
	errExists  = errors.New("outbound domain exists")
)

type entryJSON struct {
	Domain  string  `json:"domain"`
	AddedAt *string `json:"added_at"`
	AddedBy *string `json:"added_by"`
}

type listJSON struct {
	ManagedBy string      `json:"managed_by"`
	Domains   []entryJSON `json:"domains"`
}

func (s *Store) Register(api auth.Routes) {
	g := api.Group("/outbound-domains")
	g.GET("", auth.RoleViewer, auth.RequireUser, s.list)
	g.POST("", auth.RoleAdmin, auth.RequireUser, s.add)
	g.DELETE("/:domain", auth.RoleAdmin, auth.RequireUser, s.remove)
}

func (s *Store) refuseWhenLocked(c *gin.Context) bool {
	if !s.lock.Locked {
		return false
	}
	httpapi.AbortError(c, http.StatusConflict, "managed_by_config", "the outbound domains are set by SLASHOBSERVE_OUTBOUND_DOMAINS and cannot be changed here")
	return true
}

func (s *Store) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, s.logger, action, err) {
		return
	}
	s.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}

func (s *Store) list(c *gin.Context) {
	if s.lock.Locked {
		entries := make([]entryJSON, 0, len(s.lock.Domains))
		for _, p := range s.lock.Domains {
			entries = append(entries, entryJSON{Domain: p.String()})
		}
		slices.SortFunc(entries, func(a, b entryJSON) int { return strings.Compare(a.Domain, b.Domain) })
		c.JSON(http.StatusOK, listJSON{ManagedBy: managedByConfig, Domains: entries})
		return
	}
	rows, err := s.db.Queries().ListOutboundDomains(c.Request.Context())
	if err != nil {
		s.fail(c, "list outbound domains", err)
		return
	}
	body := listJSON{ManagedBy: managedBySettings, Domains: make([]entryJSON, 0, len(rows))}
	for _, r := range rows {
		at := httpapi.FormatTime(r.CreatedAt.Time)
		entry := entryJSON{Domain: r.Domain, AddedAt: &at}
		if r.HasAuthor {
			name := r.CreatedByName
			entry.AddedBy = &name
		}
		body.Domains = append(body.Domains, entry)
	}
	c.JSON(http.StatusOK, body)
}

func (s *Store) add(c *gin.Context) {
	if s.refuseWhenLocked(c) {
		return
	}
	var req struct {
		Domain string `json:"domain"`
	}
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	pattern, err := domains.ParsePattern(req.Domain)
	if err != nil {
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "invalid_domain", err.Error())
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	params := dbsqlc.InsertOutboundDomainParams{Domain: pattern.String(), CreatedByName: caller.Name}
	if params.CreatedByName == "" {
		params.CreatedByName = caller.Email
	}
	if id, err := db.ParseUUID(caller.ID); err == nil {
		params.CreatedByID = id
	}
	var row dbsqlc.InsertOutboundDomainRow
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockOutboundDomains(ctx); err != nil {
			return err
		}
		n, err := q.CountOutboundDomains(ctx)
		if err != nil {
			return err
		}
		if n >= config.MaxOutboundDomains {
			return errTooMany
		}
		row, err = q.InsertOutboundDomain(ctx, params)
		if errors.Is(err, pgx.ErrNoRows) {
			return errExists
		}
		return err
	})
	switch {
	case errors.Is(err, errTooMany):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "too_many_domains", "at most 100 outbound domains can be allowed")
		return
	case errors.Is(err, errExists):
		httpapi.AbortError(c, http.StatusConflict, "domain_exists", "that domain is already allowed")
		return
	case err != nil:
		s.fail(c, "allow outbound domain", err)
		return
	}
	s.logger.InfoContext(ctx, "allowed an outbound domain", slog.String("actor_id", caller.ID), slog.String("domain", row.Domain))
	at := httpapi.FormatTime(row.CreatedAt.Time)
	by := row.CreatedByName
	c.JSON(http.StatusCreated, entryJSON{Domain: row.Domain, AddedAt: &at, AddedBy: &by})
}

func (s *Store) remove(c *gin.Context) {
	if s.refuseWhenLocked(c) {
		return
	}
	pattern, err := domains.ParsePattern(c.Param("domain"))
	if err != nil {
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "invalid_domain", err.Error())
		return
	}
	ctx := c.Request.Context()
	n, err := s.db.Queries().DeleteOutboundDomain(ctx, pattern.String())
	if err != nil {
		s.fail(c, "remove outbound domain", err)
		return
	}
	if n == 0 {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "domain is not allowed")
		return
	}
	s.logger.InfoContext(ctx, "removed an outbound domain", slog.String("actor_id", httpapi.Caller(c).ID), slog.String("domain", pattern.String()))
	c.Status(http.StatusNoContent)
}
