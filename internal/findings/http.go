package findings

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	defaultLimit = 20
	maxLimit     = 100

	deprecationValue = "@1794355200"
)

type FindingAuthorJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type FindingAskOnJSON struct {
	DashboardID string `json:"dashboard_id"`
	Title       string `json:"title"`
}

type RootCauseJSON struct {
	Component string `json:"component"`
	Kind      string `json:"kind"`
	Summary   string `json:"summary"`
}

type ComponentJSON struct {
	Kind      string  `json:"kind"`
	Name      string  `json:"name"`
	Namespace *string `json:"namespace"`
}

type TriggerJSON struct {
	Kind  string  `json:"kind"`
	Key   string  `json:"key"`
	Value *string `json:"value"`
}

type CauseEvidenceJSON struct {
	Facts    []string `json:"facts"`
	TraceIDs []string `json:"trace_ids"`
	Queries  []string `json:"queries"`
}

type CauseJSON struct {
	Component  ComponentJSON     `json:"component"`
	Kind       string            `json:"kind"`
	Mechanism  string            `json:"mechanism"`
	Trigger    *TriggerJSON      `json:"trigger"`
	Onset      *string           `json:"onset"`
	Affected   []ComponentJSON   `json:"affected"`
	Relation   string            `json:"relation"`
	KnockOnOf  *int              `json:"knock_on_of"`
	Evidence   CauseEvidenceJSON `json:"evidence"`
	Estimate   *string           `json:"estimate"`
	Confidence *float64          `json:"confidence"`
	Band       *string           `json:"band"`
	Stated     bool              `json:"stated"`
}

func componentJSON(c Component) ComponentJSON {
	return ComponentJSON{Kind: c.Kind, Name: c.Name, Namespace: httpapi.OptionalString(c.Namespace)}
}

func CausesJSON(causes []Cause) []CauseJSON {
	out := make([]CauseJSON, len(causes))
	for i, c := range causes {
		j := CauseJSON{
			Component: componentJSON(c.Component), Kind: c.Kind, Mechanism: c.Mechanism,
			Onset: httpapi.FormatOptionalTime(c.Onset), Affected: []ComponentJSON{}, Relation: c.Relation(), KnockOnOf: c.KnockOnOf,
			Evidence: CauseEvidenceJSON{
				Facts:    append([]string{}, c.Evidence.Facts...),
				TraceIDs: append([]string{}, c.Evidence.TraceIDs...),
				Queries:  append([]string{}, c.Evidence.Queries...),
			},
			Estimate: httpapi.OptionalString(c.Estimate), Confidence: c.Score, Band: httpapi.OptionalString(c.Band), Stated: c.Stated,
		}
		if t := c.Trigger; t != nil {
			j.Trigger = &TriggerJSON{Kind: t.Kind, Key: t.Key, Value: httpapi.OptionalString(t.Value)}
		}
		for _, a := range c.Affected {
			j.Affected = append(j.Affected, componentJSON(a))
		}
		out[i] = j
	}
	return out
}

type StructuredJSON struct {
	RootCause  *RootCauseJSON `json:"root_cause"`
	Causes     []CauseJSON    `json:"causes"`
	Onset      *string        `json:"onset"`
	Impact     *string        `json:"impact"`
	Confidence *string        `json:"confidence"`
	RuledOut   []string       `json:"ruled_out"`
	NextSteps  []string       `json:"next_steps"`
	NoIncident bool           `json:"no_incident"`
}

func StructuredJSONOf(s Structured) StructuredJSON {
	out := StructuredJSON{
		Onset:      httpapi.FormatOptionalTime(s.Onset),
		Impact:     httpapi.OptionalString(s.Impact),
		Confidence: httpapi.OptionalString(s.Confidence),
		RuledOut:   append([]string{}, s.RuledOut...),
		NextSteps:  append([]string{}, s.NextSteps...),
		NoIncident: s.NoIncident,
		Causes:     CausesJSON(s.Causes),
	}
	if s.RootCause != nil {
		out.RootCause = &RootCauseJSON{Component: s.RootCause.Component, Kind: s.RootCause.Kind, Summary: s.RootCause.Summary}
	}
	return out
}

type FindingJSON struct {
	ID             string            `json:"id"`
	ObservationID  *string           `json:"observation_id"`
	AgentRequestID *string           `json:"agent_request_id"`
	DashboardID    *string           `json:"dashboard_id"`
	FolderID       *string           `json:"folder_id"`
	AskOn          *FindingAskOnJSON `json:"ask_on"`
	Title          string            `json:"title"`
	Markdown       string            `json:"markdown"`
	Evidence       []spec.Panel      `json:"evidence"`

	SuggestedChange *string `json:"suggested_change"`
	ObserveWorthy   bool    `json:"observe_worthy"`
	Restricted      bool    `json:"restricted"`

	StructuredJSON
	Unverified []Unverified `json:"unverified"`

	From         string            `json:"from"`
	To           string            `json:"to"`
	AsOf         *string           `json:"as_of"`
	AnnotationID *string           `json:"annotation_id"`
	CreatedBy    FindingAuthorJSON `json:"created_by"`
	Actor        actor.JSON        `json:"actor"`
	CreatedAt    string            `json:"created_at"`
	URL          *string           `json:"url"`
}

type findingFolderJSON struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	General  bool   `json:"general"`
	Personal bool   `json:"personal"`
}

type findingEnvelopeJSON struct {
	Finding     FindingJSON         `json:"finding"`
	Folder      *findingFolderJSON  `json:"folder"`
	Permissions folders.Permissions `json:"permissions"`
}

func envelopeOf(f FindingJSON, folder *folders.Folder, abilities folders.Abilities) findingEnvelopeJSON {
	body := findingEnvelopeJSON{Finding: f, Permissions: folders.PermissionsJSON(abilities)}
	if folder != nil {
		body.Folder = &findingFolderJSON{ID: folder.ID.String(), Title: folder.Title, General: folder.General, Personal: folder.Personal}
	}
	return body
}

type findingListJSON struct {
	Findings   []FindingJSON `json:"findings"`
	NextCursor *string       `json:"next_cursor"`
}

func JSONOf(f Finding, url string) FindingJSON {
	evidence := f.Evidence
	if evidence == nil {
		evidence = []spec.Panel{}
	}
	var askOn *FindingAskOnJSON
	if f.AskOnID != "" {
		askOn = &FindingAskOnJSON{DashboardID: f.AskOnID, Title: f.AskOnTitle}
	}
	return FindingJSON{
		ID:             f.ID,
		ObservationID:  httpapi.OptionalString(f.ObservationID),
		AgentRequestID: httpapi.OptionalString(f.AgentRequestID),
		DashboardID:    httpapi.OptionalString(f.DashboardID),
		FolderID:       httpapi.OptionalString(f.FolderID),
		AskOn:          askOn,
		Title:          f.Title,
		Markdown:       f.Markdown,
		Evidence:       evidence,

		SuggestedChange: httpapi.OptionalString(f.SuggestedChange),
		ObserveWorthy:   f.ObserveWorthy,
		Restricted:      f.Restricted,
		StructuredJSON:  StructuredJSONOf(f.Structured),
		Unverified:      append([]Unverified{}, f.Unverified...),
		From:            httpapi.FormatTime(f.From),
		To:              httpapi.FormatTime(f.To),
		AsOf:            httpapi.FormatOptionalTime(f.AsOf),
		AnnotationID:    httpapi.OptionalString(f.AnnotationID),
		CreatedBy:       FindingAuthorJSON{Kind: f.CreatedBy.Kind, ID: f.CreatedBy.ID, Name: f.CreatedBy.Name},
		Actor:           f.Actor.JSON(),
		CreatedAt:       httpapi.FormatTime(f.CreatedAt),
		URL:             httpapi.OptionalString(url),
	}
}

type Handler struct {
	service    *Service
	findingURL func(f Finding) string
	logger     *slog.Logger
}

func MarkDeprecated(c *gin.Context) {
	c.Header("Deprecation", deprecationValue)
}

func NewHandler(service *Service, findingURL func(f Finding) string, logger *slog.Logger) *Handler {
	return &Handler{service: service, findingURL: findingURL, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/findings", auth.RoleViewer, h.list)
	api.GET("/findings/:id", auth.RoleViewer, h.get)
	api.POST("/findings/:id/dashboard", auth.RoleViewer, h.saveDashboard)
}

func (h *Handler) render(f Finding) FindingJSON {
	return JSONOf(f, h.findingURL(f))
}

func (h *Handler) list(c *gin.Context) {
	MarkDeprecated(c)
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	var filter ListFilter
	for _, param := range []struct {
		name   string
		target *pgtype.UUID
	}{{"dashboard_id", &filter.DashboardID}, {"folder_id", &filter.FolderID}} {
		raw, present := c.GetQuery(param.name)
		if !present {
			continue
		}
		id, err := db.ParseUUID(raw)
		if err != nil {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", param.name+" must be a UUID")
			return
		}
		*param.target = id
	}
	rows, next, err := h.service.List(c.Request.Context(), httpapi.Caller(c), filter, limit, c.Query("cursor"))
	if err != nil {
		h.fail(c, "list findings", err)
		return
	}
	body := findingListJSON{Findings: make([]FindingJSON, len(rows))}
	for i, f := range rows {
		body.Findings[i] = h.render(f)
	}
	if next != "" {
		body.NextCursor = &next
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) get(c *gin.Context) {
	MarkDeprecated(c)
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	f, err := h.service.Read(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "get finding", err)
		return
	}
	if f.Saved() {
		c.Header("Link", "</api/v1/observations/"+f.ObservationID+">; rel=\"successor-version\"")
	}
	c.JSON(http.StatusOK, envelopeOf(h.render(f), f.Folder, f.Abilities))
}

func (h *Handler) saveDashboard(c *gin.Context) {
	MarkDeprecated(c)
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	dash, ver, err := h.service.SaveAsDashboard(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "save finding as dashboard", err)
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+dash.ID)
	c.JSON(http.StatusCreated, dashboards.NewDashboardVersionJSON(dash, ver))
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	var invalid *dashboards.InvalidSpecError
	switch {
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", ErrNotFound.Error())
	case errors.Is(err, ErrFolderNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", ErrFolderNotFound.Error())
	case errors.Is(err, ErrPlacement):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", ErrPlacement.Error())
	case errors.Is(err, ErrInvalidCursor):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "cursor is not valid")
	case errors.Is(err, ErrNotSaved):
		httpapi.AbortError(c, http.StatusConflict, "finding_not_saved", err.Error())
	case errors.Is(err, ErrRequesterOnly):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.AccessMessage)
	case errors.Is(err, ErrNoEvidence):
		httpapi.AbortError(c, http.StatusConflict, "finding_has_no_evidence", err.Error())
	case errors.Is(err, annotations.ErrForbidden), errors.Is(err, folders.ErrForbidden):
		c.AbortWithStatusJSON(http.StatusForbidden, forbidden.BodyOf(err))
	case errors.Is(err, folders.ErrNoAccess):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "dashboard not found")
	case errors.As(err, &invalid):
		httpapi.AbortInvalidSpec(c, "spec is invalid", invalid.Issues)
	case folders.AbortRestricted(c, err):
	default:
		principal := httpapi.Caller(c)
		attrs := []any{slog.String("principal_kind", string(principal.Kind)), slog.String("principal_id", principal.ID)}
		if id, parseErr := db.ParseUUID(c.Param("id")); parseErr == nil {
			attrs = append(attrs, slog.String("finding_id", id.String()))
		}
		h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, attrs...)...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}
