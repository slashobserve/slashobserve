package folders

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type dashboardKey struct{}

type folderKey struct{}

type folderAccess struct {
	Folder    Folder
	Abilities Abilities
}

func DashboardFrom(c *gin.Context) DashboardAccess {
	v, _ := c.Get(dashboardKey{})
	d, _ := v.(DashboardAccess)
	return d
}

func folderFrom(c *gin.Context) folderAccess {
	v, _ := c.Get(folderKey{})
	f, _ := v.(folderAccess)
	return f
}

func (a *Access) RequireDashboard(n Need) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := httpapi.UUIDParam(c, "id")
		if !ok {
			return
		}
		caller := httpapi.Caller(c)
		access, err := a.Dashboard(c.Request.Context(), caller, id)
		switch {
		case errors.Is(err, ErrNotFound):
			httpapi.AbortError(c, http.StatusNotFound, "not_found", "dashboard not found")
			return
		case errors.Is(err, ErrNoAccess), errors.Is(err, ErrForbidden):
			httpapi.AbortError(c, http.StatusForbidden, "forbidden_role", "you don't have access to this dashboard")
			return
		case err != nil:
			a.fail(c, "check dashboard access", err)
			return
		}
		if !access.Abilities.Has(n) {
			httpapi.AbortError(c, http.StatusForbidden, "forbidden_role", "you don't have permission to do this on this dashboard")
			return
		}
		c.Set(dashboardKey{}, access)
		c.Next()
	}
}

func (a *Access) RequireFolder(n Need) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := httpapi.UUIDParam(c, "id")
		if !ok {
			return
		}
		folder, abilities, err := a.Folder(c.Request.Context(), httpapi.Caller(c), id)
		switch {
		case errors.Is(err, ErrNotFound):
			httpapi.AbortError(c, http.StatusNotFound, "not_found", "folder not found")
			return
		case errors.Is(err, ErrForbidden):
			httpapi.AbortError(c, http.StatusForbidden, "forbidden_role", "your role does not allow this")
			return
		case err != nil:
			a.fail(c, "check folder access", err)
			return
		}
		if !abilities.Has(n) {
			httpapi.AbortError(c, http.StatusForbidden, "forbidden_role", "you don't have permission to do this in this folder")
			return
		}
		c.Set(folderKey{}, folderAccess{Folder: folder, Abilities: abilities})
		c.Next()
	}
}

func (a *Access) fail(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, a.logger, action, err) {
		return
	}
	a.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}
