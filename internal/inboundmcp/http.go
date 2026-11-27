package inboundmcp

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const maxBodyBytes = 16 << 10

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/mcp-sources")
	g.GET("", auth.RoleAdmin, h.list)
	g.POST("", auth.RoleAdmin, h.create)
	g.GET("/:uid", auth.RoleAdmin, h.get)
	g.PATCH("/:uid", auth.RoleAdmin, h.update)
	g.DELETE("/:uid", auth.RoleAdmin, h.remove)
	g.POST("/:uid/discover", auth.RoleAdmin, h.discover)
	g.GET("/:uid/tools", auth.RoleAdmin, h.tools)
	g.POST("/:uid/tools/approve-read-only", auth.RoleAdmin, h.approveReadOnly)
	g.POST("/:uid/tools/:name/approve", auth.RoleAdmin, h.approve)
	g.POST("/:uid/tools/:name/revoke", auth.RoleAdmin, h.revoke)
}

type toolCountsJSON struct {
	Offered  int `json:"offered"`
	Approved int `json:"approved"`
	Changed  int `json:"changed"`
	Gone     int `json:"gone"`
	Refused  int `json:"refused"`
}

type sourceJSON struct {
	UID          string         `json:"uid"`
	Name         string         `json:"name"`
	Description  *string        `json:"description"`
	URL          string         `json:"url"`
	Audience     string         `json:"audience"`
	SecretHeader *string        `json:"secret_header"`
	SecretSet    bool           `json:"secret_set"`
	Provisioned  bool           `json:"provisioned"`
	Private      bool           `json:"private"`
	State        string         `json:"state"`
	Failure      *string        `json:"failure"`
	CheckedAt    *string        `json:"checked_at"`
	Tools        toolCountsJSON `json:"tools"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

func newSourceJSON(s Source) sourceJSON {
	return sourceJSON{
		UID: s.UID, Name: s.Name, Description: httpapi.OptionalString(s.Description), URL: s.URL, Audience: string(s.Audience),
		SecretHeader: httpapi.OptionalString(s.SecretHeader), SecretSet: s.SecretSet, Provisioned: s.Provisioned, Private: s.Private,
		State: s.State, Failure: httpapi.OptionalString(s.Failure), CheckedAt: httpapi.FormatOptionalTime(s.CheckedAt),
		Tools:     toolCountsJSON(s.Tools),
		CreatedAt: httpapi.FormatTime(s.CreatedAt), UpdatedAt: httpapi.FormatTime(s.UpdatedAt),
	}
}

type definitionJSON struct {
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	ReadOnly    bool            `json:"read_only"`
	Hash        string          `json:"hash"`
}

type toolJSON struct {
	Name           string          `json:"name"`
	State          string          `json:"state"`
	Refusal        *string         `json:"refusal"`
	Description    string          `json:"description"`
	InputSchema    json.RawMessage `json:"input_schema"`
	ReadOnly       bool            `json:"read_only"`
	Hash           string          `json:"hash"`
	Seen           *definitionJSON `json:"seen"`
	ApprovedBy     *string         `json:"approved_by"`
	ApprovedAt     *string         `json:"approved_at"`
	UpdatedAt      string          `json:"updated_at"`
	AllowedActions []string        `json:"allowed_actions"`
}

func newToolJSON(t Tool) toolJSON {
	out := toolJSON{
		Name: t.Name, State: string(t.State), Refusal: httpapi.OptionalString(t.Refusal),
		Description: t.Pinned.Description, InputSchema: t.Pinned.InputSchema, ReadOnly: t.Pinned.ReadOnly, Hash: t.Pinned.Hash,
		ApprovedBy: httpapi.OptionalString(t.ApprovedBy), UpdatedAt: httpapi.FormatTime(t.UpdatedAt),
		AllowedActions: []string{},
	}
	if t.Seen != nil {
		d := definitionJSON(*t.Seen)
		out.Seen = &d
	}
	if !t.ApprovedAt.IsZero() {
		at := httpapi.FormatTime(t.ApprovedAt)
		out.ApprovedAt = &at
	}
	for _, a := range ToolMachine.Allowed(t.State, isAdmin) {
		out.AllowedActions = append(out.AllowedActions, string(a))
	}
	return out
}

func toolsJSON(tools []Tool) []toolJSON {
	out := make([]toolJSON, len(tools))
	for i, t := range tools {
		out[i] = newToolJSON(t)
	}
	return out
}

type createBody struct {
	UID          string `json:"uid"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	URL          string `json:"url"`
	Audience     string `json:"audience"`
	SecretHeader string `json:"secret_header"`
	Secret       string `json:"secret"`
}

type updateBody struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	URL          *string `json:"url"`
	Audience     *string `json:"audience"`
	SecretHeader *string `json:"secret_header"`
	Secret       *string `json:"secret"`
	ClearSecret  bool    `json:"clear_secret"`
}

type approveBody struct {
	Hash string `json:"hash"`
}

func (h *Handler) list(c *gin.Context) {
	sources, err := h.svc.List(c.Request.Context())
	if err != nil {
		h.fail(c, "list MCP sources", err)
		return
	}
	out := make([]sourceJSON, len(sources))
	for i, s := range sources {
		out[i] = newSourceJSON(s)
	}
	c.JSON(http.StatusOK, gin.H{"mcp_sources": out})
}

func (h *Handler) get(c *gin.Context) {
	source, err := h.svc.Get(c.Request.Context(), c.Param("uid"))
	if err != nil {
		h.fail(c, "read MCP source", err)
		return
	}
	c.JSON(http.StatusOK, newSourceJSON(source))
}

func (h *Handler) create(c *gin.Context) {
	var body createBody
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	source, err := h.svc.Create(c.Request.Context(), httpapi.Caller(c), Input{
		UID: body.UID, Name: body.Name, Description: body.Description, URL: body.URL, Audience: datasource.Audience(body.Audience),
		SecretHeader: body.SecretHeader, Secret: body.Secret,
	})
	if err != nil {
		h.fail(c, "add MCP source", err)
		return
	}
	c.Header("Location", "/api/v1/mcp-sources/"+source.UID)
	c.JSON(http.StatusCreated, newSourceJSON(source))
}

func (h *Handler) update(c *gin.Context) {
	var body updateBody
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	patch := Patch{Name: body.Name, Description: body.Description, URL: body.URL, SecretHeader: body.SecretHeader, Secret: body.Secret, ClearSecret: body.ClearSecret}
	if body.Audience != nil {
		audience := datasource.Audience(*body.Audience)
		patch.Audience = &audience
	}
	source, err := h.svc.Update(c.Request.Context(), httpapi.Caller(c), c.Param("uid"), patch)
	if err != nil {
		h.fail(c, "change MCP source", err)
		return
	}
	c.JSON(http.StatusOK, newSourceJSON(source))
}

func (h *Handler) remove(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), httpapi.Caller(c), c.Param("uid")); err != nil {
		h.fail(c, "remove MCP source", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) discover(c *gin.Context) {
	found, err := h.svc.Discover(c.Request.Context(), httpapi.Caller(c), c.Param("uid"))
	if err != nil {
		h.fail(c, "discover MCP source tools", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"source": newSourceJSON(found.Source), "tally": found.Tally})
}

func (h *Handler) tools(c *gin.Context) {
	tools, err := h.svc.Tools(c.Request.Context(), c.Param("uid"))
	if err != nil {
		h.fail(c, "list MCP source tools", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tools": toolsJSON(tools)})
}

func (h *Handler) approve(c *gin.Context) {
	var body approveBody
	if c.Request.ContentLength != 0 && !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	tool, err := h.svc.Approve(c.Request.Context(), httpapi.Caller(c), c.Param("uid"), c.Param("name"), body.Hash)
	if err != nil {
		h.fail(c, "approve MCP source tool", err)
		return
	}
	c.JSON(http.StatusOK, newToolJSON(tool))
}

func (h *Handler) approveReadOnly(c *gin.Context) {
	tools, err := h.svc.ApproveReadOnly(c.Request.Context(), httpapi.Caller(c), c.Param("uid"))
	if err != nil {
		h.fail(c, "approve MCP source tools", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"approved": toolsJSON(tools)})
}

func (h *Handler) revoke(c *gin.Context) {
	tool, err := h.svc.Revoke(c.Request.Context(), httpapi.Caller(c), c.Param("uid"), c.Param("name"))
	if err != nil {
		h.fail(c, "revoke MCP source tool", err)
		return
	}
	c.JSON(http.StatusOK, newToolJSON(tool))
}

type answer struct {
	err     error
	status  int
	code    string
	message string
}

var answers = []answer{
	{ErrUIDTaken, http.StatusConflict, "uid_taken", ""},
	{ErrProvisioned, http.StatusConflict, "provisioned", ""},
	{ErrNoSecretKey, http.StatusConflict, "secret_key_missing", ""},
	{ErrSourceChanged, http.StatusConflict, "source_changed", ""},
	{ErrToolChanged, http.StatusConflict, "tool_changed", ""},
	{ErrToolHashRequired, http.StatusBadRequest, "tool_hash_required", ""},
	{ErrTooMany, http.StatusUnprocessableEntity, "too_many_mcp_sources", "at most 50 MCP sources can be added"},
	{ErrAllowlistFull, http.StatusUnprocessableEntity, "too_many_domains", "at most 100 outbound domains can be allowed"},
}

func answerFor(err error) (answer, bool) {
	for _, a := range answers {
		if errors.Is(err, a.err) {
			if a.message == "" {
				a.message = err.Error()
			}
			return a, true
		}
	}
	return answer{}, false
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	var invalidErr *InvalidError
	var refused *RefusedError
	var refusal *lifecycle.Refusal
	var hostErr *egress.HostError
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	if a, ok := answerFor(err); ok {
		httpapi.AbortError(c, a.status, a.code, a.message)
		return
	}
	switch {
	case errors.As(err, &invalidErr):
		fields := invalidErr.Fields
		if fields == nil {
			fields = []config.FieldError{}
		}
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": invalidErr.Error(), "code": "invalid_mcp_source", "fields": fields})
	case errors.As(err, &refusal):
		lifecycle.AbortRefusal(c, refusal)
	case errors.As(err, &refused):
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": refused.Error(), "code": "tool_refused", "refusal": refused.Reason})
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrToolNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, egress.ErrNotAllowed):
		message := "the address is not on the outbound allowlist"
		if errors.As(err, &hostErr) {
			message = hostErr.Host + " is not on the outbound allowlist."
		}
		httpapi.AbortError(c, http.StatusForbidden, "outbound_domain_not_allowed", message)
	default:
		h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}
