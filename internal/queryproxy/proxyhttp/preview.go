package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxPreviewBodyBytes = spec.MaxBytes + 64<<10
	maxPanelIDLength    = 64
)

type previewRequest struct {
	DashboardID   string              `json:"dashboard_id"`
	Version       int                 `json:"version"`
	PanelID       string              `json:"panel_id"`
	Panel         json.RawMessage     `json:"panel"`
	From          *string             `json:"from"`
	To            *string             `json:"to"`
	MaxDataPoints *int                `json:"max_data_points"`
	Variables     map[string][]string `json:"variables"`
}

func (h *Handler) preview(c *gin.Context) {
	ctx := c.Request.Context()
	principal, _ := auth.FromContext(ctx)
	if principal.Kind != auth.KindUser {
		abortError(c, http.StatusForbidden, "user_session_required", "the draft preview is for signed-in users; MCP clients and scripts use query_range")
		return
	}
	release, ok := h.acquire(c)
	if !ok {
		return
	}
	defer release()
	var req previewRequest
	if !httpapi.DecodeBody(c, &req, maxPreviewBodyBytes) {
		return
	}
	id, message := req.validate()
	if message != "" {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), message)
		return
	}
	uid := c.Param("uid")
	if _, found := h.registry.Get(uid); !found {
		abortError(c, http.StatusNotFound, string(queryproxy.CodeNotFound), "datasource not found")
		return
	}
	saved, err := h.dashboard(ctx, id, req.Version)
	if err != nil {
		h.previewFail(c, uid, err)
		return
	}
	d, draft, ok := h.placeDraft(c, saved.dashboard, req, uid)
	if !ok {
		return
	}
	p, err := parseParams(req.values(), true)
	if err != nil {
		h.previewFail(c, uid, err)
		return
	}
	rng, err := p.timeRange(d, time.Now())
	if err != nil {
		h.previewFail(c, uid, err)
		return
	}
	maxDataPoints := p.maxDataPoints
	if draft.MaxDataPoints > 0 {
		maxDataPoints = min(maxDataPoints, draft.MaxDataPoints)
	}
	al := align(rng, queryproxy.PanelKind(draft), maxDataPoints)
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	user := queryproxy.UserFor(principal)
	selections, err := h.runner.Defaults(ctx, d, al.rng, p.selections, user)
	if err != nil {
		h.previewFail(c, uid, err)
		return
	}
	entry, err := h.render(ctx, queryproxy.Request{
		Dashboard:     d,
		PanelID:       req.PanelID,
		Range:         al.rng,
		Selections:    selections,
		MaxDataPoints: al.maxDataPoints,
		Limits:        queryproxy.DraftLimits,
		User:          user,
	})
	if err != nil {
		h.previewFail(c, uid, err)
		return
	}
	writeData(c, entry, false, entry.executedAt)
}

func (h *Handler) placeDraft(c *gin.Context, saved *spec.Dashboard, req previewRequest, uid string) (d *spec.Dashboard, draft *spec.Panel, ok bool) {
	d, err := spec.WithPanel(saved, req.PanelID, req.Panel)
	var invalid *spec.ValidationError
	switch {
	case err == nil:
	case errors.Is(err, spec.ErrNoPanel):
		abortError(c, http.StatusNotFound, string(queryproxy.CodePanelNotFound), "panel not found")
		return nil, nil, false
	case errors.As(err, &invalid):
		httpapi.AbortInvalidSpec(c, "the draft panel is not valid", invalid.Issues)
		return nil, nil, false
	default:
		h.internalFailure(c, uid, err)
		return nil, nil, false
	}
	original, _ := saved.FindPanel(req.PanelID)
	entries := d.AllPanels()
	at := slices.IndexFunc(entries, func(e spec.PanelEntry) bool { return e.Pointer == original.Pointer })
	if at < 0 {
		h.internalFailure(c, uid, errInternal)
		return nil, nil, false
	}
	draft = entries[at].Panel
	invalidRequest := func(message string) (*spec.Dashboard, *spec.Panel, bool) {
		abortError(c, http.StatusBadRequest, string(queryproxy.CodeInvalidRequest), message)
		return nil, nil, false
	}
	switch {
	case !draft.Type.IsData() || len(draft.Queries) == 0:
		return invalidRequest("only data panels can be previewed")
	case draft.ID != req.PanelID:
		return invalidRequest("the panel's id must be " + req.PanelID)
	case draft.Datasource != uid:
		return invalidRequest("the panel's datasource must be " + uid)
	case usesOtherDatasource(draft, uid):
		return invalidRequest("every query of the panel must use datasource " + uid)
	}
	return d, draft, true
}

func usesOtherDatasource(p *spec.Panel, uid string) bool {
	for i := range p.Queries {
		if p.QueryDatasource(i) != uid {
			return true
		}
	}
	return false
}

func (r previewRequest) validate() (id pgtype.UUID, message string) {
	id, ok := parseID(r.DashboardID)
	switch {
	case !ok:
		return id, "dashboard_id must be a UUID"
	case !validVersion(r.Version):
		return id, "version must be a positive 32-bit integer"
	case utf8.RuneCountInString(r.PanelID) < 1 || utf8.RuneCountInString(r.PanelID) > maxPanelIDLength:
		return id, "panel_id must be from 1 to 64 characters"
	case !bytes.HasPrefix(bytes.TrimSpace(r.Panel), []byte("{")):
		return id, "panel must be a JSON object"
	}
	for _, name := range slices.Sorted(maps.Keys(r.Variables)) {
		switch {
		case name == "":
			return id, "variables needs non-empty names"
		case len(r.Variables[name]) == 0:
			return id, "variables." + name + " needs at least one value"
		}
	}
	return id, ""
}

func (r previewRequest) values() url.Values {
	q := url.Values{}
	if r.From != nil {
		q.Set(paramFrom, *r.From)
	}
	if r.To != nil {
		q.Set(paramTo, *r.To)
	}
	if r.MaxDataPoints != nil {
		q.Set(paramMaxDataPoints, strconv.Itoa(*r.MaxDataPoints))
	}
	for name, values := range r.Variables {
		q[variablePrefix+name] = values
	}
	return q
}

// A 500 was already logged where it happened (version load, frame conversion, recovered panics, upstream calls), so only 502 and 504 are logged here.
func (h *Handler) previewFail(c *gin.Context, uid string, err error) {
	if status, code, _ := queryproxy.Classify(err); status > http.StatusInternalServerError {
		h.logPreviewFailure(c, uid, code, err)
	}
	h.fail(c, err)
}

func (h *Handler) internalFailure(c *gin.Context, uid string, err error) {
	h.logPreviewFailure(c, uid, "internal", err)
	h.fail(c, errInternal)
}

func (h *Handler) logPreviewFailure(c *gin.Context, uid, code string, err error) {
	h.logger.ErrorContext(c.Request.Context(), "draft preview failed", logattrs.Failure(code, err, slog.String("datasource_uid", uid))...)
}
