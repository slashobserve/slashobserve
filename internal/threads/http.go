package threads

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

type Threads struct {
	service *Service
	runs    *MentionRuns
}

func NewThreads(service *Service, runs *MentionRuns) *Threads {
	return &Threads{service: service, runs: runs}
}

type targetJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type threadJSON struct {
	ID                string               `json:"id"`
	Target            targetJSON           `json:"target"`
	DashboardID       *string              `json:"dashboard_id"`
	Version           *int                 `json:"version"`
	PanelID           *string              `json:"panel_id"`
	PanelPresent      bool                 `json:"panel_present"`
	Status            string               `json:"status"`
	AllowedActions    []string             `json:"allowed_actions"`
	CreatedAt         string               `json:"created_at"`
	UpdatedAt         string               `json:"updated_at"`
	ResolvedAt        *string              `json:"resolved_at"`
	ResolvedActor     *actor.JSON          `json:"resolved_actor"`
	ResolvedByVersion *int                 `json:"resolved_by_version"`
	Revision          *commentRevisionJSON `json:"revision"`
}

type messageJSON struct {
	ID         string          `json:"id"`
	ThreadID   string          `json:"thread_id"`
	Body       string          `json:"body"`
	Actor      actor.JSON      `json:"actor"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
	EditedAt   *string         `json:"edited_at"`
	Mentions   []mentionJSON   `json:"mentions"`
	CanEdit    bool            `json:"can_edit"`
	Run        *startedRunJSON `json:"run"`
	RunRefusal *runRefusalJSON `json:"run_refusal"`
}

type startedRunJSON struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	AssignedKind string `json:"assigned_kind"`
}

type runRefusalJSON struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type threadPageJSON struct {
	Threads    []threadJSON  `json:"threads"`
	Messages   []messageJSON `json:"messages"`
	NextCursor *string       `json:"next_cursor"`
	CanComment bool          `json:"can_comment"`
}

type threadEnvelopeJSON struct {
	Thread threadJSON `json:"thread"`
}

type threadMessageEnvelopeJSON struct {
	Thread  threadJSON  `json:"thread"`
	Message messageJSON `json:"message"`
}

type openThreadRequest struct {
	Target  string  `json:"target"`
	Version *int    `json:"version"`
	PanelID *string `json:"panel_id"`
	Body    string  `json:"body"`

	StartRuns bool `json:"start_runs"`
}

type messageBodyRequest struct {
	Body string `json:"body"`
}

type replyRequest struct {
	Body      string `json:"body"`
	StartRuns bool   `json:"start_runs"`
}

type resolveThreadRequest struct {
	Version *int `json:"version"`
}

func (th *Threads) Register(api auth.Routes) {
	api.GET("/threads", auth.RoleViewer, th.list)
	api.POST("/threads", auth.RoleViewer, th.open)
	api.POST("/threads/:thread_id/messages", auth.RoleViewer, th.reply)
	api.POST("/threads/:thread_id/resolve", auth.RoleViewer, th.resolve)
	api.POST("/threads/:thread_id/reopen", auth.RoleViewer, th.reopen)
	api.PATCH("/messages/:message_id", auth.RoleViewer, th.edit)
}

func (th *Threads) list(c *gin.Context) {
	target, err := ParseTarget(c.Query("target"))
	if err != nil {
		th.fail(c, "list threads", err)
		return
	}
	limit, ok := httpapi.LimitParam(c, defaultCommentLimit, maxCommentLimit)
	if !ok {
		return
	}
	status, ok := httpapi.NonEmptyQuery(c, "status")
	if !ok {
		return
	}
	panelID, ok := httpapi.NonEmptyQuery(c, "panel_id")
	if !ok {
		return
	}
	page, err := th.service.List(c.Request.Context(), httpapi.Caller(c), ListQuery{
		Target: target, Status: status, PanelID: panelID, Limit: limit, Cursor: c.Query("cursor"),
	})
	if err != nil {
		th.fail(c, "list threads", err)
		return
	}
	threads, err := th.threadsJSON(c, page.Threads)
	if err != nil {
		th.fail(c, "list threads", err)
		return
	}
	body := threadPageJSON{
		Threads:    threads,
		Messages:   make([]messageJSON, 0, len(page.Messages)),
		NextCursor: httpapi.OptionalString(page.Next),
		CanComment: page.CanComment,
	}
	for _, m := range page.Messages {
		body.Messages = append(body.Messages, newMessageJSON(m))
	}
	c.JSON(http.StatusOK, body)
}

func (th *Threads) open(c *gin.Context) {
	var req openThreadRequest
	if !httpapi.DecodeBody(c, &req, maxCommentRequestBytes) {
		return
	}
	target, err := ParseTarget(req.Target)
	if err != nil {
		th.fail(c, "open thread", err)
		return
	}
	thread, message, run, refusal, err := th.runs.Open(c.Request.Context(), httpapi.Caller(c), OpenInput{
		Target: target, Version: req.Version, PanelID: req.PanelID, Body: req.Body,
	}, req.StartRuns)
	if err != nil {
		th.fail(c, "open thread", err)
		return
	}
	th.respondWithMessage(c, http.StatusCreated, thread, withRun(newMessageJSON(message), run, refusal))
}

func (th *Threads) reply(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "thread_id")
	if !ok {
		return
	}
	var req replyRequest
	if !httpapi.DecodeBody(c, &req, maxCommentRequestBytes) {
		return
	}
	thread, message, run, refusal, err := th.runs.Reply(c.Request.Context(), httpapi.Caller(c), id, req.Body, req.StartRuns)
	if err != nil {
		th.fail(c, "reply to thread", err)
		return
	}
	th.respondWithMessage(c, http.StatusCreated, thread, withRun(newMessageJSON(message), run, refusal))
}

func (th *Threads) edit(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "message_id")
	if !ok {
		return
	}
	var req messageBodyRequest
	if !httpapi.DecodeBody(c, &req, maxCommentRequestBytes) {
		return
	}
	thread, message, _, err := th.service.Edit(c.Request.Context(), httpapi.Caller(c), id, req.Body)
	if err != nil {
		th.fail(c, "edit message", err)
		return
	}
	th.respondWithMessage(c, http.StatusOK, thread, newMessageJSON(message))
}

func (th *Threads) resolve(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "thread_id")
	if !ok {
		return
	}
	var req resolveThreadRequest
	if c.Request.ContentLength != 0 && !httpapi.DecodeBody(c, &req, maxCommentRequestBytes) {
		return
	}
	th.setStatus(c, id, StatusResolved, req.Version)
}

func (th *Threads) reopen(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "thread_id")
	if !ok {
		return
	}
	th.setStatus(c, id, StatusOpen, nil)
}

func (th *Threads) setStatus(c *gin.Context, id pgtype.UUID, status string, resolvedByVersion *int) {
	thread, _, err := th.service.SetStatus(c.Request.Context(), httpapi.Caller(c), id, status, resolvedByVersion)
	if err != nil {
		th.fail(c, "set thread status", err)
		return
	}
	threads, err := th.threadsJSON(c, []Thread{thread})
	if err != nil {
		th.fail(c, "set thread status", err)
		return
	}
	c.JSON(http.StatusOK, threadEnvelopeJSON{Thread: threads[0]})
}

func (th *Threads) respondWithMessage(c *gin.Context, status int, thread Thread, message messageJSON) {
	threads, err := th.threadsJSON(c, []Thread{thread})
	if err != nil {
		th.fail(c, "read thread", err)
		return
	}
	c.JSON(status, threadMessageEnvelopeJSON{Thread: threads[0], Message: message})
}

func (th *Threads) threadsJSON(c *gin.Context, threads []Thread) ([]threadJSON, error) {
	var dashboardThreads []string
	for _, t := range threads {
		if t.Target.Kind == KindDashboard {
			dashboardThreads = append(dashboardThreads, t.ID)
		}
	}
	latest, err := th.service.latestRevisions(c.Request.Context(), dashboardThreads)
	if err != nil {
		return nil, err
	}
	out := make([]threadJSON, 0, len(threads))
	for _, t := range threads {
		j := newThreadJSON(t)
		if rev := latest[t.ID]; rev != nil {
			j.Revision = &commentRevisionJSON{ID: rev.ID, Status: rev.Status}
		}
		out = append(out, j)
	}
	return out, nil
}

func (th *Threads) fail(c *gin.Context, action string, err error) {
	if abortKnown(c, err) {
		return
	}
	var invalid *InvalidInputError
	var exists *ObservationThreadExistsError
	switch {
	case errors.As(err, &exists):
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": exists.Error(), "code": CodeObservationThreadExists, "thread_id": exists.ThreadID.String()})
	case errors.Is(err, ErrNotAuthor):
		httpapi.AbortError(c, http.StatusForbidden, CodeNotAuthor, err.Error())
	case errors.As(err, &invalid) && invalid.Code != "":
		httpapi.AbortError(c, http.StatusUnprocessableEntity, invalid.Code, invalid.Message)
	default:
		th.service.dashboards.Fail(c, action, err)
	}
}

func abortKnown(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrTargetNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrUnknownPanel):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, CodeUnknownPanel, err.Error())
	case errors.Is(err, ErrUnknownVersion):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, CodeUnknownVersion, err.Error())
	default:
		return false
	}
	return true
}

func newThreadJSON(t Thread) threadJSON {
	out := threadJSON{
		ID:                t.ID,
		Target:            targetJSON{Kind: string(t.Target.Kind), ID: t.Target.ID.String()},
		DashboardID:       httpapi.OptionalString(t.DashboardID),
		Version:           t.Version,
		PanelID:           httpapi.OptionalString(t.PanelID),
		PanelPresent:      t.PanelPresent,
		Status:            t.Status,
		AllowedActions:    t.AllowedActions,
		CreatedAt:         httpapi.FormatTime(t.CreatedAt),
		UpdatedAt:         httpapi.FormatTime(t.UpdatedAt),
		ResolvedByVersion: t.ResolvedByVersion,
	}
	if t.ResolvedAt != nil {
		at := httpapi.FormatTime(*t.ResolvedAt)
		out.ResolvedAt = &at
	}
	if t.ResolvedBy != nil {
		resolver := t.ResolvedBy.JSON()
		out.ResolvedActor = &resolver
	}
	return out
}

func newMessageJSON(m Message) messageJSON {
	out := messageJSON{
		ID:        m.ID,
		ThreadID:  m.ThreadID,
		Body:      m.Body,
		Actor:     m.Author.JSON(),
		CreatedAt: httpapi.FormatTime(m.CreatedAt),
		UpdatedAt: httpapi.FormatTime(m.UpdatedAt),
		Mentions:  newMentionsJSON(m.Mentions),
		CanEdit:   m.CanEdit,
	}
	if m.EditedAt != nil {
		at := httpapi.FormatTime(*m.EditedAt)
		out.EditedAt = &at
	}
	return out
}

func withRun(message messageJSON, run *StartedRun, refusal *RunRefusal) messageJSON {
	if run != nil {
		message.Run = &startedRunJSON{ID: run.ID, Status: run.Status, AssignedKind: run.AssignedKind}
	}
	if refusal != nil {
		message.RunRefusal = &runRefusalJSON{Code: refusal.Code, Message: refusal.Message}
	}
	return message
}
