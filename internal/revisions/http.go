package revisions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxRequestBytes = 64 << 10
	defaultLimit    = 50
	maxLimit        = 200
)

type createRequest struct {
	Prompt      *string `json:"prompt"`
	BaseVersion *int    `json:"base_version"`
	PanelID     *string `json:"panel_id"`
	CommentID   *string `json:"comment_id"`
}

type authorJSON struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type revisionJSON struct {
	ID             string     `json:"id"`
	DashboardID    string     `json:"dashboard_id"`
	Intent         Intent     `json:"intent"`
	Investigation  bool       `json:"investigation"`
	CommentID      *string    `json:"comment_id"`
	PanelID        *string    `json:"panel_id"`
	Prompt         string     `json:"prompt"`
	BaseVersion    *int       `json:"base_version"`
	Status         Status     `json:"status"`
	State          State      `json:"state"`
	ResultKind     ResultKind `json:"result_kind"`
	Attempts       int        `json:"attempts"`
	AllowedActions []string   `json:"allowed_actions"`
	ResultVersion  *int       `json:"result_version"`
	Error          *string    `json:"error"`
	RequestedBy    authorJSON `json:"requested_by"`
	Origin         *string    `json:"origin"`
	OriginClient   *string    `json:"origin_client"`
	CreatedAt      string     `json:"created_at"`
	StartedAt      *string    `json:"started_at"`
	FinishedAt     *string    `json:"finished_at"`

	FailureCode  *string            `json:"failure_code"`
	Build        *buildProgressJSON `json:"build"`
	BuildSavedAt *string            `json:"build_saved_at"`
	BuildState   BuildState         `json:"build_state"`
	ContinuesID  *string            `json:"continues_id"`
	ContinuedBy  *string            `json:"continued_by"`
	RetriedBy    *string            `json:"retried_by"`
	ContinueNote *string            `json:"continue_note"`
	Redacted     bool               `json:"redacted"`
}

type envelopeJSON struct {
	Revision revisionJSON `json:"revision"`
}

type listJSON struct {
	Revisions  []revisionJSON `json:"revisions"`
	NextCursor *string        `json:"next_cursor"`
}

func (s *Service) Register(api auth.Routes) {
	g := api.Group("/dashboards/:id/revisions")
	g.POST("", auth.RoleViewer, s.access.RequireDashboard(folders.NeedComment), s.createRevision)
	g.GET("", auth.RoleViewer, s.access.RequireDashboard(folders.NeedView), s.listRevisions)
	g.GET("/:revision_id", auth.RoleViewer, s.access.RequireDashboard(folders.NeedView), s.getRevision)
	api.POST("/dashboards/:id/suggestions", auth.RoleViewer, s.access.RequireDashboard(folders.NeedEdit), s.createSuggestion)
	api.GET("/dashboards/:id/suggestions/:suggestion_id", auth.RoleViewer, s.access.RequireDashboard(folders.NeedView), s.getSuggestion)
}

func (s *Service) createRevision(c *gin.Context) {
	dashboardID, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req createRequest
	if !httpapi.DecodeBody(c, &req, maxRequestBytes) {
		return
	}
	if req.Prompt == nil || req.BaseVersion == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "prompt and base_version are required")
		return
	}
	prompt, err := CheckPrompt("prompt", *req.Prompt)
	if err != nil {
		httpapi.AbortText(c, http.StatusBadRequest, "invalid_request", err)
		return
	}
	if *req.BaseVersion < 1 || *req.BaseVersion > math.MaxInt32 {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "base_version must be a positive 32-bit integer")
		return
	}
	panelID := ""
	if req.PanelID != nil {
		panelID = *req.PanelID
		if err := ValidPanelID(panelID); err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	commentID, ok := threadCommentID(c, req)
	if !ok {
		return
	}
	if !s.Available() {
		httpapi.AbortError(c, http.StatusServiceUnavailable, "agent_unavailable", "no agent is configured")
		return
	}
	var row dbsqlc.AgentRequest
	if commentID.Valid {
		row, err = s.SubmitForThread(c.Request.Context(), dashboardID, commentID, req.BaseVersion, prompt, httpapi.Caller(c))
	} else {
		row, err = s.SubmitFollowUp(c.Request.Context(), dashboardID, req.BaseVersion, panelID, prompt, httpapi.Caller(c))
	}
	if err != nil {
		s.fail(c, "enqueue revision", err, dashboardAttr(dashboardID))
		return
	}
	rev := revisionFromRow(row)
	views, err := s.revisionViews(c.Request.Context(), dashboardID, []Revision{rev}, httpapi.Caller(c))
	if err != nil {
		s.fail(c, "render enqueued revision", err, dashboardAttr(dashboardID))
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+rev.DashboardID+"/revisions/"+rev.ID)
	c.JSON(http.StatusAccepted, envelopeJSON{Revision: views[0]})
}

func threadCommentID(c *gin.Context, req createRequest) (pgtype.UUID, bool) {
	if req.CommentID == nil {
		return pgtype.UUID{}, true
	}
	id, err := db.ParseUUID(*req.CommentID)
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "comment_id must be a UUID")
		return pgtype.UUID{}, false
	}
	if req.PanelID != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "send comment_id or panel_id, not both; a thread request uses its thread's panel")
		return pgtype.UUID{}, false
	}
	return id, true
}

func (s *Service) listRevisions(c *gin.Context) {
	dashboardID, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if _, err := s.db.Queries().GetDashboardLatestVersion(ctx, dashboardID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpapi.AbortError(c, http.StatusNotFound, "not_found", "dashboard not found")
			return
		}
		s.fail(c, "read latest dashboard version", err, dashboardAttr(dashboardID))
		return
	}
	items, next, err := s.List(ctx, httpapi.Caller(c), dashboardID, limit, c.Query("cursor"))
	if err != nil {
		s.fail(c, "list revisions", err, dashboardAttr(dashboardID))
		return
	}
	views, err := s.revisionViews(ctx, dashboardID, items, httpapi.Caller(c))
	if err != nil {
		s.fail(c, "list revision views", err, dashboardAttr(dashboardID))
		return
	}
	body := listJSON{Revisions: views}
	if next != "" {
		body.NextCursor = &next
	}
	c.JSON(http.StatusOK, body)
}

func (s *Service) getRevision(c *gin.Context) {
	dashboardID, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	id, ok := httpapi.UUIDParam(c, "revision_id")
	if !ok {
		return
	}
	rev, err := s.Get(c.Request.Context(), httpapi.Caller(c), dashboardID, id)
	if err != nil {
		s.fail(c, "get revision", err, dashboardAttr(dashboardID), slog.String("agent_request_id", id.String()))
		return
	}
	views, err := s.revisionViews(c.Request.Context(), dashboardID, []Revision{rev}, httpapi.Caller(c))
	if err != nil {
		s.fail(c, "get revision views", err, dashboardAttr(dashboardID), slog.String("agent_request_id", id.String()))
		return
	}
	c.JSON(http.StatusOK, envelopeJSON{Revision: views[0]})
}

func (s *Service) revisionViews(ctx context.Context, dashboardID pgtype.UUID, revs []Revision, caller auth.Principal) ([]revisionJSON, error) {
	ids := make([]pgtype.UUID, len(revs))
	for i, rev := range revs {
		id, err := db.ParseUUID(rev.ID)
		if err != nil {
			return nil, fmt.Errorf("revision id: %w", err)
		}
		ids[i] = id
	}
	states, err := s.buildStates(ctx, ids)
	if err != nil {
		return nil, err
	}
	starts := s.newStartChecks(caller)
	views := make([]revisionJSON, len(revs))
	for i, rev := range revs {
		state := states[ids[i]]
		mayStart, err := starts.may(ctx, rev.State, rev.Source, RunTarget{Intent: IntentApply, DashboardID: dashboardID})
		if err != nil {
			return nil, fmt.Errorf("check who may start a revision again: %w", err)
		}
		mayEdit, err := starts.mayEdit(ctx, rev.State, dashboardID)
		if err != nil {
			return nil, fmt.Errorf("check who may keep a revision's build: %w", err)
		}
		view := s.viewFor(ctx, rev.audience, caller)
		facts := newFacts(rev.State, rev.Source, dashboardID, rev.BuildSavedAt != nil, state, sameActor(rev.RequestedBy, caller), mayStart && view == viewFull, mayEdit, false)
		views[i] = newRevisionJSON(rev, state, allowedActions(facts), caller)
		if view != viewFull {
			views[i].redact()
		}
	}
	return views, nil
}

func (s *Service) fail(c *gin.Context, action string, err error, idAttrs ...any) {
	ctx := c.Request.Context()
	if clientgone.Handle(c, s.logger, action, err) {
		return
	}
	reply, known := failureReplyFor(err)
	switch {
	case known && errors.Is(err, ErrForbidden):
		c.AbortWithStatusJSON(reply.Status, forbidden.BodyOf(err))
	case known:
		httpapi.AbortError(c, reply.Status, reply.Code, reply.Message)
	default:
		s.logger.ErrorContext(ctx, action, logattrs.Failure("internal", err, append(idAttrs, principalAttrs(c)...)...)...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}

var missingForFail = []error{ErrNotFound, ErrDashboardNotFound, ErrObservationNotFound, ErrDashboardVersionNotFound, ErrCommentNotFound, ErrChatNotFound, ErrRequestNotFound, ErrFindingNotFound}

func failureReplyFor(err error) (Reply, bool) {
	switch {
	case slices.ContainsFunc(missingForFail, func(target error) bool { return errors.Is(err, target) }):
		return Reply{Status: http.StatusNotFound, Code: "not_found", Message: err.Error()}, true
	case errors.Is(err, folders.ErrProvisioned):
		return Reply{Status: http.StatusConflict, Code: folders.CodeProvisioned, Message: err.Error()}, true
	case errors.Is(err, ErrChatReadOnly):
		return Reply{Status: http.StatusForbidden, Code: CodeChatReadOnly, Message: err.Error()}, true
	case errors.Is(err, ErrForbidden):
		return Reply{Status: http.StatusForbidden, Code: forbidden.Code, Message: forbidden.RoleMessage}, true
	case errors.Is(err, ErrUnknownPanel):
		return Reply{Status: http.StatusUnprocessableEntity, Code: "unknown_panel", Message: err.Error()}, true
	case errors.Is(err, ErrClientIDReused):
		return Reply{Status: http.StatusUnprocessableEntity, Code: "client_id_reused", Message: err.Error()}, true
	case errors.Is(err, ErrTooManyRevisions):
		return Reply{Status: http.StatusTooManyRequests, Code: "too_many_revisions", Message: err.Error()}, true
	case errors.Is(err, ErrFindingApplied):
		return Reply{Status: http.StatusConflict, Code: CodeAlreadyApplied, Message: err.Error()}, true
	case errors.Is(err, folders.ErrRestrictedAnswer):
		return Reply{Status: http.StatusConflict, Code: folders.CodeRestrictedAnswer, Message: err.Error()}, true
	case errors.Is(err, ErrTooManyRequests):
		return Reply{Status: http.StatusTooManyRequests, Code: "too_many_requests", Message: err.Error()}, true
	case errors.Is(err, ErrAgentBusy):
		return Reply{Status: http.StatusTooManyRequests, Code: "agent_busy", Message: err.Error()}, true
	case errors.Is(err, ErrUnavailable):
		return Reply{Status: http.StatusServiceUnavailable, Code: "agent_unavailable", Message: err.Error()}, true
	case errors.Is(err, ErrInvalidCursor), errors.Is(err, ErrAnchoredNeedsMine):
		return Reply{Status: http.StatusBadRequest, Code: "invalid_request", Message: err.Error()}, true
	}
	return Reply{}, false
}

func StartReplyFor(err error) Reply {
	var invalid *StartError
	if errors.As(err, &invalid) {
		return Reply{Status: http.StatusBadRequest, Code: "invalid_request", Message: invalid.Error()}
	}
	if reply, known := failureReplyFor(err); known {
		return reply
	}
	return Reply{Status: http.StatusInternalServerError, Code: "internal", Message: "internal error"}
}

func principalAttrs(c *gin.Context) []any {
	principal := httpapi.Caller(c)
	return []any{slog.String("principal_kind", string(principal.Kind)), slog.String("principal_id", principal.ID)}
}

func dashboardAttr(id pgtype.UUID) any {
	return slog.String("dashboard_id", id.String())
}

func newRevisionJSON(r Revision, state buildState, allowed []Action, caller auth.Principal) revisionJSON {
	out := revisionJSON{
		ID:             r.ID,
		DashboardID:    r.DashboardID,
		Intent:         r.Intent,
		Investigation:  r.Investigation,
		CommentID:      httpapi.OptionalString(r.CommentID),
		PanelID:        httpapi.OptionalString(r.PanelID),
		Prompt:         r.Prompt,
		BaseVersion:    r.BaseVersion,
		Status:         r.Status,
		State:          r.State,
		ResultKind:     r.ResultKind,
		Attempts:       r.Attempts,
		AllowedActions: actionStrings(allowed),
		ResultVersion:  r.ResultVersion,
		Error:          httpapi.OptionalString(r.Error),
		RequestedBy:    authorJSON{Kind: string(r.RequestedBy.Kind), ID: r.RequestedBy.ID, Name: r.RequestedBy.Name, Email: r.RequestedBy.Email},
		Origin:         httpapi.OptionalString(r.Origin),
		OriginClient:   httpapi.OptionalString(r.OriginClient),
		CreatedAt:      httpapi.FormatTime(r.CreatedAt),
		StartedAt:      httpapi.FormatOptionalTime(r.StartedAt),
		FinishedAt:     httpapi.FormatOptionalTime(r.FinishedAt),

		FailureCode:  httpapi.OptionalString(r.FailureCode),
		Build:        state.progressFor(r.RequestedBy, caller),
		BuildSavedAt: httpapi.FormatOptionalTime(r.BuildSavedAt),
		BuildState:   state.derived(),
		ContinuesID:  httpapi.OptionalString(r.ContinuesID),
		ContinuedBy:  state.continuedBy,
		RetriedBy:    state.retriedBy,
		ContinueNote: httpapi.OptionalString(r.ContinueNote),
	}
	return out
}

func (out *revisionJSON) redact() {
	out.Redacted = true
	out.Prompt, out.ContinueNote, out.Error = "", nil, nil
}
