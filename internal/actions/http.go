package actions

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxBodyBytes        = 64 << 10
	maxIDBytes          = 64
	maxVariableValues   = 100
	maxValueBytes       = 1000
	maxFields           = 50
	maxInFlightPerUser  = 2
	sendTimeout         = 15 * time.Second
	auditMessage        = "canvas action run"
	staleVersionMessage = "The dashboard changed; reload it before running this action."
	invalidMessage      = "The action could not be built from the current values."
)

type Specs interface {
	LatestVersion(ctx context.Context, id pgtype.UUID) (int, error)
	VersionSpec(ctx context.Context, id pgtype.UUID, version int) (*spec.Dashboard, error)
}

type Sender interface {
	Send(ctx context.Context, req egress.SendRequest) (egress.SendResult, error)
}

type Handler struct {
	specs       Specs
	sender      Sender
	requireEdit gin.HandlerFunc
	logger      *slog.Logger
	inFlight    *inFlight
}

func NewHandler(specs Specs, sender Sender, requireEdit gin.HandlerFunc, logger *slog.Logger) *Handler {
	return &Handler{specs: specs, sender: sender, requireEdit: requireEdit, logger: logger, inFlight: &inFlight{counts: map[string]int{}}}
}

func (h *Handler) Register(api auth.Routes) {
	api.POST("/dashboards/:id/actions", auth.RoleViewer, auth.RequireUser, h.requireEdit, h.run)
}

type inFlight struct {
	mu     sync.Mutex
	counts map[string]int
}

func (f *inFlight) acquire(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.counts[id] >= maxInFlightPerUser {
		return false
	}
	f.counts[id]++
	return true
}

func (f *inFlight) release(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.counts[id]--; f.counts[id] <= 0 {
		delete(f.counts, id)
	}
}

type valueRequest struct {
	Raw     string   `json:"raw"`
	Text    string   `json:"text"`
	Numeric *float64 `json:"numeric"`
}

type runRequest struct {
	Version   int                 `json:"version"`
	Panel     string              `json:"panel"`
	Element   string              `json:"element"`
	Action    *int                `json:"action"`
	Variables map[string][]string `json:"variables"`
	From      *time.Time          `json:"from"`
	To        *time.Time          `json:"to"`
	Value     *valueRequest       `json:"value"`
	Fields    map[string]string   `json:"fields"`
}

type runResponse struct {
	Status int  `json:"status"`
	OK     bool `json:"ok"`
}

func (r runRequest) shapeProblem() string {
	switch {
	case r.Version < 1 || r.Version > math.MaxInt32:
		return "version must be a saved version number"
	case r.Panel == "" || len(r.Panel) > maxIDBytes || r.Element == "" || len(r.Element) > maxIDBytes:
		return "panel and element must be ids of at most 64 bytes"
	case r.Action == nil || *r.Action < 0:
		return "action must be the index of an action, starting at 0"
	case r.From == nil || r.To == nil:
		return "from and to are required"
	}
	return r.valueProblem()
}

func (r runRequest) valueProblem() string {
	if len(r.Fields) > maxFields {
		return "fields holds at most 50 entries"
	}
	for _, values := range r.Variables {
		if len(values) > maxVariableValues || slices.ContainsFunc(values, tooLong) {
			return "each variable takes at most 100 values of at most 1000 bytes"
		}
	}
	for name, value := range r.Fields {
		if len(name) > maxValueBytes || tooLong(value) {
			return "field names and values are at most 1000 bytes"
		}
	}
	if v := r.Value; v != nil && (tooLong(v.Raw) || tooLong(v.Text)) {
		return "value.raw and value.text are at most 1000 bytes"
	}
	return ""
}

func tooLong(s string) bool { return len(s) > maxValueBytes }

func findAction(d *spec.Dashboard, panelID, elementID string, index int) (spec.Action, bool) {
	entry, ok := d.FindPanel(panelID)
	if !ok || entry.Panel.Type != spec.Canvas || entry.Panel.Display == nil {
		return spec.Action{}, false
	}
	for _, el := range entry.Panel.Display.Elements {
		if el.ID == elementID && index < len(el.Actions) {
			return el.Actions[index], true
		}
	}
	return spec.Action{}, false
}

func undeclared(d *spec.Dashboard, variables map[string][]string) bool {
	for name := range variables {
		if !slices.ContainsFunc(d.Variables, func(v spec.Variable) bool { return v.Name == name }) {
			return true
		}
	}
	return false
}

func (h *Handler) run(c *gin.Context) {
	var req runRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	if problem := req.shapeProblem(); problem != "" {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", problem)
		return
	}
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	caller := httpapi.Caller(c)
	ctx := c.Request.Context()

	dash, err := h.loadLatest(ctx, id, req.Version)
	switch {
	case errors.Is(err, errStale):
		httpapi.AbortError(c, http.StatusConflict, "stale_version", staleVersionMessage)
		return
	case err != nil:
		h.fail(c, "load canvas action", err)
		return
	}
	action, found := findAction(dash, req.Panel, req.Element, *req.Action)
	if !found {
		httpapi.AbortError(c, http.StatusNotFound, "action_not_found", "The action no longer exists in this dashboard.")
		return
	}
	if undeclared(dash, req.Variables) {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "variables may only name variables the dashboard declares")
		return
	}
	expanded, err := spec.ExpandAction(action, environment(req, caller))
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "action_invalid", invalidMessage)
		return
	}
	if !h.inFlight.acquire(caller.ID) {
		httpapi.AbortError(c, http.StatusTooManyRequests, "too_many_actions", "Too many actions are running; wait for one to finish.")
		return
	}
	defer h.inFlight.release(caller.ID)

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	started := time.Now()
	result, err := h.sender.Send(sendCtx, egress.SendRequest{Method: expanded.Method, URL: expanded.URL, Header: expanded.Header, Body: expanded.Body})
	if err != nil {
		result = egress.SendResult{}
	}
	host := hostOf(err, expanded.Host)
	h.audit(ctx, auditFields{
		caller: caller, dashboardID: c.Param("id"), req: req, method: expanded.Method, host: host,
		status: result.Status, outcome: outcomeOf(result.Status, err), elapsed: time.Since(started),
	})
	h.respond(c, result, err, host)
}

var errStale = errors.New("stale version")

func (h *Handler) loadLatest(ctx context.Context, id pgtype.UUID, version int) (*spec.Dashboard, error) {
	latest, err := h.specs.LatestVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	if latest != version {
		return nil, errStale
	}
	return h.specs.VersionSpec(ctx, id, version)
}

func environment(req runRequest, caller auth.Principal) spec.ActionEnv {
	env := spec.ActionEnv{
		Variables: req.Variables,
		From:      *req.From,
		To:        *req.To,
		User:      spec.ActionUser{ID: caller.ID, Email: caller.Email, Name: caller.Name},
		Fields:    req.Fields,
	}
	if v := req.Value; v != nil {
		env.Value = spec.ActionValue{Raw: v.Raw, Text: v.Text, Numeric: v.Numeric}
	}
	return env
}

func hostOf(err error, fallback string) string {
	var hostErr *egress.HostError
	if errors.As(err, &hostErr) {
		return hostErr.Host
	}
	return fallback
}

func outcomeOf(status int, err error) string {
	switch {
	case err == nil && status >= 200 && status < 300:
		return "ok"
	case err == nil:
		return "remote_error"
	case errors.Is(err, egress.ErrNotAllowed), errors.Is(err, egress.ErrInvalidURL),
		errors.Is(err, egress.ErrTooLarge), errors.Is(err, egress.ErrBlockedAddress):
		return "refused"
	}
	return "failed"
}

type auditFields struct {
	caller      auth.Principal
	dashboardID string
	req         runRequest
	method      string
	host        string
	status      int
	outcome     string
	elapsed     time.Duration
}

func (h *Handler) audit(ctx context.Context, f auditFields) {
	h.logger.InfoContext(ctx, auditMessage,
		slog.String("actor_id", f.caller.ID),
		slog.String("actor_role", string(f.caller.Role)),
		slog.String("dashboard_id", f.dashboardID),
		slog.Int("version", f.req.Version),
		slog.String("panel", f.req.Panel),
		slog.String("element", f.req.Element),
		slog.Int("action", *f.req.Action),
		slog.String("method", f.method),
		slog.String("host", f.host),
		slog.Int("status", f.status),
		slog.String("outcome", f.outcome),
		slog.Int64("duration_ms", f.elapsed.Milliseconds()),
	)
}

func (h *Handler) respond(c *gin.Context, result egress.SendResult, err error, host string) {
	switch {
	case err == nil:
		c.JSON(http.StatusOK, runResponse{Status: result.Status, OK: result.Status >= 200 && result.Status < 300})
	case clientgone.Handle(c, h.logger, "run canvas action", err):
	case errors.Is(err, egress.ErrNotAllowed):
		abortWithHost(c, http.StatusForbidden, "outbound_domain_not_allowed", host+" is not on the outbound allowlist.", host)
	case errors.Is(err, egress.ErrInvalidURL):
		httpapi.AbortError(c, http.StatusBadRequest, "action_invalid", invalidMessage)
	case errors.Is(err, egress.ErrTooLarge):
		httpapi.AbortError(c, http.StatusBadRequest, "action_invalid", "The action's body is larger than 64 KiB.")
	case errors.Is(err, egress.ErrBlockedAddress):
		abortWithHost(c, http.StatusBadGateway, "action_failed", host+" points to a private or reserved address.", host)
	case errors.Is(err, egress.ErrUnavailable), errors.Is(err, context.DeadlineExceeded):
		abortWithHost(c, http.StatusBadGateway, "action_failed", "The request to "+host+" failed.", host)
	default:
		h.fail(c, "run canvas action", err)
	}
}

func abortWithHost(c *gin.Context, status int, code, message, host string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message, "code": code, "host": host})
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	switch {
	case clientgone.Handle(c, h.logger, action, err):
	case errors.Is(err, dashboards.ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "dashboard not found")
	default:
		h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err,
			slog.String("actor_id", httpapi.Caller(c).ID), slog.String("dashboard_id", c.Param("id")))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}
