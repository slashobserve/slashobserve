package observations

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type RCAJSON struct {
	ID               string     `json:"id"`
	ObservationID    string     `json:"observation_id"`
	Revision         int        `json:"revision"`
	Title            string     `json:"title"`
	Summary          *string    `json:"summary"`
	ObservationState string     `json:"observation_state"`
	From             *string    `json:"from"`
	To               *string    `json:"to"`
	Blocks           []RCABlock `json:"blocks"`
	CreatedBy        actor.JSON `json:"created_by"`
	CreatedAt        string     `json:"created_at"`
}

type RCASummaryJSON struct {
	ID               string     `json:"id"`
	Revision         int        `json:"revision"`
	Title            string     `json:"title"`
	Summary          *string    `json:"summary"`
	ObservationState string     `json:"observation_state"`
	CreatedBy        actor.JSON `json:"created_by"`
	CreatedAt        string     `json:"created_at"`
}

type rcaReadJSON struct {
	RCAJSON
	State          string   `json:"state"`
	AllowedActions []string `json:"allowed_actions"`
}

type rcaSummaryReadJSON struct {
	RCASummaryJSON
	State          string   `json:"state"`
	AllowedActions []string `json:"allowed_actions"`
}

type rcaEnvelopeJSON struct {
	RCA rcaReadJSON `json:"rca"`
}

type rcaListJSON struct {
	RCAs []rcaSummaryReadJSON `json:"rcas"`
}

type rcaRefusalJSON struct {
	Error          string   `json:"error"`
	Code           string   `json:"code"`
	State          string   `json:"state"`
	AllowedActions []string `json:"allowed_actions"`
}

func rcaReadOf(r RCA) rcaReadJSON {
	return rcaReadJSON{RCAJSON: RCAOf(r), State: string(r.RevisionState), AllowedActions: named(r.AllowedActions)}
}

func RCAOf(r RCA) RCAJSON {
	return RCAJSON{
		ID:               r.ID,
		ObservationID:    r.ObservationID,
		Revision:         r.Revision,
		Title:            r.Title,
		Summary:          httpapi.OptionalString(r.Summary),
		ObservationState: string(r.State),
		From:             timeString(r.From),
		To:               timeString(r.To),
		Blocks:           r.Blocks,
		CreatedBy:        r.Actor.JSON(),
		CreatedAt:        httpapi.FormatTime(r.CreatedAt),
	}
}

func RCASummaryOf(r RCASummary) RCASummaryJSON {
	return RCASummaryJSON{
		ID:               r.ID,
		Revision:         r.Revision,
		Title:            r.Title,
		Summary:          httpapi.OptionalString(r.Summary),
		ObservationState: string(r.State),
		CreatedBy:        r.Actor.JSON(),
		CreatedAt:        httpapi.FormatTime(r.CreatedAt),
	}
}

type RCAHandler struct {
	service *Service
	base    *Handler
}

func NewRCAHandler(service *Service, logger *slog.Logger) *RCAHandler {
	return &RCAHandler{service: service, base: NewHandler(service, logger)}
}

func (h *RCAHandler) Register(api auth.Routes) {
	api.POST("/observations/:id/rcas", auth.RoleViewer, h.publish)
	api.GET("/observations/:id/rcas", auth.RoleViewer, h.list)
	api.GET("/observations/:id/rcas/:revision", auth.RoleViewer, h.get)
	api.DELETE("/observations/:id/rcas/:revision", auth.RoleViewer, h.withdraw)
}

func (h *RCAHandler) publish(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	rca, err := h.service.Publish(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "publish observation", err)
		return
	}
	c.JSON(http.StatusCreated, rcaEnvelopeJSON{RCA: rcaReadOf(rca)})
}

func (h *RCAHandler) list(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	rcas, err := h.service.ListRCAs(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "list observation RCAs", err)
		return
	}
	body := rcaListJSON{RCAs: make([]rcaSummaryReadJSON, len(rcas))}
	for i, r := range rcas {
		body.RCAs[i] = rcaSummaryReadJSON{RCASummaryJSON: RCASummaryOf(r), State: string(r.RevisionState), AllowedActions: named(r.AllowedActions)}
	}
	c.JSON(http.StatusOK, body)
}

func RevisionParam(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Param("revision"))
	if err != nil || n < 1 {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "revision must be a positive integer")
		return 0, false
	}
	return n, true
}

func (h *RCAHandler) get(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	revision, ok := RevisionParam(c)
	if !ok {
		return
	}
	rca, err := h.service.GetRCA(c.Request.Context(), httpapi.Caller(c), id, revision)
	if err != nil {
		h.fail(c, "get observation RCA", err)
		return
	}
	c.JSON(http.StatusOK, rcaEnvelopeJSON{RCA: rcaReadOf(rca)})
}

func (h *RCAHandler) withdraw(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	revision, ok := RevisionParam(c)
	if !ok {
		return
	}
	if err := h.service.WithdrawRCA(c.Request.Context(), httpapi.Caller(c), id, revision); err != nil {
		h.fail(c, "withdraw observation RCA", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *RCAHandler) fail(c *gin.Context, action string, err error) {
	var failed *PanelFailedError
	var unchanged *RCAUnchangedError
	var refusal *RCARefusal
	switch {
	case errors.As(err, &refusal):
		c.AbortWithStatusJSON(http.StatusConflict, rcaRefusalJSON{Error: refusal.Error(), Code: refusal.Code, State: refusal.State, AllowedActions: refusal.Allowed})
	case errors.As(err, &unchanged):
		httpapi.AbortError(c, http.StatusConflict, CodeRCAUnchanged, unchanged.Error())
	case errors.Is(err, ErrRevisionNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", ErrRevisionNotFound.Error())
	case errors.Is(err, ErrRCALimit):
		httpapi.AbortError(c, http.StatusConflict, CodeRCALimit, ErrRCALimit.Error())
	case errors.Is(err, ErrRCAShared):
		httpapi.AbortError(c, http.StatusConflict, CodeRCAShared, ErrRCAShared.Error())
	case errors.Is(err, ErrObservationChanged):
		httpapi.AbortError(c, http.StatusConflict, CodeObservationChanged, ErrObservationChanged.Error())
	case errors.Is(err, ErrPublishPersonal):
		httpapi.AbortError(c, http.StatusConflict, "personal_folder", ErrPublishPersonal.Error())
	case errors.As(err, &failed):
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, httpapi.InvalidSpecBody{Error: failed.Error(), Code: CodePanelFailed, Issues: []spec.Issue{failed.Issue()}})
	default:
		h.base.fail(c, action, err)
	}
}
