package agentsview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	kindBuiltin        = "builtin"
	kindGrant          = "grant"
	kindServiceAccount = "service_account"

	defaultRunLimit = 20
	maxRunLimit     = 50
	maxTitleRunes   = 120
)

var errAssigneeNotFound = errors.New("agent not found")

type linkJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type requesterJSON struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type runJSON struct {
	ID          string        `json:"id"`
	Source      string        `json:"source"`
	Title       string        `json:"title"`
	State       string        `json:"state"`
	ResultKind  string        `json:"result_kind"`
	FailureCode *string       `json:"failure_code"`
	CreatedAt   string        `json:"created_at"`
	StartedAt   *string       `json:"started_at"`
	FinishedAt  *string       `json:"finished_at"`
	CostUSD     *float64      `json:"cost_usd"`
	RequestedBy requesterJSON `json:"requested_by"`
	Openable    bool          `json:"openable"`
	Dashboard   *linkJSON     `json:"dashboard"`
	Observation *linkJSON     `json:"observation"`
}

type runPageJSON struct {
	Scope      string    `json:"scope"`
	Items      []runJSON `json:"items"`
	NextCursor *string   `json:"next_cursor"`
}

type runQuery struct {
	kind   string
	id     pgtype.UUID
	limit  int
	cursor *httpapi.TimeCursor
}

func (h *Handler) runs(c *gin.Context) {
	query, ok := parseRunQuery(c)
	if !ok {
		return
	}
	caller := httpapi.Caller(c)
	ctx := c.Request.Context()
	body, err := h.runPage(ctx, caller, query)
	switch {
	case errors.Is(err, errAssigneeNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "agent not found")
		return
	case err != nil:
		if clientgone.Handle(c, h.logger, "list agent runs", err) {
			return
		}
		h.logger.ErrorContext(ctx, "list agent runs", logattrs.Failure("internal", err, slog.String("actor_id", caller.ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	c.JSON(http.StatusOK, body)
}

func parseRunQuery(c *gin.Context) (runQuery, bool) {
	var q runQuery
	q.kind = c.Query("kind")
	rawID, hasID := c.GetQuery("id")
	switch q.kind {
	case kindBuiltin:
		if hasID {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "id is not allowed for the built-in agent")
			return q, false
		}
	case kindGrant, kindServiceAccount:
		id, err := db.ParseUUID(rawID)
		if !hasID || err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "id must be a uuid")
			return q, false
		}
		q.id = id
	default:
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "kind must be builtin, grant or service_account")
		return q, false
	}
	limit, ok := httpapi.LimitParam(c, defaultRunLimit, maxRunLimit)
	if !ok {
		return q, false
	}
	q.limit = limit
	cursor, ok := httpapi.DecodeTimeCursor(c.Query("cursor"))
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "cursor is not valid")
		return q, false
	}
	q.cursor = cursor
	return q, true
}

func (h *Handler) runPage(ctx context.Context, caller auth.Principal, query runQuery) (runPageJSON, error) {
	callerID, err := db.ParseUUID(caller.ID)
	if err != nil {
		return runPageJSON{}, fmt.Errorf("caller id: %w", err)
	}
	requestedBy, scope, err := h.runScope(ctx, caller, callerID, query)
	if err != nil {
		return runPageJSON{}, err
	}
	levels, err := folders.NewAccess(h.db).Levels(ctx, caller)
	if err != nil {
		return runPageJSON{}, fmt.Errorf("folder access: %w", err)
	}
	params := dbsqlc.ListAgentRunsParams{
		Folders:      levels.Visible(),
		AssignedKind: query.kind,
		AssignedID:   query.id,
		RequestedBy:  requestedBy,
		RowLimit:     httpapi.Int32(query.limit + 1),
	}
	if query.cursor != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: query.cursor.Time, Valid: true}
		params.CursorID = query.cursor.ID
	}
	rows, err := h.db.Queries().ListAgentRuns(ctx, params)
	if err != nil {
		return runPageJSON{}, fmt.Errorf("list agent runs: %w", err)
	}
	page := runPageJSON{Scope: scope, Items: make([]runJSON, 0, min(len(rows), query.limit))}
	if len(rows) > query.limit {
		rows = rows[:query.limit]
		last := rows[len(rows)-1]
		next := httpapi.EncodeTimeCursor(last.CreatedAt.Time, last.ID.String())
		page.NextCursor = &next
	}
	for _, row := range rows {
		page.Items = append(page.Items, newRunJSON(row, callerID))
	}
	return page, nil
}

func (h *Handler) runScope(ctx context.Context, caller auth.Principal, callerID pgtype.UUID, query runQuery) (pgtype.UUID, string, error) {
	q := h.db.Queries()
	switch query.kind {
	case kindGrant:
		owned, err := q.GrantOwnedBy(ctx, dbsqlc.GrantOwnedByParams{ID: query.id, UserID: callerID})
		if err != nil {
			return pgtype.UUID{}, "", fmt.Errorf("check grant owner: %w", err)
		}
		if !owned {
			return pgtype.UUID{}, "", errAssigneeNotFound
		}
		return pgtype.UUID{}, scopeYou, nil
	case kindServiceAccount:
		exists, err := q.ServiceAccountExists(ctx, query.id)
		if err != nil {
			return pgtype.UUID{}, "", fmt.Errorf("check service account: %w", err)
		}
		if !exists {
			return pgtype.UUID{}, "", errAssigneeNotFound
		}
	}
	if caller.Role.AtLeast(auth.RoleAdmin) {
		return pgtype.UUID{}, scopeWorkspace, nil
	}
	return callerID, scopeYou, nil
}

func newRunJSON(row dbsqlc.ListAgentRunsRow, callerID pgtype.UUID) runJSON {
	out := runJSON{
		ID:          row.ID.String(),
		Source:      row.Source,
		Title:       runTitle(row.Title.String, row.Prompt, row.Source),
		State:       row.State,
		ResultKind:  row.ResultKind,
		FailureCode: optionalText(row.FailureCode),
		CreatedAt:   httpapi.FormatTime(row.CreatedAt.Time),
		StartedAt:   optionalTime(row.StartedAt),
		FinishedAt:  optionalTime(row.FinishedAt),
		RequestedBy: requesterJSON{Kind: row.RequestedByKind, Name: row.RequestedByName},
		Openable:    row.RequestedByKind == "user" && row.RequestedByID == callerID,
	}
	if row.Priced {
		out.CostUSD = &row.CostUsd
	}
	if row.VisibleDashboardID.Valid {
		out.Dashboard = &linkJSON{ID: row.VisibleDashboardID.String(), Title: row.DashboardTitle.String}
	}
	if row.VisibleObservationID.Valid {
		out.Observation = &linkJSON{ID: row.VisibleObservationID.String(), Title: row.ObservationTitle.String}
	}
	return out
}

func runTitle(title, prompt, source string) string {
	if title != "" {
		return title
	}
	first, _, _ := strings.Cut(strings.TrimSpace(prompt), "\n")
	first = strings.TrimSpace(first)
	if first == "" {
		return source
	}
	if runes := []rune(first); len(runes) > maxTitleRunes {
		return string(runes[:maxTitleRunes])
	}
	return first
}

func optionalText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func optionalTime(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := httpapi.FormatTime(t.Time)
	return &s
}
