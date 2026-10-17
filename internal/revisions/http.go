package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
)

const (
	maxRequestBytes = 64 << 10
	defaultLimit    = 50
	maxLimit        = 200

	statusClientClosedRequest = 499
)

type createRequest struct {
	Prompt      *string `json:"prompt"`
	BaseVersion *int    `json:"base_version"`
}

type authorJSON struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type revisionJSON struct {
	ID            string     `json:"id"`
	DashboardID   string     `json:"dashboard_id"`
	Source        Source     `json:"source"`
	CommentID     *string    `json:"comment_id"`
	PanelID       *string    `json:"panel_id"`
	Prompt        string     `json:"prompt"`
	BaseVersion   int        `json:"base_version"`
	Status        Status     `json:"status"`
	Attempts      int        `json:"attempts"`
	ResultVersion *int       `json:"result_version"`
	Error         *string    `json:"error"`
	RequestedBy   authorJSON `json:"requested_by"`
	CreatedAt     string     `json:"created_at"`
	StartedAt     *string    `json:"started_at"`
	FinishedAt    *string    `json:"finished_at"`
}

type envelopeJSON struct {
	Revision revisionJSON `json:"revision"`
}

type listJSON struct {
	Revisions  []revisionJSON `json:"revisions"`
	NextCursor *string        `json:"next_cursor"`
}

func (s *Service) Register(api *gin.RouterGroup) {
	g := api.Group("/dashboards/:id/revisions")
	g.POST("", s.createRevision)
	g.GET("", s.listRevisions)
	g.GET("/:revision_id", s.getRevision)
}

func (s *Service) createRevision(c *gin.Context) {
	dashboardID, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	var req createRequest
	if !decodeBody(c, &req) {
		return
	}
	if req.Prompt == nil || req.BaseVersion == nil {
		abortError(c, http.StatusBadRequest, "invalid_request", "prompt and base_version are required")
		return
	}
	prompt, ok := CleanPrompt(*req.Prompt)
	if !ok {
		abortError(c, http.StatusBadRequest, "invalid_request", fmt.Sprintf("prompt must be 1 to %d characters without control or bidirectional override characters", MaxPromptRunes))
		return
	}
	if *req.BaseVersion < 1 || *req.BaseVersion > math.MaxInt32 {
		abortError(c, http.StatusBadRequest, "invalid_request", "base_version must be a positive 32-bit integer")
		return
	}
	if !s.Available() {
		abortError(c, http.StatusServiceUnavailable, "agent_unavailable", "no agent is configured")
		return
	}
	ctx := c.Request.Context()
	latest, err := s.db.Queries().GetDashboardLatestVersion(ctx, dashboardID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		abortError(c, http.StatusNotFound, "not_found", "dashboard not found")
		return
	case err != nil:
		s.fail(c, "read latest dashboard version", err)
		return
	case *req.BaseVersion > int(latest):
		abortError(c, http.StatusNotFound, "not_found", "dashboard version not found")
		return
	}
	rev, err := s.Submit(ctx, EnqueueParams{
		DashboardID: dashboardID,
		Source:      SourcePrompt,
		Prompt:      prompt,
		BaseVersion: *req.BaseVersion,
		By:          caller(c),
	})
	if err != nil {
		s.fail(c, "enqueue revision", err)
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+rev.DashboardID+"/revisions/"+rev.ID)
	c.JSON(http.StatusAccepted, envelopeJSON{Revision: newRevisionJSON(rev)})
}

func (s *Service) listRevisions(c *gin.Context) {
	dashboardID, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	limit, ok := limitParam(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if _, err := s.db.Queries().GetDashboardLatestVersion(ctx, dashboardID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			abortError(c, http.StatusNotFound, "not_found", "dashboard not found")
			return
		}
		s.fail(c, "read latest dashboard version", err)
		return
	}
	items, next, err := s.List(ctx, dashboardID, limit, c.Query("cursor"))
	if err != nil {
		s.fail(c, "list revisions", err)
		return
	}
	body := listJSON{Revisions: make([]revisionJSON, len(items))}
	for i, item := range items {
		body.Revisions[i] = newRevisionJSON(item)
	}
	if next != "" {
		body.NextCursor = &next
	}
	c.JSON(http.StatusOK, body)
}

func (s *Service) getRevision(c *gin.Context) {
	dashboardID, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	id, ok := uuidParam(c, "revision_id")
	if !ok {
		return
	}
	rev, err := s.Get(c.Request.Context(), dashboardID, id)
	if err != nil {
		s.fail(c, "get revision", err)
		return
	}
	c.JSON(http.StatusOK, envelopeJSON{Revision: newRevisionJSON(rev)})
}

func (s *Service) fail(c *gin.Context, action string, err error) {
	ctx := c.Request.Context()
	switch {
	case errors.Is(err, context.Canceled):
		s.logger.DebugContext(ctx, action+": client went away")
		c.AbortWithStatus(statusClientClosedRequest)
	case errors.Is(err, ErrNotFound):
		abortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrTooManyRevisions):
		abortError(c, http.StatusTooManyRequests, "too_many_revisions", err.Error())
	case errors.Is(err, ErrUnavailable):
		abortError(c, http.StatusServiceUnavailable, "agent_unavailable", err.Error())
	case errors.Is(err, ErrInvalidCursor):
		abortError(c, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		s.logger.ErrorContext(ctx, action, slog.Any("error", err))
		abortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}

func abortError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message, "code": code})
}

func decodeBody(c *gin.Context, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes))
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil {
		_, err = dec.Token()
		if errors.Is(err, io.EOF) {
			return true
		}
		if err == nil {
			abortError(c, http.StatusBadRequest, "invalid_request", "request body must be a single JSON object")
			return false
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		abortError(c, http.StatusRequestEntityTooLarge, "too_large", "request body is too large")
		return false
	}
	abortError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON with known fields")
	return false
}

func caller(c *gin.Context) auth.Principal {
	p, _ := auth.FromContext(c.Request.Context())
	return p
}

func uuidParam(c *gin.Context, name string) (pgtype.UUID, bool) {
	id, err := db.ParseUUID(c.Param(name))
	if err != nil {
		abortError(c, http.StatusBadRequest, "invalid_request", name+" must be a UUID")
		return pgtype.UUID{}, false
	}
	return id, true
}

func limitParam(c *gin.Context) (int, bool) {
	raw, present := c.GetQuery("limit")
	if !present {
		return defaultLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxLimit || strconv.Itoa(n) != raw {
		abortError(c, http.StatusBadRequest, "invalid_request", "limit must be an integer from 1 to "+strconv.Itoa(maxLimit))
		return 0, false
	}
	return n, true
}

func newRevisionJSON(r Revision) revisionJSON {
	out := revisionJSON{
		ID:            r.ID,
		DashboardID:   r.DashboardID,
		Source:        r.Source,
		CommentID:     optionalString(r.CommentID),
		PanelID:       optionalString(r.PanelID),
		Prompt:        r.Prompt,
		BaseVersion:   r.BaseVersion,
		Status:        r.Status,
		Attempts:      r.Attempts,
		ResultVersion: r.ResultVersion,
		Error:         optionalString(r.Error),
		RequestedBy:   authorJSON{Kind: string(r.RequestedBy.Kind), ID: r.RequestedBy.ID, Name: r.RequestedBy.Name, Email: r.RequestedBy.Email},
		CreatedAt:     formatTime(r.CreatedAt),
		StartedAt:     formatOptionalTime(r.StartedAt),
		FinishedAt:    formatOptionalTime(r.FinishedAt),
	}
	return out
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func formatOptionalTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := formatTime(*t)
	return &s
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
