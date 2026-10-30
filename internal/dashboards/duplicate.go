package dashboards

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type duplicateRequest struct {
	Title    *string `json:"title"`
	FolderID *string `json:"folder_id"`
}

func duplicateDraft(source Version, title string) (string, *spec.Dashboard, []byte, error) {
	copied := *source.Spec
	copied.Title = title
	raw, err := json.Marshal(&copied)
	if err != nil {
		return "", nil, nil, fmt.Errorf("marshal spec: %w", err)
	}
	summary := "Duplicated from \"" + source.Spec.Title + "\" version " + strconv.Itoa(source.Number)
	return validateSave(raw, summary, "")
}

func (s *Service) Duplicate(ctx context.Context, by auth.Principal, id, folderID pgtype.UUID, title string) (Dashboard, Version, error) {
	if _, err := s.need(ctx, by, id, folders.NeedView); err != nil {
		return Dashboard{}, Version{}, err
	}
	target, err := s.creationTarget(ctx, by, folderID)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	_, source, err := s.store.latest(ctx, id)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	summary, parsed, canonical, err := duplicateDraft(source, title)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	if issues := s.runner.CheckDatasources(parsed); len(issues) > 0 {
		return Dashboard{}, Version{}, &InvalidSpecError{Issues: issues}
	}
	dash, ver, err := s.store.create(ctx, newVersion{spec: parsed, canonical: canonical, summary: summary, author: authorOf(by), origin: origin.From(ctx), unverified: source.Unverified, folder: target.folder})
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	dash.Abilities = target.abilities
	return dash, ver, nil
}

func readDuplicate(c *gin.Context) (string, pgtype.UUID, bool) {
	var req duplicateRequest
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return "", pgtype.UUID{}, false
	}
	if req.Title == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "title is required")
		return "", pgtype.UUID{}, false
	}
	folderID, ok := optionalFolderID(c, req.FolderID)
	if !ok {
		return "", pgtype.UUID{}, false
	}
	return *req.Title, folderID, true
}

func (s *Service) duplicateDashboard(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	title, folderID, ok := readDuplicate(c)
	if !ok {
		return
	}
	dash, ver, err := s.Duplicate(c.Request.Context(), httpapi.Caller(c), id, folderID, title)
	if err != nil {
		s.fail(c, "duplicate dashboard", err)
		return
	}
	c.Header("Location", "/api/v1/dashboards/"+dash.ID)
	c.JSON(http.StatusCreated, NewDashboardVersionJSON(dash, ver))
}
