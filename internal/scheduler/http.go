package scheduler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type jobJSON struct {
	Name           string  `json:"name"`
	State          State   `json:"state"`
	EverySeconds   int32   `json:"every_seconds"`
	NextRunAt      string  `json:"next_run_at"`
	LastStartedAt  *string `json:"last_started_at"`
	LastFinishedAt *string `json:"last_finished_at"`
	LastOutcome    *string `json:"last_outcome"`
	LastError      *string `json:"last_error"`
}

type jobListJSON struct {
	Jobs []jobJSON `json:"jobs"`
}

func optionalTime(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	out := httpapi.FormatTime(t.Time)
	return &out
}

func factsOf(leaseUntil, lastStartedAt, lastFinishedAt, nextRunAt pgtype.Timestamptz, now time.Time) Facts {
	return Facts{
		LeaseUntil: leaseUntil.Time, Leased: leaseUntil.Valid,
		LastStartedAt: lastStartedAt.Time, Started: lastStartedAt.Valid,
		LastFinishedAt: lastFinishedAt.Time, Finished: lastFinishedAt.Valid,
		NextRunAt: nextRunAt.Time, Now: now,
	}
}

func RowState(row dbsqlc.Job, now time.Time) State {
	return StateOf(factsOf(row.LeaseUntil, row.LastStartedAt, row.LastFinishedAt, row.NextRunAt, now))
}

func newJobJSON(row dbsqlc.ListJobsRow) jobJSON {
	return jobJSON{
		Name:           row.Name,
		State:          StateOf(factsOf(row.LeaseUntil, row.LastStartedAt, row.LastFinishedAt, row.NextRunAt, row.ReadAt.Time)),
		EverySeconds:   row.EverySeconds,
		NextRunAt:      httpapi.FormatTime(row.NextRunAt.Time),
		LastStartedAt:  optionalTime(row.LastStartedAt),
		LastFinishedAt: optionalTime(row.LastFinishedAt),
		LastOutcome:    httpapi.OptionalString(row.LastOutcome),
		LastError:      httpapi.OptionalString(row.LastError),
	}
}

type Handler struct {
	db     *db.DB
	logger *slog.Logger
}

func NewHandler(database *db.DB, logger *slog.Logger) *Handler {
	return &Handler{db: database, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/jobs", auth.RoleAdmin, h.listJobs)
}

func (h *Handler) listJobs(c *gin.Context) {
	rows, err := h.db.Queries().ListJobs(c.Request.Context())
	if err != nil {
		if clientgone.Handle(c, h.logger, "list scheduled jobs", err) {
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "list scheduled jobs", logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	body := jobListJSON{Jobs: make([]jobJSON, 0, len(rows))}
	for _, row := range rows {
		body.Jobs = append(body.Jobs, newJobJSON(row))
	}
	c.JSON(http.StatusOK, body)
}
