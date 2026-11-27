package observations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/usertext"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	defaultLimit = 20
	maxLimit     = 100
	maxOffset    = 100000

	maxBodyBytes            = 128 << 10
	moveIntoPersonalMessage = "an observation cannot move into My Dashboards"
	codeVersionConflict     = "version_conflict"
)

type ObservationJSON struct {
	ID             string           `json:"id"`
	FolderID       string           `json:"folder_id"`
	Title          string           `json:"title"`
	State          string           `json:"state"`
	Summary        *string          `json:"summary"`
	From           *string          `json:"from"`
	To             *string          `json:"to"`
	ThreadID       *string          `json:"thread_id"`
	RunID          *string          `json:"run_id"`
	DashboardID    *string          `json:"dashboard_id"`
	Pinned         bool             `json:"pinned"`
	CanPin         bool             `json:"can_pin"`
	MessageCount   int              `json:"message_count"`
	URL            string           `json:"url"`
	CreatedBy      actor.JSON       `json:"created_by"`
	CreatedAt      string           `json:"created_at"`
	UpdatedAt      string           `json:"updated_at"`
	ResolvedAt     *string          `json:"resolved_at"`
	AllowedActions []string         `json:"allowed_actions"`
	Blocks         *[]BlockJSON     `json:"blocks,omitempty"`
	Findings       []RunFindingJSON `json:"findings,omitempty"`

	ChatRename revisions.SharedTitle `json:"chat_rename,omitempty"`
}

type RunFindingJSON struct {
	RunID string `json:"run_id"`
	findings.StructuredJSON
	Unverified []findings.Unverified `json:"unverified"`
}

type BlockJSON struct {
	ID             string          `json:"id"`
	ObservationID  string          `json:"observation_id"`
	Kind           string          `json:"kind"`
	State          string          `json:"state"`
	Position       int             `json:"position"`
	Width          string          `json:"width"`
	Height         *int            `json:"height"`
	Version        int             `json:"version"`
	Body           json.RawMessage `json:"body"`
	SnapshotID     *string         `json:"snapshot_id"`
	RunID          *string         `json:"run_id"`
	CreatedBy      actor.JSON      `json:"created_by"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
	AllowedActions []string        `json:"allowed_actions"`
}

type SnapshotJSON struct {
	ID            string          `json:"id"`
	ObservationID string          `json:"observation_id"`
	Panel         json.RawMessage `json:"panel"`
	Query         json.RawMessage `json:"query"`
	Data          json.RawMessage `json:"data"`
	UserScoped    bool            `json:"user_scoped"`
	TimeFrom      string          `json:"time_from"`
	TimeTo        string          `json:"time_to"`
	CreatedBy     actor.JSON      `json:"created_by"`
	CreatedAt     string          `json:"created_at"`
}

type folderJSON struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	General  bool   `json:"general"`
	Personal bool   `json:"personal"`
}

type envelopeJSON struct {
	Observation ObservationJSON     `json:"observation"`
	Folder      folderJSON          `json:"folder"`
	Permissions folders.Permissions `json:"permissions"`
}

type listJSON struct {
	Observations []ObservationJSON `json:"observations"`
	NextCursor   *string           `json:"next_cursor"`
	Total        *int              `json:"total"`
}

type blockEnvelopeJSON struct {
	Block BlockJSON `json:"block"`
}

type conflictJSON struct {
	Error   string    `json:"error"`
	Code    string    `json:"code"`
	Current BlockJSON `json:"current"`
}

func timeString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	return httpapi.OptionalString(httpapi.FormatTime(*t))
}

func actions[A ~string](in []A) []string {
	out := make([]string, len(in))
	for i, a := range in {
		out[i] = string(a)
	}
	return out
}

func ObservationOf(o Observation, url string) ObservationJSON {
	body := ObservationJSON{
		ID:             o.ID,
		FolderID:       o.FolderID,
		Title:          o.Title,
		State:          string(o.State),
		Summary:        httpapi.OptionalString(o.Summary),
		From:           timeString(o.From),
		To:             timeString(o.To),
		ThreadID:       httpapi.OptionalString(o.ThreadID),
		RunID:          httpapi.OptionalString(o.RunID),
		DashboardID:    httpapi.OptionalString(o.DashboardID),
		Pinned:         o.Pinned,
		CanPin:         o.CanPin,
		MessageCount:   o.MessageCount,
		URL:            url,
		CreatedBy:      o.Actor.JSON(),
		CreatedAt:      httpapi.FormatTime(o.CreatedAt),
		UpdatedAt:      httpapi.FormatTime(o.UpdatedAt),
		ResolvedAt:     timeString(o.ResolvedAt),
		AllowedActions: actions(o.AllowedActions),
		ChatRename:     o.ChatRename,
	}
	for _, f := range o.Findings {
		body.Findings = append(body.Findings, RunFindingJSON{RunID: f.RunID, StructuredJSON: findings.StructuredJSONOf(f.Fields), Unverified: append([]findings.Unverified{}, f.Unverified...)})
	}
	if o.Blocks != nil {
		blocks := make([]BlockJSON, len(o.Blocks))
		for i, b := range o.Blocks {
			blocks[i] = BlockOf(b)
		}
		body.Blocks = &blocks
	}
	return body
}

func BlockOf(b Block) BlockJSON {
	return BlockJSON{
		ID:             b.ID,
		ObservationID:  b.ObservationID,
		Kind:           string(b.Kind),
		State:          string(b.State),
		Position:       b.Position,
		Width:          string(b.Width),
		Height:         b.Height,
		Version:        b.Version,
		Body:           b.Body,
		SnapshotID:     httpapi.OptionalString(b.SnapshotID),
		RunID:          httpapi.OptionalString(b.RunID),
		CreatedBy:      b.Actor.JSON(),
		CreatedAt:      httpapi.FormatTime(b.CreatedAt),
		UpdatedAt:      httpapi.FormatTime(b.UpdatedAt),
		AllowedActions: actions(b.AllowedActions),
	}
}

func SnapshotOf(s Snapshot) SnapshotJSON {
	return SnapshotJSON{
		ID:            s.ID,
		ObservationID: s.ObservationID,
		Panel:         s.Panel,
		Query:         s.Query,
		Data:          s.Data,
		UserScoped:    s.UserScoped,
		TimeFrom:      httpapi.FormatTime(s.From),
		TimeTo:        httpapi.FormatTime(s.To),
		CreatedBy:     s.Actor.JSON(),
		CreatedAt:     httpapi.FormatTime(s.CreatedAt),
	}
}

type PanelChecker interface {
	CheckPanel(ctx context.Context, by auth.Principal, body PanelBody, fallbackFrom, fallbackTo *time.Time) error
}

type Handler struct {
	service *Service
	logger  *slog.Logger
	panels  PanelChecker
}

type HandlerOption func(*Handler)

func WithPanelChecker(checker PanelChecker) HandlerOption {
	return func(h *Handler) { h.panels = checker }
}

func NewHandler(service *Service, logger *slog.Logger, options ...HandlerOption) *Handler {
	h := &Handler{service: service, logger: logger}
	for _, option := range options {
		option(h)
	}
	return h
}

func (h *Handler) panelRuns(c *gin.Context, raw json.RawMessage, window func(by auth.Principal) (*time.Time, *time.Time, error)) bool {
	by := httpapi.Caller(c)
	if h.panels == nil || by.Kind != auth.KindServiceAccount {
		return true
	}
	var body PanelBody
	if json.Unmarshal(raw, &body) != nil || len(body.Panel) == 0 {
		return true
	}
	from, to, err := window(by)
	if errors.Is(err, ErrNotFound) {
		return true
	}
	if err == nil {
		err = h.panels.CheckPanel(c.Request.Context(), by, body, from, to)
	}
	if err != nil {
		h.fail(c, "check observation panel", err)
		return false
	}
	return true
}

func (h *Handler) Register(api auth.Routes) {
	api.POST("/observations", auth.RoleViewer, h.create)
	api.GET("/observations", auth.RoleViewer, h.list)
	api.GET("/observations/:id", auth.RoleViewer, h.get)
	api.PATCH("/observations/:id", auth.RoleViewer, h.update)
	api.POST("/observations/:id/move", auth.RoleViewer, h.move)
	api.DELETE("/observations/:id", auth.RoleViewer, h.remove)
	api.POST("/observations/:id/resolve", auth.RoleViewer, h.resolve)
	api.POST("/observations/:id/reopen", auth.RoleViewer, h.reopen)
	api.POST("/observations/:id/pin", auth.RoleViewer, h.pin)
	api.POST("/observations/:id/dashboard", auth.RoleViewer, h.makeObservationDashboard)
	api.POST("/blocks/:id/dashboard", auth.RoleViewer, h.makeBlockDashboard)
	api.POST("/observations/:id/blocks", auth.RoleViewer, h.addBlock)
	api.PUT("/observations/:id/layout", auth.RoleViewer, h.arrange)
	api.PATCH("/blocks/:id", auth.RoleViewer, h.updateBlock)
	api.DELETE("/blocks/:id", auth.RoleViewer, h.removeBlock)
	api.POST("/blocks/:id/keep", auth.RoleViewer, h.keepBlock)
	api.POST("/blocks/:id/discard", auth.RoleViewer, h.discardBlock)
	api.GET("/snapshots/:id", auth.RoleViewer, h.snapshot)
}

func (h *Handler) render(o Observation) ObservationJSON {
	return ObservationOf(o, h.service.URL(o.ID))
}

func (h *Handler) envelope(o Observation) envelopeJSON {
	return envelopeJSON{
		Observation: h.render(o),
		Folder:      folderJSON{ID: o.Folder.ID.String(), Title: o.Folder.Title, General: o.Folder.General, Personal: o.Folder.Personal},
		Permissions: folders.PermissionsJSON(o.Abilities),
	}
}

type blockRequest struct {
	Kind        string          `json:"kind"`
	Body        json.RawMessage `json:"body"`
	Width       string          `json:"width"`
	Height      *int            `json:"height"`
	Position    *int            `json:"position"`
	FromBlockID *string         `json:"from_block_id"`
}

type firstBlockRequest struct {
	Kind  string          `json:"kind"`
	Body  json.RawMessage `json:"body"`
	Width string          `json:"width"`
}

type createRequest struct {
	Title      string             `json:"title"`
	FolderID   *string            `json:"folder_id"`
	From       *time.Time         `json:"from"`
	To         *time.Time         `json:"to"`
	FirstBlock *firstBlockRequest `json:"first_block"`
}

func optionalUUID(c *gin.Context, name string, raw *string) (pgtype.UUID, bool) {
	if raw == nil {
		return pgtype.UUID{}, true
	}
	id, err := db.ParseUUID(*raw)
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", name+" must be a UUID")
		return pgtype.UUID{}, false
	}
	return id, true
}

func (h *Handler) create(c *gin.Context) {
	var req createRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	folderID, ok := optionalUUID(c, "folder_id", req.FolderID)
	if !ok {
		return
	}
	in := CreateInput{FolderID: folderID, Title: req.Title, From: req.From, To: req.To}
	if req.FirstBlock != nil {
		in.Blocks = []BlockInput{{Kind: Kind(req.FirstBlock.Kind), Body: req.FirstBlock.Body, Width: Width(req.FirstBlock.Width)}}
	}
	o, err := h.service.Create(c.Request.Context(), httpapi.Caller(c), in)
	if err != nil {
		h.fail(c, "create observation", err)
		return
	}
	c.Header("Location", "/api/v1/observations/"+o.ID)
	c.JSON(http.StatusCreated, h.envelope(o))
}

const maxQueryRunes = 200

func (h *Handler) list(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	filter, ok := listFilter(c)
	if !ok {
		return
	}
	if raw, present := c.GetQuery("offset"); present {
		if c.Query("cursor") != "" {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "cursor and offset cannot be used together")
			return
		}
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > maxOffset {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "offset must be an integer from 0 to "+strconv.Itoa(maxOffset))
			return
		}
		listing, err := h.service.Page(c.Request.Context(), httpapi.Caller(c), filter, limit, offset)
		if err != nil {
			h.fail(c, "list observations", err)
			return
		}
		body := h.listBody(listing.Observations)
		body.Total = &listing.Total
		c.JSON(http.StatusOK, body)
		return
	}
	rows, next, err := h.service.List(c.Request.Context(), httpapi.Caller(c), filter, limit, c.Query("cursor"))
	if err != nil {
		h.fail(c, "list observations", err)
		return
	}
	body := h.listBody(rows)
	if next != "" {
		body.NextCursor = &next
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) listBody(rows []Observation) listJSON {
	body := listJSON{Observations: make([]ObservationJSON, len(rows))}
	for i, o := range rows {
		body.Observations[i] = h.render(o)
	}
	return body
}

func listFilter(c *gin.Context) (ListFilter, bool) {
	var filter ListFilter
	if raw, present := c.GetQuery("folder_id"); present {
		var ok bool
		if filter.FolderID, ok = optionalUUID(c, "folder_id", &raw); !ok {
			return filter, false
		}
	}
	if raw, present := c.GetQuery("state"); present {
		if filter.State = State(raw); filter.State != StateOpen && filter.State != StateResolved {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "state must be open or resolved")
			return filter, false
		}
	}
	filter.Query = c.Query("q")
	if utf8.RuneCountInString(filter.Query) > maxQueryRunes {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "q must be at most 200 characters")
		return filter, false
	}
	switch raw := c.DefaultQuery("created_by", ""); raw {
	case "":
	case "me":
		filter.CreatedByMe = true
	default:
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "created_by must be me")
		return filter, false
	}
	return filter, listOrder(c, &filter)
}

func listOrder(c *gin.Context, filter *ListFilter) bool {
	filter.Sort = SortUpdated
	if raw, present := c.GetQuery("sort"); present {
		if filter.Sort = Sort(raw); filter.Sort != SortUpdated && filter.Sort != SortCreated && filter.Sort != SortTitle {
			httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "sort must be updated, created or title")
			return false
		}
	}
	filter.Ascending = filter.Sort == SortTitle
	switch raw := c.Query("order"); raw {
	case "":
	case "asc":
		filter.Ascending = true
	case "desc":
		filter.Ascending = false
	default:
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "order must be asc or desc")
		return false
	}
	return true
}

func (h *Handler) get(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	o, err := h.service.Get(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "get observation", err)
		return
	}
	audit.Conclude(c.Request.Context(), audit.Result{
		Outcome: audit.OutcomeSuccess,
		Target:  audit.Target{Kind: "observation", ID: c.Param("id")},
		Data:    map[string]any{"updated_at": o.UpdatedAt.UTC().Format(time.RFC3339Nano)},
	})
	c.JSON(http.StatusOK, h.envelope(o))
}

type updateRequest struct {
	Title *string         `json:"title"`
	From  json.RawMessage `json:"from"`
	To    json.RawMessage `json:"to"`
}

func isNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func timeField(c *gin.Context, name string, raw json.RawMessage) (*time.Time, bool) {
	if len(raw) == 0 || isNull(raw) {
		return nil, true
	}
	var t time.Time
	if err := json.Unmarshal(raw, &t); err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", name+" must be an RFC 3339 time or null")
		return nil, false
	}
	return &t, true
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req updateRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	from, ok := timeField(c, "from", req.From)
	if !ok {
		return
	}
	to, ok := timeField(c, "to", req.To)
	if !ok {
		return
	}
	in := UpdateInput{Title: req.Title, From: from, To: to}
	if cleared := isNull(req.From) && isNull(req.To); cleared {
		in.ClearRange = true
	} else if isNull(req.From) || isNull(req.To) {
		h.fail(c, "update observation", invalid(CodeInvalidRange, "from and to are cleared together"))
		return
	}
	o, err := h.service.Update(c.Request.Context(), httpapi.Caller(c), id, in)
	if err != nil {
		h.fail(c, "update observation", err)
		return
	}
	c.JSON(http.StatusOK, h.envelope(o))
}

type moveRequest struct {
	FolderID string `json:"folder_id"`
}

func (h *Handler) move(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req moveRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	folderID, ok := optionalUUID(c, "folder_id", &req.FolderID)
	if !ok {
		return
	}
	o, err := h.service.Move(c.Request.Context(), httpapi.Caller(c), id, folderID)
	if err != nil {
		h.fail(c, "move observation", err)
		return
	}
	c.JSON(http.StatusOK, h.envelope(o))
}

type resolveRequest struct {
	Summary string `json:"summary"`
}

func (h *Handler) remove(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), httpapi.Caller(c), id); err != nil {
		h.fail(c, "delete observation", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) resolve(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req resolveRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	o, err := h.service.Resolve(c.Request.Context(), httpapi.Caller(c), id, req.Summary)
	if err != nil {
		h.fail(c, "resolve observation", err)
		return
	}
	c.JSON(http.StatusOK, h.envelope(o))
}

func (h *Handler) reopen(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	o, err := h.service.Reopen(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "reopen observation", err)
		return
	}
	c.JSON(http.StatusOK, h.envelope(o))
}

type PinJSON struct {
	DashboardID  string `json:"dashboard_id"`
	AnnotationID string `json:"annotation_id"`
}

type pinRequest struct {
	DashboardID *string `json:"dashboard_id"`
}

type makeDashboardRequest struct {
	New bool `json:"new"`
}

func (h *Handler) pin(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req pinRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	dashboardID, ok := optionalUUID(c, "dashboard_id", req.DashboardID)
	if !ok {
		return
	}
	pin, created, err := h.service.Pin(c.Request.Context(), httpapi.Caller(c), id, dashboardID)
	if err != nil {
		h.fail(c, "pin observation", err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		c.Header("Location", "/api/v1/annotations/"+pin.AnnotationID)
	}
	c.JSON(status, PinJSON(pin))
}

func (h *Handler) makeObservationDashboard(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	h.makeDashboard(c, id, pgtype.UUID{})
}

func (h *Handler) makeBlockDashboard(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	h.makeDashboard(c, pgtype.UUID{}, id)
}

func (h *Handler) makeDashboard(c *gin.Context, observationID, blockID pgtype.UUID) {
	var req makeDashboardRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	d, v, created, err := h.service.MakeDashboard(c.Request.Context(), httpapi.Caller(c), observationID, blockID, req.New)
	if err != nil {
		h.fail(c, "make dashboard from observation", err)
		return
	}
	if !created {
		c.JSON(http.StatusOK, dashboards.NewDashboardVersionJSON(d, v))
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+d.ID)
	c.JSON(http.StatusCreated, dashboards.NewDashboardVersionJSON(d, v))
}

func (h *Handler) addBlock(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req blockRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	from, ok := optionalUUID(c, "from_block_id", req.FromBlockID)
	if !ok {
		return
	}
	if Kind(req.Kind) == KindPanel && !h.panelRuns(c, req.Body, func(by auth.Principal) (*time.Time, *time.Time, error) {
		o, err := h.service.Get(c.Request.Context(), by, id)
		return o.From, o.To, err
	}) {
		return
	}
	b, err := h.service.AddBlock(c.Request.Context(), httpapi.Caller(c), id, BlockInput{
		Kind: Kind(req.Kind), Body: req.Body, Width: Width(req.Width), Height: req.Height, Position: req.Position, FromBlockID: from,
	})
	if err != nil {
		h.fail(c, "add observation block", err)
		return
	}
	c.JSON(http.StatusCreated, blockEnvelopeJSON{Block: BlockOf(b)})
}

type updateBlockRequest struct {
	Version    *int            `json:"version"`
	Body       json.RawMessage `json:"body"`
	Width      *string         `json:"width"`
	Height     HeightChange    `json:"height"`
	Position   *int            `json:"position"`
	FollowPage bool            `json:"follow_page"`
}

func (h *Handler) updateBlock(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req updateBlockRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	if req.Version == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "version is required")
		return
	}
	if !h.panelRuns(c, req.Body, func(by auth.Principal) (*time.Time, *time.Time, error) {
		_, from, to, err := h.service.PanelDashboard(c.Request.Context(), by, id)
		return &from, &to, err
	}) {
		return
	}
	change := BlockChange{Version: *req.Version, Body: req.Body, Height: req.Height, Position: req.Position, FollowPage: req.FollowPage}
	if req.Width != nil {
		width := Width(*req.Width)
		change.Width = &width
	}
	b, err := h.service.UpdateBlock(c.Request.Context(), httpapi.Caller(c), id, change)
	if err != nil {
		h.fail(c, "update observation block", err)
		return
	}
	c.JSON(http.StatusOK, blockEnvelopeJSON{Block: BlockOf(b)})
}

type layoutItemRequest struct {
	ID     string       `json:"id"`
	Width  *string      `json:"width"`
	Height HeightChange `json:"height"`
}

type layoutRequest struct {
	Blocks []layoutItemRequest `json:"blocks"`
}

type layoutJSON struct {
	Blocks []BlockJSON `json:"blocks"`
}

func (h *Handler) arrange(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req layoutRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	items := make([]BlockLayout, len(req.Blocks))
	for i, item := range req.Blocks {
		blockID, ok := optionalUUID(c, "blocks.id", &item.ID)
		if !ok {
			return
		}
		items[i] = BlockLayout{ID: blockID, Height: item.Height}
		if item.Width != nil {
			width := Width(*item.Width)
			items[i].Width = &width
		}
	}
	blocks, err := h.service.Arrange(c.Request.Context(), httpapi.Caller(c), id, items, pgtype.UUID{})
	if err != nil {
		h.fail(c, "arrange observation blocks", err)
		return
	}
	body := layoutJSON{Blocks: make([]BlockJSON, len(blocks))}
	for i, b := range blocks {
		body.Blocks[i] = BlockOf(b)
	}
	c.JSON(http.StatusOK, body)
}

func versionParam(c *gin.Context) (int, bool) {
	n, err := strconv.Atoi(c.Query("version"))
	if err != nil || n < 1 {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "version must be a positive integer")
		return 0, false
	}
	return n, true
}

func (h *Handler) removeBlock(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	version, ok := versionParam(c)
	if !ok {
		return
	}
	if err := h.service.RemoveBlock(c.Request.Context(), httpapi.Caller(c), id, version, pgtype.UUID{}); err != nil {
		h.fail(c, "remove observation block", err)
		return
	}
	c.Status(http.StatusNoContent)
}

type keepRequest struct {
	Version *int `json:"version"`
}

func (h *Handler) keepBlock(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req keepRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	if req.Version == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "version is required")
		return
	}
	b, err := h.service.KeepBlock(c.Request.Context(), httpapi.Caller(c), id, *req.Version, pgtype.UUID{})
	if err != nil {
		h.fail(c, "keep observation block", err)
		return
	}
	c.JSON(http.StatusOK, blockEnvelopeJSON{Block: BlockOf(b)})
}

func (h *Handler) discardBlock(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.DiscardBlock(c.Request.Context(), httpapi.Caller(c), id, pgtype.UUID{}); err != nil {
		h.fail(c, "discard observation block", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) snapshot(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	s, err := h.service.Snapshot(c.Request.Context(), httpapi.Caller(c), id)
	if err != nil {
		h.fail(c, "get snapshot", err)
		return
	}
	c.JSON(http.StatusOK, SnapshotOf(s))
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) || abortKnown(c, err) {
		return
	}
	if status, code, message := queryproxy.Classify(err); queryFailure(status) {
		if status == http.StatusTooManyRequests {
			c.Header("Retry-After", "1")
		}
		httpapi.AbortError(c, status, code, message)
		return
	}
	principal := httpapi.Caller(c)
	attrs := []any{slog.String("principal_kind", string(principal.Kind)), slog.String("principal_id", principal.ID)}
	if id, parseErr := db.ParseUUID(c.Param("id")); parseErr == nil {
		attrs = append(attrs, slog.String("id", id.String()))
	}
	h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, attrs...)...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}

type knownFailure struct {
	err     error
	status  int
	code    string
	message string
}

var knownFailures = []knownFailure{
	{ErrNoDashboard, http.StatusConflict, "observation_has_no_dashboard", ErrNoDashboard.Error()},
	{ErrDashboardNotFound, http.StatusNotFound, "not_found", ErrDashboardNotFound.Error()},
	{ErrNoPanels, http.StatusConflict, "observation_has_no_panels", ErrNoPanels.Error()},
	{ErrRunInProgress, http.StatusConflict, "run_in_progress", ErrRunInProgress.Error()},
	{ErrNotFound, http.StatusNotFound, "not_found", ErrNotFound.Error()},
	{ErrFolderNotFound, http.StatusNotFound, "not_found", ErrFolderNotFound.Error()},
	{ErrInvalidCursor, http.StatusBadRequest, "invalid_request", "cursor is not valid"},
	{ErrCursorOrder, http.StatusBadRequest, "invalid_request", ErrCursorOrder.Error()},
	{ErrNothingToUpdate, http.StatusBadRequest, "invalid_request", ErrNothingToUpdate.Error()},
	{folders.ErrForbidden, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage},
	{folders.ErrNoAccess, http.StatusNotFound, "not_found", ErrNotFound.Error()},
}

func abortKnown(c *gin.Context, err error) bool {
	for _, known := range knownFailures {
		if errors.Is(err, known.err) {
			httpapi.AbortError(c, known.status, known.code, known.message)
			return true
		}
	}
	var invalid *InvalidInputError
	var conflict *ConflictError
	var refusal *lifecycle.Refusal
	var badSpec *dashboards.InvalidSpecError
	switch {
	case errors.Is(err, folders.ErrMoveIntoPersonal):
		folders.AbortPersonalFolder(c, moveIntoPersonalMessage)
	case folders.AbortRestricted(c, err):
	case errors.As(err, &badSpec):
		httpapi.AbortInvalidSpec(c, "spec is invalid", badSpec.Issues)
	case errors.As(err, &refusal):
		lifecycle.AbortRefusal(c, refusal)
	case errors.As(err, &conflict):
		c.AbortWithStatusJSON(http.StatusConflict, conflictJSON{Error: conflict.Error(), Code: codeVersionConflict, Current: BlockOf(conflict.Current)})
	case errors.As(err, &invalid):
		issues := invalid.Issues
		if issues == nil {
			issues = []spec.Issue{}
		}
		reason, _ := usertext.ReasonOf(invalid)
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, httpapi.InvalidSpecBody{Error: invalid.Message, Code: invalid.Code, Issues: issues, Reason: string(reason)})
	default:
		return false
	}
	return true
}

func queryFailure(status int) bool {
	switch status {
	case http.StatusUnprocessableEntity, http.StatusTooManyRequests, http.StatusBadGateway, http.StatusGatewayTimeout:
		return true
	}
	return false
}
