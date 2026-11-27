package datasourceadmin

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const maxBodyBytes = 64 << 10

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/datasources")
	g.POST("", auth.RoleAdmin, h.create)
	g.POST("/test", auth.RoleAdmin, h.test)
	g.GET("/:uid/config", auth.RoleAdmin, h.get)
	g.PATCH("/:uid", auth.RoleAdmin, h.update)
	g.DELETE("/:uid", auth.RoleAdmin, h.remove)
}

type writeBody struct {
	Definition
	Secrets      Secrets  `json:"secrets"`
	ClearSecrets []string `json:"clear_secrets"`
}

func (b writeBody) change() SecretsChange {
	return SecretsChange{Set: b.Secrets, Clear: b.ClearSecrets}
}

type recordJSON struct {
	Definition
	Source     string   `json:"source"`
	SecretsSet []string `json:"secrets_set"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

func newRecordJSON(r Record) recordJSON {
	set := r.SecretsSet
	if set == nil {
		set = []string{}
	}
	return recordJSON{Definition: r.Definition, Source: "app", SecretsSet: set, CreatedAt: httpapi.FormatTime(r.CreatedAt), UpdatedAt: httpapi.FormatTime(r.UpdatedAt)}
}

type testJSON struct {
	Healthy     bool                   `json:"healthy"`
	Message     string                 `json:"message,omitempty"`
	Detail      string                 `json:"detail,omitempty"`
	FailureKind datasource.FailureKind `json:"failure_kind,omitempty"`
}

type userJSON struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (h *Handler) get(c *gin.Context) {
	record, err := h.svc.Get(c.Request.Context(), c.Param("uid"))
	if err != nil {
		h.fail(c, "read datasource", err)
		return
	}
	c.JSON(http.StatusOK, newRecordJSON(record))
}

func (h *Handler) create(c *gin.Context) {
	var body writeBody
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	record, err := h.svc.Create(c.Request.Context(), httpapi.Caller(c), body.Definition, body.Secrets)
	if err != nil {
		h.fail(c, "add datasource", err)
		return
	}
	c.JSON(http.StatusCreated, newRecordJSON(record))
}

func (h *Handler) update(c *gin.Context) {
	uid := c.Param("uid")
	current, err := h.svc.Get(c.Request.Context(), uid)
	if err != nil {
		h.fail(c, "change datasource", err)
		return
	}
	body := writeBody{Definition: current.Definition}
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	if err := checkAudience(body.Audience); err != nil {
		h.fail(c, "change datasource", err)
		return
	}
	record, err := h.svc.Update(c.Request.Context(), httpapi.Caller(c), uid, body.Definition, body.change())
	if err != nil {
		h.fail(c, "change datasource", err)
		return
	}
	c.JSON(http.StatusOK, newRecordJSON(record))
}

func (h *Handler) remove(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), httpapi.Caller(c), c.Param("uid")); err != nil {
		h.fail(c, "remove datasource", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) test(c *gin.Context) {
	baseUID, ok := httpapi.NonEmptyQuery(c, "base")
	if !ok {
		return
	}
	var body writeBody
	if baseUID != "" {
		base, err := h.svc.Get(c.Request.Context(), baseUID)
		if err != nil {
			h.fail(c, "test datasource", err)
			return
		}
		body.Definition = base.Definition
	}
	if !httpapi.DecodeBody(c, &body, maxBodyBytes) {
		return
	}
	result, err := h.svc.Test(c.Request.Context(), baseUID, body.Definition, body.change())
	if err != nil {
		h.fail(c, "test datasource", err)
		return
	}
	audit.Conclude(c.Request.Context(), testRecord(baseUID, body.Definition, result))
	c.JSON(http.StatusOK, testJSON(result))
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	var invalidErr *InvalidError
	var inUse *InUseError
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	if abortRefusedChange(c, err) {
		return
	}
	switch {
	case errors.As(err, &invalidErr):
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": invalidErr.Error(), "code": "invalid_datasource", "fields": invalidErr.Fields})
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrReadOnly), errors.Is(err, ErrBuiltIn):
		httpapi.AbortError(c, http.StatusConflict, "datasource_read_only", err.Error())
	case errors.Is(err, ErrUIDTaken):
		httpapi.AbortError(c, http.StatusConflict, "uid_taken", err.Error())
	case errors.Is(err, ErrNoSecretKey):
		httpapi.AbortError(c, http.StatusConflict, "secret_key_missing", err.Error())
	case errors.As(err, &inUse):
		h.failInUse(c, inUse)
	case folders.AbortRestricted(c, err):
	case errors.Is(err, ErrTooMany):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "too_many_datasources", "at most 100 datasources can be added in the app")
	case errors.Is(err, ErrAllowlistFull):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "too_many_domains", "at most 100 outbound domains can be allowed")
	case errors.Is(err, ErrAddressRefused):
		httpapi.AbortError(c, http.StatusForbidden, "outbound_domain_not_allowed", err.Error())
	case errors.Is(err, egress.ErrNotAllowed):
		var hostErr *egress.HostError
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

func abortRefusedChange(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, ErrIdentityChanged):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "invalid_datasource", err.Error())
	case errors.Is(err, ErrSecretRequired):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "secret_required", err.Error())
	case errors.Is(err, ErrChanged):
		httpapi.AbortError(c, http.StatusConflict, "datasource_changed", err.Error())
	default:
		return false
	}
	return true
}

func (h *Handler) failInUse(c *gin.Context, inUse *InUseError) {
	used := make([]userJSON, len(inUse.Users))
	for i, u := range inUse.Users {
		used[i] = userJSON(u)
	}
	c.AbortWithStatusJSON(http.StatusConflict, gin.H{
		"error":   "This datasource is used by dashboards or saved searches; change them to another datasource first.",
		"code":    "datasource_in_use",
		"used_by": used,
		"hidden":  inUse.Hidden,
	})
}
