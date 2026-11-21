package folders

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxBodyBytes = 64 << 10

	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

var (
	errFolderGone    = errors.New("folder is gone")
	errGeneralFolder = errors.New("general folder")
	errDuplicateUser = errors.New("duplicate user")
)

type unknownUserError struct {
	email     string
	ambiguous bool
}

func (e *unknownUserError) Error() string { return "unknown user" }

type SharingSettler func(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, folderID pgtype.UUID) error

type OpeningGuard func(ctx context.Context, q *dbsqlc.Queries, folder dbsqlc.Folder) error

type Handler struct {
	db      *db.DB
	access  *Access
	logger  *slog.Logger
	settle  SharingSettler
	opening OpeningGuard
}

func NewHandler(database *db.DB, access *Access, logger *slog.Logger) *Handler {
	return &Handler{db: database, access: access, logger: logger}
}

func (h *Handler) SettleSharingWith(settle SharingSettler) *Handler {
	h.settle = settle
	return h
}

func (h *Handler) GuardOpeningWith(guard OpeningGuard) *Handler {
	h.opening = guard
	return h
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/folders")
	g.GET("", auth.RoleViewer, h.list)
	g.POST("", auth.RoleEditor, h.create)
	g.GET("/:id", auth.RoleViewer, h.access.RequireFolder(NeedView), h.get)
	g.PATCH("/:id", auth.RoleViewer, h.access.RequireFolder(NeedManage), h.rename)
	g.DELETE("/:id", auth.RoleViewer, h.access.RequireFolder(NeedManage), h.remove)
	g.GET("/:id/permissions", auth.RoleViewer, h.access.RequireFolder(NeedView), h.permissions)
	g.PUT("/:id/permissions", auth.RoleViewer, h.access.RequireFolder(NeedManage), h.setPermissions)
}

func (h *Handler) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	h.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}

func (h *Handler) bodies(ctx context.Context, items []folderAccess) ([]folderJSON, error) {
	ids := make([]pgtype.UUID, len(items))
	for i, item := range items {
		ids[i] = item.Folder.ID
	}
	rows, err := h.db.Queries().ListFolderDetails(ctx, ids)
	if err != nil {
		return nil, err
	}
	details := make(map[pgtype.UUID]dbsqlc.ListFolderDetailsRow, len(rows))
	for _, r := range rows {
		details[r.ID] = r
	}
	audit, err := h.access.AuditFolder(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]folderJSON, 0, len(items))
	for _, item := range items {
		out = append(out, folderBody(item.Folder, item.Abilities, details[item.Folder.ID], audit))
	}
	return out, nil
}

func (h *Handler) respondFolder(c *gin.Context, status int, item folderAccess) {
	bodies, err := h.bodies(c.Request.Context(), []folderAccess{item})
	if err != nil {
		h.fail(c, "describe folder", err)
		return
	}
	c.JSON(status, bodies[0])
}

func (h *Handler) list(c *gin.Context) {
	ctx := c.Request.Context()
	levels, err := h.access.Levels(ctx, httpapi.Caller(c))
	if errors.Is(err, ErrForbidden) {
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
		return
	}
	if err != nil {
		h.fail(c, "list folders", err)
		return
	}
	folders := levels.Folders()
	items := make([]folderAccess, len(folders))
	for i, f := range folders {
		items[i] = folderAccess{Folder: f, Abilities: levels.Of(f.ID)}
	}
	bodies, err := h.bodies(ctx, items)
	if err != nil {
		h.fail(c, "list folders", err)
		return
	}
	httpapi.RevalidatedJSON(c, gin.H{"folders": bodies})
}

func (h *Handler) get(c *gin.Context) {
	h.respondFolder(c, http.StatusOK, folderFrom(c))
}

type titleBody struct {
	Title string `json:"title"`
}

func (h *Handler) create(c *gin.Context) {
	var req titleBody
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	caller := httpapi.Caller(c)
	folder, abilities, err := h.access.CreateFolder(c.Request.Context(), caller, req.Title)
	if h.abortTitleWrite(c, err, "create folder") {
		return
	}
	c.Header("Location", "/api/v1/folders/"+folder.ID.String())
	h.respondFolder(c, http.StatusCreated, folderAccess{Folder: folder, Abilities: abilities})
}

func (h *Handler) abortTitleWrite(c *gin.Context, err error, action string) bool {
	var invalidTitle *InvalidTitleError
	switch {
	case err == nil:
		return false
	case errors.As(err, &invalidTitle):
		httpapi.AbortText(c, http.StatusBadRequest, "invalid_request", err)
	case errors.Is(err, ErrFolderExists):
		httpapi.AbortError(c, http.StatusConflict, "folder_exists", "a folder with that name already exists")
	case errors.Is(err, ErrGeneralFolder):
		httpapi.AbortError(c, http.StatusConflict, "general_folder", "the General folder cannot be renamed")
	case errors.Is(err, ErrNotFound):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "folder not found")
	case errors.Is(err, ErrForbidden):
		httpapi.AbortError(c, http.StatusForbidden, forbidden.Code, forbidden.RoleMessage)
	default:
		h.fail(c, action, err)
	}
	return true
}

func (h *Handler) rename(c *gin.Context) {
	var req titleBody
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	folder, abilities, err := h.access.RenameFolder(c.Request.Context(), httpapi.Caller(c), folderFrom(c).Folder.ID, req.Title)
	if h.abortTitleWrite(c, err, "rename folder") {
		return
	}
	h.respondFolder(c, http.StatusOK, folderAccess{Folder: folder, Abilities: abilities})
}

func (h *Handler) remove(c *gin.Context) {
	current := folderFrom(c)
	if current.Folder.General {
		httpapi.AbortError(c, http.StatusConflict, "general_folder", "the General folder cannot be deleted")
		return
	}
	ctx := c.Request.Context()
	n, err := h.deleteEmpty(ctx, current.Folder.ID)
	if isPgError(err, foreignKeyViolation) {
		httpapi.AbortError(c, http.StatusConflict, "folder_not_empty", "move or delete the dashboards, findings, saved searches and observations in this folder first")
		return
	}
	if err != nil {
		h.fail(c, "delete folder", err)
		return
	}
	if n == 0 {
		h.refuseUndeleted(c, current.Folder.ID)
		return
	}
	h.logger.InfoContext(ctx, "deleted a folder", slog.String("actor_id", httpapi.Caller(c).ID), slog.String("folder_id", current.Folder.ID.String()))
	c.Status(http.StatusNoContent)
}

func (h *Handler) deleteEmpty(ctx context.Context, id pgtype.UUID) (int64, error) {
	var n int64
	err := h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		folder, err := q.LockFolder(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		n, err = q.DeleteFolderIfEmpty(ctx, id)
		if err != nil || n == 0 {
			return err
		}
		return recordFolder(ctx, q, "folder.delete", id, folder.Title, "", nil)
	})
	return n, err
}

func (h *Handler) refuseUndeleted(c *gin.Context, id pgtype.UUID) {
	holds, err := h.db.Queries().FolderHasContents(c.Request.Context(), id)
	if err != nil {
		h.fail(c, "delete folder", err)
		return
	}
	if holds {
		httpapi.AbortError(c, http.StatusConflict, "folder_not_empty", "move or delete the dashboards, findings, saved searches and observations in this folder first")
		return
	}
	httpapi.AbortError(c, http.StatusNotFound, "not_found", "folder not found")
}

func (h *Handler) permissions(c *gin.Context) {
	h.respondPermissions(c, folderFrom(c).Folder.ID)
}

func (h *Handler) respondPermissions(c *gin.Context, id pgtype.UUID) {
	ctx := c.Request.Context()
	folder, err := h.db.Queries().GetFolder(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "folder not found")
		return
	}
	if err != nil {
		h.fail(c, "list folder permissions", err)
		return
	}
	rows, err := h.db.Queries().ListFolderPermissions(ctx, id)
	if err != nil {
		h.fail(c, "list folder permissions", err)
		return
	}
	c.JSON(http.StatusOK, permissionsBody(folder, rows))
}

type grant struct {
	userID pgtype.UUID
	group  pgtype.Text
	level  Level
}

func resolveGrants(ctx context.Context, q *dbsqlc.Queries, entries []entry) ([]grant, error) {
	grants := make([]grant, 0, len(entries))
	seen := map[pgtype.UUID]bool{}
	for _, e := range entries {
		g := grant{level: e.level}
		switch {
		case e.group != "":
			g.group = pgtype.Text{String: e.group, Valid: true}
		case e.email != "":
			ids, err := q.UsersByEmail(ctx, e.email)
			if err != nil {
				return nil, err
			}
			if len(ids) != 1 {
				return nil, &unknownUserError{email: e.email, ambiguous: len(ids) > 1}
			}
			g.userID = ids[0]
		default:
			exists, err := q.UserExists(ctx, e.userID)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, &unknownUserError{}
			}
			g.userID = e.userID
		}
		if g.userID.Valid {
			if seen[g.userID] {
				return nil, errDuplicateUser
			}
			seen[g.userID] = true
		}
		grants = append(grants, g)
	}
	return grants, nil
}

type sharing struct {
	entries    []entry
	adminsOnly bool
}

func (h *Handler) replacePermissions(ctx context.Context, by auth.Principal, folderID, grantedBy pgtype.UUID, next sharing) error {
	return h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		folder, err := q.LockFolder(ctx, folderID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errFolderGone
		}
		if err != nil {
			return err
		}
		if folder.General {
			return errGeneralFolder
		}
		grants, err := resolveGrants(ctx, q, next.entries)
		if err != nil {
			return err
		}
		if err := h.setAdminsOnly(ctx, q, folder, next.adminsOnly); err != nil {
			return err
		}
		before, err := q.ListFolderGrantsForAudit(ctx, folderID)
		if err != nil {
			return err
		}
		if err := q.DeleteFolderPermissions(ctx, folderID); err != nil {
			return err
		}
		for _, g := range grants {
			err := q.InsertFolderPermission(ctx, dbsqlc.InsertFolderPermissionParams{
				FolderID: folderID, UserID: g.userID, GroupName: g.group, Level: g.level.String(), CreatedByID: grantedBy,
			})
			if err != nil {
				return err
			}
		}
		after, err := q.ListFolderGrantsForAudit(ctx, folderID)
		if err != nil {
			return err
		}
		data, unchanged := permissionChange(before, after)
		data["admins_only"], data["admins_only_before"] = next.adminsOnly, AdminsOnly(folder)
		unchanged = unchanged && next.adminsOnly == AdminsOnly(folder)
		if err := recordFolder(ctx, q, "folder.permissions_change", folderID, folder.Title, noChangeReason(unchanged), data); err != nil {
			return err
		}
		if h.settle == nil {
			return nil
		}
		return h.settle(ctx, q, by, folderID)
	})
}

func (h *Handler) setAdminsOnly(ctx context.Context, q *dbsqlc.Queries, folder dbsqlc.Folder, adminsOnly bool) error {
	if AdminsOnly(folder) && !adminsOnly && h.opening != nil {
		if err := h.opening(ctx, q, folder); err != nil {
			return err
		}
	}
	if folder.AdminsOnly == adminsOnly {
		return nil
	}
	if adminsOnly {
		linked, err := q.FolderHasLivePublicLinks(ctx, folder.ID)
		if err != nil {
			return err
		}
		if linked {
			return ErrLinkedFolder
		}
	}
	return q.SetFolderAdminsOnly(ctx, dbsqlc.SetFolderAdminsOnlyParams{ID: folder.ID, AdminsOnly: adminsOnly})
}

func abortPermissionsRefusal(c *gin.Context, err error) bool {
	var unknown *unknownUserError
	switch {
	case errors.Is(err, errFolderGone):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "folder not found")
	case errors.Is(err, errGeneralFolder):
		httpapi.AbortError(c, http.StatusConflict, "general_folder", "the General folder is open to everyone and cannot be restricted")
	case errors.Is(err, errDuplicateUser):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "a user can appear only once")
	case errors.As(err, &unknown):
		abortUnknownUser(c, unknown)
	case AbortRestricted(c, err):
	default:
		return false
	}
	return true
}

func (h *Handler) setPermissions(c *gin.Context) {
	var req struct {
		Entries    []entryBody `json:"entries"`
		AdminsOnly bool        `json:"admins_only"`
	}
	if !httpapi.DecodeBody(c, &req, maxBodyBytes) {
		return
	}
	if req.Entries == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "entries is required")
		return
	}
	entries, err := parseEntries(req.Entries)
	if err != nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.AdminsOnly && len(entries) > 0 {
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "invalid_request", "a folder only Admins can read has no entries; send admins_only with an empty list")
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	if req.AdminsOnly && caller.Role != auth.RoleAdmin {
		httpapi.AbortError(c, http.StatusForbidden, "forbidden", "only an Admin can make a folder only Admins can read")
		return
	}
	folderID := folderFrom(c).Folder.ID
	var grantedBy pgtype.UUID
	if caller.Kind == auth.KindUser {
		grantedBy, _ = db.ParseUUID(caller.ID)
	}
	if err := h.replacePermissions(ctx, caller, folderID, grantedBy, sharing{entries: entries, adminsOnly: req.AdminsOnly}); err != nil {
		if !abortPermissionsRefusal(c, err) {
			h.fail(c, "set folder permissions", err)
		}
		return
	}
	h.logger.InfoContext(ctx, "set folder permissions",
		slog.String("actor_id", caller.ID), slog.String("folder_id", folderID.String()), slog.Int("entry_count", len(entries)), slog.Bool("admins_only", req.AdminsOnly))
	h.respondPermissions(c, folderID)
}

func abortUnknownUser(c *gin.Context, e *unknownUserError) {
	message := "no signed-in user matches that entry"
	if e.ambiguous {
		message = "more than one user has that email address"
	}
	body := gin.H{"error": message, "code": "unknown_user"}
	if e.email != "" {
		body["email"] = e.email
	}
	c.AbortWithStatusJSON(http.StatusUnprocessableEntity, body)
}
