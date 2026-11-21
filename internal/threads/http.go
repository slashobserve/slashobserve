package threads

import (
	"encoding/json"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type Threads struct {
	service *Service
	asks    *Asks
}

func NewThreads(service *Service, asks *Asks) *Threads {
	return &Threads{service: service, asks: asks}
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
	Unread            bool                 `json:"unread"`
	CreatedAt         string               `json:"created_at"`
	UpdatedAt         string               `json:"updated_at"`
	ResolvedAt        *string              `json:"resolved_at"`
	ResolvedActor     *actor.JSON          `json:"resolved_actor"`
	ResolvedByVersion *int                 `json:"resolved_by_version"`
	Revision          *commentRevisionJSON `json:"revision"`
	Anchor            *anchor.Anchor       `json:"anchor"`
	BlockID           *string              `json:"block_id"`
	Agent             *threadAgentJSON     `json:"agent"`
	AgentOff          bool                 `json:"agent_off"`
}

type threadAgentJSON struct {
	Kind string  `json:"kind"`
	ID   *string `json:"id"`
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
	CanDelete  bool            `json:"can_delete"`
	BlockID    *string         `json:"block_id"`
	ReplyTo    *replyToJSON    `json:"reply_to"`
	Run        *startedRunJSON `json:"run"`
	RunRefusal *runRefusalJSON `json:"run_refusal"`
	Answer     *answerJSON     `json:"answer"`
}

type answerJSON struct {
	Finding json.RawMessage `json:"finding"`
	CanSave bool            `json:"can_save"`
}

type replyToJSON struct {
	ID      string     `json:"id"`
	Actor   actor.JSON `json:"actor"`
	Excerpt string     `json:"excerpt"`
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

type sentEnvelopeJSON struct {
	Thread   threadJSON  `json:"thread"`
	Message  messageJSON `json:"message"`
	To       string      `json:"to"`
	Replayed bool        `json:"replayed"`
}

type openThreadRequest struct {
	Target   string         `json:"target"`
	Version  *int           `json:"version"`
	PanelID  *string        `json:"panel_id"`
	BlockID  *string        `json:"block_id"`
	Anchor   *anchor.Anchor `json:"anchor"`
	Body     string         `json:"body"`
	To       string         `json:"to"`
	ClientID *string        `json:"client_id"`

	StartRuns bool `json:"start_runs"`
}

type messageBodyRequest struct {
	Body string `json:"body"`
}

type replyRequest struct {
	Body      string  `json:"body"`
	BlockID   *string `json:"block_id"`
	ReplyToID *string `json:"reply_to_id"`
	To        string  `json:"to"`
	ClientID  *string `json:"client_id"`
	StartRuns bool    `json:"start_runs"`
}

func deliveryOf(to string, startRuns bool) (To, error) {
	if to == "" && startRuns {
		return ToAgent, nil
	}
	return ParseTo(to)
}

func ParseClientID(raw *string) (pgtype.UUID, error) {
	if raw == nil {
		return pgtype.UUID{}, nil
	}
	id, err := db.ParseUUID(*raw)
	if err != nil {
		return pgtype.UUID{}, &InvalidInputError{Code: CodeInvalidClientID, Message: "client_id must be a UUID"}
	}
	return id, nil
}

type applyRequest struct {
	FindingID string `json:"finding_id"`
}

type resolveThreadRequest struct {
	Version *int `json:"version"`
}

func (th *Threads) Register(api auth.Routes) {
	api.GET("/threads", auth.RoleViewer, th.list)
	api.GET("/threads/mentionable", auth.RoleViewer, th.mentionable)
	api.POST("/threads", auth.RoleViewer, th.open)
	api.POST("/threads/:thread_id/messages", auth.RoleViewer, th.reply)
	api.POST("/threads/:thread_id/resolve", auth.RoleViewer, th.resolve)
	api.POST("/threads/:thread_id/reopen", auth.RoleViewer, th.reopen)
	api.POST("/threads/:thread_id/read", auth.RoleViewer, auth.RequireUser, th.markRead)
	api.DELETE("/threads/:thread_id/share", auth.RoleViewer, th.withdraw)
	api.POST("/threads/:thread_id/apply", auth.RoleViewer, th.apply)
	api.PATCH("/messages/:message_id", auth.RoleViewer, th.edit)
	api.DELETE("/messages/:message_id", auth.RoleViewer, th.deleteMessage)
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
		Target: target, Status: status, PanelID: panelID, Limit: limit, Cursor: c.Query("cursor"), WithAnswers: true,
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

type mentionablePersonJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (th *Threads) mentionable(c *gin.Context) {
	target, err := ParseTarget(c.Query("target"))
	if err != nil {
		th.fail(c, "list mentionable people", err)
		return
	}
	limit, ok := httpapi.LimitParam(c, maxMentionablePeople, maxMentionablePeople)
	if !ok {
		return
	}
	term := c.Query("q")
	if utf8.RuneCountInString(term) > maxMentionTermRunes {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "q must be at most 100 characters")
		return
	}
	people, more, err := th.service.MentionablePeople(c.Request.Context(), httpapi.Caller(c), target, term, limit)
	if err != nil {
		th.fail(c, "list mentionable people", err)
		return
	}
	body := make([]mentionablePersonJSON, len(people))
	for i, p := range people {
		body[i] = mentionablePersonJSON(p)
	}
	c.JSON(http.StatusOK, gin.H{"people": body, "more": more})
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
	to, err := deliveryOf(req.To, req.StartRuns)
	if err != nil {
		th.fail(c, "open thread", err)
		return
	}
	clientID, err := ParseClientID(req.ClientID)
	if err != nil {
		th.fail(c, "open thread", err)
		return
	}
	sent, err := th.asks.Open(c.Request.Context(), httpapi.Caller(c), OpenInput{
		Target: target, Version: req.Version, PanelID: req.PanelID, BlockID: req.BlockID, Anchor: req.Anchor, ClientID: clientID, Body: req.Body,
	}, to)
	if err != nil {
		th.fail(c, "open thread", err)
		return
	}
	th.respondSent(c, sent)
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
	to, err := deliveryOf(req.To, req.StartRuns)
	if err != nil {
		th.fail(c, "reply to thread", err)
		return
	}
	clientID, err := ParseClientID(req.ClientID)
	if err != nil {
		th.fail(c, "reply to thread", err)
		return
	}
	sent, err := th.asks.Reply(c.Request.Context(), httpapi.Caller(c), id, ReplyInput{Body: req.Body, BlockID: req.BlockID, ReplyToID: req.ReplyToID, ClientID: clientID}, to)
	if err != nil {
		th.fail(c, "reply to thread", err)
		return
	}
	th.respondSent(c, sent)
}

func (th *Threads) respondSent(c *gin.Context, sent Sent) {
	threads, err := th.threadsJSON(c, []Thread{sent.Thread})
	if err != nil {
		th.fail(c, "read thread", err)
		return
	}
	status := http.StatusCreated
	if sent.Replayed {
		status = http.StatusOK
	}
	c.JSON(status, sentEnvelopeJSON{Thread: threads[0], Message: withRun(newMessageJSON(sent.Message), sent.Run, sent.Refusal), To: string(sent.To), Replayed: sent.Replayed})
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

func (th *Threads) deleteMessage(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "message_id")
	if !ok {
		return
	}
	if err := th.service.Delete(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		th.fail(c, "delete message", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (th *Threads) withdraw(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "thread_id")
	if !ok {
		return
	}
	if err := th.service.Withdraw(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		th.fail(c, "withdraw shared answer", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (th *Threads) apply(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "thread_id")
	if !ok {
		return
	}
	var req applyRequest
	if !httpapi.DecodeBody(c, &req, maxCommentRequestBytes) {
		return
	}
	findingID, err := db.ParseUUID(req.FindingID)
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "finding_id must be a UUID")
		return
	}
	sent, err := th.asks.Apply(c.Request.Context(), httpapi.Caller(c), id, findingID)
	if err != nil {
		th.failApply(c, err)
		return
	}
	th.respondSent(c, sent)
}

func (th *Threads) failApply(c *gin.Context, err error) {
	reply, known := revisions.ReplyFor(err)
	switch {
	case errors.Is(err, ErrAnswerNotHere):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, revisions.ErrTooManyRevisions), errors.Is(err, revisions.ErrUnavailable):
		reply = revisions.StartReplyFor(err)
		httpapi.AbortError(c, reply.Status, reply.Code, reply.Message)
	case known:
		httpapi.AbortError(c, reply.Status, reply.Code, reply.Message)
	default:
		th.fail(c, "apply suggested change", err)
	}
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

func (th *Threads) markRead(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "thread_id")
	if !ok {
		return
	}
	if err := th.service.MarkRead(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		th.fail(c, "mark thread read", err)
		return
	}
	c.Status(http.StatusNoContent)
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
	switch {
	case errors.Is(err, ErrNotAuthor), errors.Is(err, ErrCannotDelete):
		httpapi.AbortError(c, http.StatusForbidden, CodeNotAuthor, err.Error())
	case errors.As(err, &invalid) && invalid.Code != "":
		httpapi.AbortText(c, http.StatusUnprocessableEntity, invalid.Code, invalid)
	default:
		th.service.dashboards.Fail(c, action, err)
	}
}

func abortKnown(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrTargetNotFound), errors.Is(err, ErrNothingShared):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrOthersWroteHere):
		httpapi.AbortError(c, http.StatusConflict, CodeOthersWroteHere, err.Error())
	case errors.Is(err, ErrNotStarter):
		httpapi.AbortError(c, http.StatusForbidden, CodeNotStarter, err.Error())
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
		Unread:            t.Unread,
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
	out.Anchor, out.BlockID, out.AgentOff = t.Anchor, httpapi.OptionalString(t.BlockID), t.AgentOff
	if t.Agent != nil {
		out.Agent = &threadAgentJSON{Kind: string(t.Agent.Kind)}
		if t.Agent.ID.Valid {
			id := t.Agent.ID.String()
			out.Agent.ID = &id
		}
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
		CanDelete: m.CanDelete,
		BlockID:   httpapi.OptionalString(m.BlockID),
	}
	if m.Run != nil {
		out.Run = &startedRunJSON{ID: m.Run.ID, Status: m.Run.Status, AssignedKind: m.Run.AssignedKind}
	}
	if m.ReplyTo != nil {
		out.ReplyTo = &replyToJSON{ID: m.ReplyTo.ID, Actor: m.ReplyTo.Author.JSON(), Excerpt: m.ReplyTo.Excerpt}
	}
	if m.Answer != nil {
		out.Answer = &answerJSON{Finding: m.Answer.Finding, CanSave: m.Answer.CanSave}
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
