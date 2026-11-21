package server

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type datasourceJSON struct {
	datasource.Status
	DashboardCount int                 `json:"dashboard_count"`
	Audience       datasource.Audience `json:"audience,omitempty"`
}

func audienceForAdmin(c *gin.Context, registry *datasource.Registry, uid string) datasource.Audience {
	if !datasource.ReadsRestricted(c.Request.Context()) {
		return ""
	}
	if registry.AdminsOnly(uid) {
		return datasource.AudienceAdmins
	}
	return datasource.AudienceEveryone
}

type datasourcesResponse struct {
	Datasources []datasourceJSON `json:"datasources"`
}

func registerDatasourceRoutes(api auth.Routes, registry *datasource.Registry, usage DatasourceUsage, logger *slog.Logger) {
	counts := func(c *gin.Context) (map[string]int, bool) {
		if usage == nil {
			return nil, true
		}
		ctx := c.Request.Context()
		counts, err := usage.DatasourceUsage(ctx, httpapi.Caller(c))
		if err != nil {
			if clientgone.Handle(c, logger, "count dashboards per datasource", err) {
				return nil, false
			}
			logger.ErrorContext(ctx, "count dashboards per datasource", logattrs.Failure("datasource_usage_failed", err)...)
			httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
			return nil, false
		}
		return counts, true
	}
	api.GET("/datasources", auth.RoleViewer, func(c *gin.Context) {
		counts, ok := counts(c)
		if !ok {
			return
		}
		statuses := []datasource.Status{}
		if registry != nil {
			statuses = registry.Health(c.Request.Context())
		}
		out := make([]datasourceJSON, len(statuses))
		for i, s := range statuses {
			out[i] = datasourceJSON{Status: s, DashboardCount: counts[s.UID], Audience: audienceForAdmin(c, registry, s.UID)}
		}
		c.JSON(http.StatusOK, datasourcesResponse{Datasources: out})
	})
	api.POST("/datasources/:uid/recheck", auth.RoleViewer, func(c *gin.Context) {
		if registry == nil {
			httpapi.AbortError(c, http.StatusNotFound, "not_found", "datasource not found")
			return
		}
		status, found := registry.Recheck(c.Request.Context(), c.Param("uid"))
		if !found {
			httpapi.AbortError(c, http.StatusNotFound, "not_found", "datasource not found")
			return
		}
		counts, ok := counts(c)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, datasourceJSON{Status: status, DashboardCount: counts[status.UID], Audience: audienceForAdmin(c, registry, status.UID)})
	})
}
