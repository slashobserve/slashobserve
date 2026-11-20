package users

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const (
	maxTeams          = 200
	maxTeamNameLength = 100
	maxTeamBodyBytes  = 1 << 10
	foreignKeyCode    = "23503"
)

type teamMemberJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type teamJSON struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	CreatedBy string           `json:"created_by"`
	CreatedAt string           `json:"created_at"`
	Members   []teamMemberJSON `json:"members"`
}

func (h *Handler) registerTeams(api auth.Routes) {
	g := api.Group("/teams")
	g.GET("", auth.RoleAdmin, h.listTeams)
	g.POST("", auth.RoleAdmin, h.createTeam)
	g.DELETE("/:id", auth.RoleAdmin, h.deleteTeam)
	g.PUT("/:id/members/:user_id", auth.RoleAdmin, h.addTeamMember)
	g.DELETE("/:id/members/:user_id", auth.RoleAdmin, h.removeTeamMember)
}

func teamNameProblem(name string) string {
	if n := utf8.RuneCountInString(name); n < 1 || n > maxTeamNameLength {
		return "name must be 1 to 100 characters"
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "name must not contain control characters"
	}
	return ""
}

func (h *Handler) teams(c *gin.Context, q *dbsqlc.Queries, rows []dbsqlc.Team) ([]teamJSON, bool) {
	ids := make([]pgtype.UUID, len(rows))
	byID := make(map[pgtype.UUID]int, len(rows))
	out := make([]teamJSON, len(rows))
	for i, t := range rows {
		ids[i] = t.ID
		byID[t.ID] = i
		out[i] = teamJSON{ID: t.ID.String(), Name: t.Name, CreatedBy: t.CreatedByName, CreatedAt: httpapi.FormatTime(t.CreatedAt.Time), Members: []teamMemberJSON{}}
	}
	members, err := q.ListTeamMembers(c.Request.Context(), ids)
	if err != nil {
		h.fail(c, "list team members", err)
		return nil, false
	}
	for _, m := range members {
		i := byID[m.TeamID]
		out[i].Members = append(out[i].Members, teamMemberJSON{ID: m.ID.String(), Name: m.Name, Email: m.Email})
	}
	return out, true
}

func (h *Handler) team(c *gin.Context, id pgtype.UUID) (teamJSON, bool) {
	q := h.db.Queries()
	row, err := q.GetTeam(c.Request.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "team not found")
		return teamJSON{}, false
	}
	if err != nil {
		h.fail(c, "get team", err, slog.String("team_id", id.String()))
		return teamJSON{}, false
	}
	out, ok := h.teams(c, q, []dbsqlc.Team{row})
	if !ok {
		return teamJSON{}, false
	}
	return out[0], true
}

func (h *Handler) listTeams(c *gin.Context) {
	q := h.db.Queries()
	rows, err := q.ListTeams(c.Request.Context())
	if err != nil {
		h.fail(c, "list teams", err)
		return
	}
	out, ok := h.teams(c, q, rows)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"teams": out})
}

func (h *Handler) createTeam(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if !httpapi.DecodeBody(c, &req, maxTeamBodyBytes) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if problem := teamNameProblem(name); problem != "" {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", problem)
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	var row dbsqlc.Team
	full, exists := false, false
	err := h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		n, err := q.CountTeams(ctx)
		if err != nil {
			return err
		}
		if n >= maxTeams {
			full = true
			return nil
		}
		row, err = q.CreateTeam(ctx, dbsqlc.CreateTeamParams{Name: name, CreatedByName: caller.Name})
		if errors.Is(err, pgx.ErrNoRows) {
			exists = true
			return nil
		}
		if err != nil {
			return err
		}
		return recordTeam(ctx, q, "team.create", row.ID, row.Name)
	})
	switch {
	case err != nil:
		h.fail(c, "create team", err)
		return
	case full:
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "too_many_teams", "200 teams already exist; delete one first")
		return
	case exists:
		httpapi.AbortError(c, http.StatusConflict, "team_exists", "a team with this name already exists")
		return
	}
	h.logger.InfoContext(ctx, "created a team", slog.String("actor_id", caller.ID), slog.String("team_id", row.ID.String()))
	c.JSON(http.StatusCreated, teamJSON{ID: row.ID.String(), Name: row.Name, CreatedBy: row.CreatedByName, CreatedAt: httpapi.FormatTime(row.CreatedAt.Time), Members: []teamMemberJSON{}})
}

func (h *Handler) deleteTeam(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	n, err := h.deleteTeamRecorded(c.Request.Context(), id)
	if err != nil {
		h.fail(c, "delete team", err, slog.String("team_id", id.String()))
		return
	}
	if n == 0 {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "team not found")
		return
	}
	h.logger.InfoContext(c.Request.Context(), "deleted a team", slog.String("actor_id", httpapi.Caller(c).ID), slog.String("team_id", id.String()))
	c.Status(http.StatusNoContent)
}

func (h *Handler) addTeamMember(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	userID, ok := httpapi.UUIDParam(c, "user_id")
	if !ok {
		return
	}
	err := h.changeMember(c.Request.Context(), "team.member_add", id, userID, func(q *dbsqlc.Queries) (int64, error) {
		return q.AddTeamMember(c.Request.Context(), dbsqlc.AddTeamMemberParams{TeamID: id, UserID: userID})
	})
	if missing := missingReference(err); missing != "" {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", missing)
		return
	}
	if err != nil {
		h.fail(c, "add team member", err, slog.String("team_id", id.String()), slog.String("user_id", userID.String()))
		return
	}
	h.memberChanged(c, "added a team member", id, userID)
}

func (h *Handler) removeTeamMember(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	userID, ok := httpapi.UUIDParam(c, "user_id")
	if !ok {
		return
	}
	err := h.changeMember(c.Request.Context(), "team.member_remove", id, userID, func(q *dbsqlc.Queries) (int64, error) {
		return q.RemoveTeamMember(c.Request.Context(), dbsqlc.RemoveTeamMemberParams{TeamID: id, UserID: userID})
	})
	if err != nil {
		h.fail(c, "remove team member", err, slog.String("team_id", id.String()), slog.String("user_id", userID.String()))
		return
	}
	h.memberChanged(c, "removed a team member", id, userID)
}

func (h *Handler) deleteTeamRecorded(ctx context.Context, id pgtype.UUID) (int64, error) {
	var n int64
	err := h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		team, err := q.GetTeam(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		n, err = q.DeleteTeam(ctx, id)
		if err != nil || n == 0 {
			return err
		}
		return recordTeam(ctx, q, "team.delete", id, team.Name)
	})
	return n, err
}

func (h *Handler) changeMember(ctx context.Context, action audit.Action, teamID, userID pgtype.UUID, change func(*dbsqlc.Queries) (int64, error)) error {
	return h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		n, err := change(q)
		if err != nil || n == 0 {
			return err
		}
		return recordTeamMember(ctx, q, action, teamID, userID)
	})
}

func (h *Handler) memberChanged(c *gin.Context, msg string, id, userID pgtype.UUID) {
	team, ok := h.team(c, id)
	if !ok {
		return
	}
	h.logger.InfoContext(c.Request.Context(), msg, slog.String("actor_id", httpapi.Caller(c).ID), slog.String("team_id", id.String()), slog.String("user_id", userID.String()))
	c.JSON(http.StatusOK, team)
}

func missingReference(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != foreignKeyCode {
		return ""
	}
	if pgErr.ConstraintName == "team_members_team_id_fkey" {
		return "team not found"
	}
	return "user not found"
}
