package agentsview

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/presence"
)

const (
	scopeWorkspace = "workspace"
	scopeYou       = "you"
	sessionMessage = "the agents overview shows a person's own connected agents; service-account keys cannot read it"
	anonymousName  = "MCP"

	runtimeSidecar = "sidecar"
)

type Observe struct {
	Enabled  bool
	Provider string
	Model    string
}

type Handler struct {
	db      *db.DB
	observe Observe
	logger  *slog.Logger
}

type statsJSON struct {
	Runs                   int      `json:"runs"`
	Failed                 int      `json:"failed"`
	CostUSD                *float64 `json:"cost_usd"`
	MedianQueueWaitSeconds *float64 `json:"median_queue_wait_seconds"`
}

type observeJSON struct {
	Enabled   bool      `json:"enabled"`
	Provider  *string   `json:"provider"`
	Model     *string   `json:"model"`
	Runtime   string    `json:"runtime"`
	Scope     string    `json:"scope"`
	Last7Days statsJSON `json:"last_7_days"`
}

type connectedJSON struct {
	GrantID     string    `json:"grant_id"`
	ClientName  string    `json:"client_name"`
	Mode        string    `json:"mode"`
	Present     bool      `json:"present"`
	Presence    string    `json:"presence"`
	LastSeenAt  *string   `json:"last_seen_at"`
	ConnectedAt string    `json:"connected_at"`
	Writes7d    int       `json:"writes_7d"`
	Last7Days   statsJSON `json:"last_7_days"`
}

type serviceAccountJSON struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Present    bool       `json:"present"`
	Presence   string     `json:"presence"`
	LastSeenAt *string    `json:"last_seen_at"`
	Last7Days  *statsJSON `json:"last_7_days"`
}

type overviewJSON struct {
	ObserveAgent    observeJSON          `json:"observe_agent"`
	Connected       []connectedJSON      `json:"connected"`
	ServiceAccounts []serviceAccountJSON `json:"service_accounts"`
}

func NewHandler(database *db.DB, observe Observe, logger *slog.Logger) *Handler {
	return &Handler{db: database, observe: observe, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/agents", auth.RoleViewer, requireSession, h.list)
	api.GET("/agents/runs", auth.RoleViewer, requireSession, h.runs)
}

func requireSession(c *gin.Context) {
	if httpapi.Caller(c).Kind != auth.KindUser {
		httpapi.AbortError(c, http.StatusForbidden, "user_session_required", sessionMessage)
		return
	}
	c.Next()
}

func (h *Handler) list(c *gin.Context) {
	caller := httpapi.Caller(c)
	ctx := c.Request.Context()
	body, err := h.overview(ctx, caller)
	if err != nil {
		if clientgone.Handle(c, h.logger, "list agents", err) {
			return
		}
		h.logger.ErrorContext(ctx, "list agents", logattrs.Failure("internal", err, slog.String("actor_id", caller.ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) overview(ctx context.Context, caller auth.Principal) (overviewJSON, error) {
	userID, err := db.ParseUUID(caller.ID)
	if err != nil {
		return overviewJSON{}, fmt.Errorf("caller id: %w", err)
	}
	q := h.db.Queries()
	admin := caller.Role.AtLeast(auth.RoleAdmin)

	observeScope := dbsqlc.AgentRunStatsParams{AssignedKind: "builtin", RequestedBy: userID}
	out := overviewJSON{
		ObserveAgent: observeJSON{
			Enabled:  h.observe.Enabled,
			Provider: httpapi.OptionalString(h.observe.Provider),
			Model:    httpapi.OptionalString(h.observe.Model),
			Runtime:  runtimeSidecar,
			Scope:    scopeYou,
		},
		Connected:       []connectedJSON{},
		ServiceAccounts: []serviceAccountJSON{},
	}
	if admin {
		observeScope.RequestedBy = pgtype.UUID{}
		out.ObserveAgent.Scope = scopeWorkspace
	}
	if out.ObserveAgent.Last7Days, err = h.stats(ctx, observeScope); err != nil {
		return overviewJSON{}, err
	}

	grants, err := q.ConnectedGrantsOf(ctx, userID)
	if err != nil {
		return overviewJSON{}, fmt.Errorf("list connected grants: %w", err)
	}
	for _, g := range grants {
		stats, err := h.stats(ctx, dbsqlc.AgentRunStatsParams{AssignedKind: "grant", AssignedID: g.ID})
		if err != nil {
			return overviewJSON{}, err
		}
		state, seen := presenceOf(g.WorkerSeenAt, g.WorkerWorking, g.CheckedAt)
		out.Connected = append(out.Connected, connectedJSON{
			GrantID:     g.ID.String(),
			ClientName:  cmp.Or(actor.ClientName(g.ClientName.String), anonymousName),
			Mode:        string(grantmode.Parse(g.Mode)),
			Present:     state != presence.Offline,
			Presence:    string(state),
			LastSeenAt:  seen,
			ConnectedAt: httpapi.FormatTime(g.CreatedAt.Time),
			Writes7d:    int(g.Writes7d),
			Last7Days:   stats,
		})
	}

	accounts, err := q.RunTakingServiceAccounts(ctx)
	if err != nil {
		return overviewJSON{}, fmt.Errorf("list run-taking service accounts: %w", err)
	}
	for _, a := range accounts {
		state, seen := presenceOf(a.WorkerSeenAt, a.WorkerWorking, a.CheckedAt)
		item := serviceAccountJSON{ID: a.ID.String(), Name: a.Name, Present: state != presence.Offline, Presence: string(state), LastSeenAt: seen}
		if admin {
			stats, err := h.stats(ctx, dbsqlc.AgentRunStatsParams{AssignedKind: "service_account", AssignedID: a.ID})
			if err != nil {
				return overviewJSON{}, err
			}
			item.Last7Days = &stats
		}
		out.ServiceAccounts = append(out.ServiceAccounts, item)
	}
	return out, nil
}

func (h *Handler) stats(ctx context.Context, params dbsqlc.AgentRunStatsParams) (statsJSON, error) {
	row, err := h.db.Queries().AgentRunStats(ctx, params)
	if err != nil {
		return statsJSON{}, fmt.Errorf("read run stats: %w", err)
	}
	out := statsJSON{Runs: int(row.Runs), Failed: int(row.Failed)}
	if row.Unpriced == 0 {
		out.CostUSD = &row.CostUsd
	}
	if row.Started > 0 {
		out.MedianQueueWaitSeconds = &row.MedianQueueWaitSeconds
	}
	return out, nil
}

func presenceOf(seenAt pgtype.Timestamptz, working bool, checkedAt pgtype.Timestamptz) (presence.State, *string) {
	state := presence.Of(seenAt.Time, seenAt.Valid, working, checkedAt.Time)
	if !seenAt.Valid {
		return state, nil
	}
	seen := httpapi.FormatTime(seenAt.Time)
	return state, &seen
}
