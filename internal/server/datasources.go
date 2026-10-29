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
	DashboardCount int `json:"dashboard_count"`
}

type datasourcesResponse struct {
	Datasources []datasourceJSON `json:"datasources"`
}

func registerDatasourceRoutes(api auth.Routes, registry *datasource.Registry, usage DatasourceUsage, logger *slog.Logger) {
	api.GET("/datasources", auth.RoleViewer, func(c *gin.Context) {
		ctx := c.Request.Context()
		var counts map[string]int
		if usage != nil {
			var err error
			if counts, err = usage.DatasourceUsage(ctx, httpapi.Caller(c)); err != nil {
				if clientgone.Handle(c, logger, "count dashboards per datasource", err) {
					return
				}
				logger.ErrorContext(ctx, "count dashboards per datasource", logattrs.Failure("datasource_usage_failed", err)...)
				httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
				return
			}
		}
		statuses := []datasource.Status{}
		if registry != nil {
			statuses = registry.Health(ctx)
		}
		out := make([]datasourceJSON, len(statuses))
		for i, s := range statuses {
			out[i] = datasourceJSON{Status: s, DashboardCount: counts[s.UID]}
		}
		c.JSON(http.StatusOK, datasourcesResponse{Datasources: out})
	})
}
