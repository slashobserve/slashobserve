package prompts

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxBodyBytes     = 32 << 10
	maxQueryNameSize = 256

	uniqueViolation      = "23505"
	ownerNameConstraint  = "saved_prompts_owner_name_key"
	sharedNameConstraint = "saved_prompts_shared_name_key"
)

var (
	errTooMany  = errors.New("too many saved prompts")
	errNotFound = errors.New("saved prompt not found")
	errNotOwner = errors.New("saved prompt is not editable by the caller")
)

type record = dbsqlc.ListSavedPromptsRow

type ownerJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type promptJSON struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	Shared    bool      `json:"shared"`
	Owner     ownerJSON `json:"owner"`
	CanEdit   bool      `json:"can_edit"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

type listJSON struct {
	Prompts []promptJSON `json:"prompts"`
}

type Handler struct {
	db     *db.DB
	logger *slog.Logger
}

func NewHandler(database *db.DB, logger *slog.Logger) *Handler {
	return &Handler{db: database, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/prompts")
	g.GET("", auth.RoleViewer, h.list)
	g.POST("", auth.RoleEditor, auth.RequireUser, h.create)
	g.PATCH("/:id", auth.RoleEditor, auth.RequireUser, h.update)
	g.DELETE("/:id", auth.RoleEditor, auth.RequireUser, h.remove)
}

func promptOf(r record, caller auth.Principal) promptJSON {
	ownerID := r.OwnerID.String()
	return promptJSON{
		ID:        r.ID.String(),
		Name:      r.Name,
		Body:      r.Body,
		Shared:    r.Shared,
		Owner:     ownerJSON{ID: ownerID, Name: r.OwnerName},
		CanEdit:   CanEdit(caller, ownerID, r.Shared),
		CreatedAt: httpapi.FormatTime(r.CreatedAt.Time),
		UpdatedAt: httpapi.FormatTime(r.UpdatedAt.Time),
	}
}

func (h *Handler) fail(c *gin.Context, action, promptID string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err,
		slog.String("actor_id", httpapi.Caller(c).ID), slog.String("prompt_id", promptID))...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}

func (h *Handler) list(c *gin.Context) {
	caller := httpapi.Caller(c)
	rawName, byName := c.GetQuery("name")
	if len(rawName) > maxQueryNameSize {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "name is too long")
		return
	}
	body := listJSON{Prompts: []promptJSON{}}
	var name string
	if byName {
		var ok bool
		if name, ok = NormalizeName(rawName); !ok {
			c.JSON(http.StatusOK, body)
			return
		}
	}
	callerID, err := db.ParseUUID(caller.ID)
	if err != nil {
		h.fail(c, "list saved prompts", "", err)
		return
	}
	ctx := c.Request.Context()
	var rows []record
	if byName {
		var found []dbsqlc.FindSavedPromptByNameRow
		found, err = h.db.Queries().FindSavedPromptByName(ctx, dbsqlc.FindSavedPromptByNameParams{Name: name, CallerID: callerID})
		for _, r := range found {
			rows = append(rows, record(r))
		}
	} else {
		rows, err = h.db.Queries().ListSavedPrompts(ctx, callerID)
	}
	if err != nil {
		h.fail(c, "list saved prompts", "", err)
		return
	}
	for _, r := range rows {
		body.Prompts = append(body.Prompts, promptOf(r, caller))
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) create(c *gin.Context) {
	var req struct {
		Name   *string `json:"name"`
		Body   *string `json:"body"`
		Shared *bool   `json:"shared"`
	}
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	if req.Name == nil {
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "invalid_name", messageInvalidName)
		return
	}
	if req.Body == nil {
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "invalid_body", messageInvalidBody)
		return
	}
	patch := Patch{Name: req.Name, Body: req.Body}
	if !validate(c, &patch) {
		return
	}
	name, body := *patch.Name, *patch.Body
	shared := req.Shared != nil && *req.Shared

	caller := httpapi.Caller(c)
	ownerID, err := db.ParseUUID(caller.ID)
	if err != nil {
		h.fail(c, "save prompt", "", err)
		return
	}
	ctx := c.Request.Context()
	var inserted dbsqlc.InsertSavedPromptRow
	err = h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.LockSavedPromptOwner(ctx, caller.ID); err != nil {
			return err
		}
		n, err := q.CountSavedPromptsByOwner(ctx, ownerID)
		if err != nil {
			return err
		}
		if n >= MaxPerOwner {
			return errTooMany
		}
		inserted, err = q.InsertSavedPrompt(ctx, dbsqlc.InsertSavedPromptParams{OwnerID: ownerID, Name: name, Body: body, Shared: shared})
		return err
	})
	if h.refuse(c, err) {
		return
	}
	if err != nil {
		h.fail(c, "save prompt", "", err)
		return
	}
	id := inserted.ID.String()
	h.logger.InfoContext(ctx, "saved a prompt", slog.String("actor_id", caller.ID), slog.String("prompt_id", id), slog.Bool("shared", shared))
	ownerName := caller.Name
	if ownerName == "" {
		ownerName = caller.Email
	}
	c.Header("Location", "/api/v1/prompts/"+id)
	c.JSON(http.StatusCreated, promptOf(record{
		ID: inserted.ID, OwnerID: ownerID, Name: name, Body: body, Shared: shared,
		CreatedAt: inserted.CreatedAt, UpdatedAt: inserted.UpdatedAt, OwnerName: ownerName,
	}, caller))
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var patch Patch
	if !httpapi.DecodeBody(c, &patch, maxBodyBytes) {
		return
	}
	if patch.Empty() {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "send at least one of name, body, shared")
		return
	}
	if !validate(c, &patch) {
		return
	}
	caller := httpapi.Caller(c)
	ctx := c.Request.Context()
	var updated dbsqlc.UpdateSavedPromptRow
	err := h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		current, err := h.editable(ctx, q, caller, id)
		if err != nil {
			return err
		}
		name, body, shared := patch.Apply(current.Name, current.Body, current.Shared)
		updated, err = q.UpdateSavedPrompt(ctx, dbsqlc.UpdateSavedPromptParams{ID: id, Name: name, Body: body, Shared: shared})
		return err
	})
	if h.refuse(c, err) {
		return
	}
	if err != nil {
		h.fail(c, "change saved prompt", id.String(), err)
		return
	}
	h.logger.InfoContext(ctx, "changed a saved prompt", slog.String("actor_id", caller.ID), slog.String("prompt_id", id.String()), slog.Bool("shared", updated.Shared))
	c.JSON(http.StatusOK, promptOf(record(updated), caller))
}

func (h *Handler) remove(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	caller := httpapi.Caller(c)
	ctx := c.Request.Context()
	var shared bool
	err := h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		current, err := h.editable(ctx, q, caller, id)
		if err != nil {
			return err
		}
		shared = current.Shared
		_, err = q.DeleteSavedPrompt(ctx, id)
		return err
	})
	if h.refuse(c, err) {
		return
	}
	if err != nil {
		h.fail(c, "delete saved prompt", id.String(), err)
		return
	}
	h.logger.InfoContext(ctx, "deleted a saved prompt", slog.String("actor_id", caller.ID), slog.String("prompt_id", id.String()), slog.Bool("shared", shared))
	c.Status(http.StatusNoContent)
}

func (h *Handler) editable(ctx context.Context, q *dbsqlc.Queries, caller auth.Principal, id pgtype.UUID) (dbsqlc.GetVisibleSavedPromptForUpdateRow, error) {
	callerID, err := db.ParseUUID(caller.ID)
	if err != nil {
		return dbsqlc.GetVisibleSavedPromptForUpdateRow{}, err
	}
	row, err := q.GetVisibleSavedPromptForUpdate(ctx, dbsqlc.GetVisibleSavedPromptForUpdateParams{ID: id, CallerID: callerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, errNotFound
	}
	if err != nil {
		return row, err
	}
	if !CanEdit(caller, row.OwnerID.String(), row.Shared) {
		return row, errNotOwner
	}
	return row, nil
}

func validate(c *gin.Context, patch *Patch) bool {
	if patch.Name != nil {
		name, ok := NormalizeName(*patch.Name)
		if !ok {
			httpapi.AbortError(c, http.StatusUnprocessableEntity, "invalid_name", messageInvalidName)
			return false
		}
		patch.Name = &name
	}
	if patch.Body != nil {
		body, ok := CleanBody(*patch.Body)
		if !ok {
			httpapi.AbortError(c, http.StatusUnprocessableEntity, "invalid_body", messageInvalidBody)
			return false
		}
		patch.Body = &body
	}
	return true
}

func (h *Handler) refuse(c *gin.Context, err error) bool {
	var pgErr *pgconn.PgError
	switch {
	case err == nil:
		return false
	case errors.Is(err, errTooMany):
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "too_many_prompts", messageTooMany)
	case errors.Is(err, errNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "prompt not found")
	case errors.Is(err, errNotOwner):
		httpapi.AbortError(c, http.StatusForbidden, "not_owner", messageNotOwner)
	case errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == ownerNameConstraint:
		httpapi.AbortError(c, http.StatusConflict, "name_taken", messageOwnNameTaken)
	case errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == sharedNameConstraint:
		httpapi.AbortError(c, http.StatusConflict, "name_taken", messageSharedNameTaken)
	default:
		return false
	}
	return true
}
