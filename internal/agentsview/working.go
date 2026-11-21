package agentsview

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

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
	defaultWorkingLimit = 20
	maxWorkingLimit     = 50
	builtinAgentName    = "Observe Agent"
)

type workingAgentJSON struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type workingRunJSON struct {
	ID            string           `json:"id"`
	Intent        string           `json:"intent"`
	Investigation bool             `json:"investigation"`
	State         string           `json:"state"`
	CreatedAt     string           `json:"created_at"`
	StartedAt     *string          `json:"started_at"`
	Mine          bool             `json:"mine"`
	Title         *string          `json:"title"`
	Agent         workingAgentJSON `json:"agent"`
	RequestedBy   requesterJSON    `json:"requested_by"`
	Dashboard     *linkJSON        `json:"dashboard"`
	Observation   *linkJSON        `json:"observation"`
}

type workingPageJSON struct {
	Items     []workingRunJSON `json:"items"`
	Truncated bool             `json:"truncated"`
}

func (h *Handler) working(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultWorkingLimit, maxWorkingLimit)
	if !ok {
		return
	}
	caller := httpapi.Caller(c)
	ctx := c.Request.Context()
	body, err := h.workingPage(ctx, caller, limit)
	if err != nil {
		if clientgone.Handle(c, h.logger, "list working runs", err) {
			return
		}
		h.logger.ErrorContext(ctx, "list working runs", logattrs.Failure("internal", err, slog.String("actor_id", caller.ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) workingPage(ctx context.Context, caller auth.Principal, limit int) (workingPageJSON, error) {
	callerID, err := db.ParseUUID(caller.ID)
	if err != nil {
		return workingPageJSON{}, fmt.Errorf("caller id: %w", err)
	}
	levels, err := folders.NewAccess(h.db).Levels(ctx, caller)
	if err != nil {
		return workingPageJSON{}, fmt.Errorf("folder access: %w", err)
	}
	rows, err := h.db.Queries().ListWorkingRuns(ctx, dbsqlc.ListWorkingRunsParams{
		Folders:     levels.Visible(),
		CallerKind:  string(caller.Kind),
		CallerID:    callerID,
		CallerAdmin: caller.Role.AtLeast(auth.RoleAdmin),
		RowLimit:    httpapi.Int32(limit + 1),
	})
	if err != nil {
		return workingPageJSON{}, fmt.Errorf("list working runs: %w", err)
	}
	page := workingPageJSON{Items: make([]workingRunJSON, 0, min(len(rows), limit))}
	if len(rows) > limit {
		rows = rows[:limit]
		page.Truncated = true
	}
	names := map[pgtype.UUID]string{}
	for _, row := range rows {
		agent, err := h.workingAgent(ctx, row, names)
		if err != nil {
			return workingPageJSON{}, err
		}
		page.Items = append(page.Items, newWorkingRunJSON(row, caller.Kind, callerID, agent))
	}
	return page, nil
}

func (h *Handler) workingAgent(ctx context.Context, row dbsqlc.ListWorkingRunsRow, names map[pgtype.UUID]string) (workingAgentJSON, error) {
	if row.AssignedKind == kindBuiltin {
		return workingAgentJSON{Kind: kindBuiltin, Name: builtinAgentName}, nil
	}
	name, cached := names[row.AssignedID]
	if !cached {
		var err error
		name, err = h.db.Queries().GetAssigneeName(ctx, dbsqlc.GetAssigneeNameParams{Kind: row.AssignedKind, ID: row.AssignedID})
		if err != nil {
			return workingAgentJSON{}, fmt.Errorf("assignee name: %w", err)
		}
		if name == "" {
			name = anonymousName
		}
		names[row.AssignedID] = name
	}
	return workingAgentJSON{Kind: row.AssignedKind, Name: name}, nil
}

func newWorkingRunJSON(row dbsqlc.ListWorkingRunsRow, callerKind auth.PrincipalKind, callerID pgtype.UUID, agent workingAgentJSON) workingRunJSON {
	mine := row.RequestedByKind == string(callerKind) && row.RequestedByID == callerID
	out := workingRunJSON{
		ID:            row.ID.String(),
		Intent:        row.Intent,
		Investigation: row.Investigation,
		State:         row.State,
		CreatedAt:     httpapi.FormatTime(row.CreatedAt.Time),
		StartedAt:     optionalTime(row.StartedAt),
		Mine:          mine,
		Agent:         agent,
		RequestedBy:   requesterJSON{Kind: row.RequestedByKind, Name: row.RequestedByName},
	}
	if mine {
		title := runTitle(row.Title.String, row.Prompt, out.Intent)
		out.Title = &title
	}
	if row.VisibleDashboardID.Valid {
		out.Dashboard = &linkJSON{ID: row.VisibleDashboardID.String(), Title: row.DashboardTitle.String}
	}
	if row.VisibleObservationID.Valid {
		out.Observation = &linkJSON{ID: row.VisibleObservationID.String(), Title: row.ObservationTitle.String}
	}
	return out
}
