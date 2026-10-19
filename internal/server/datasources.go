package server

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

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

func registerDatasourceRoutes(api *gin.RouterGroup, registry *datasource.Registry, usage DatasourceUsage, logger *slog.Logger) {
	api.GET("/datasources", func(c *gin.Context) {
		statuses := []datasource.Status{}
		if registry != nil {
			statuses = registry.Health(c.Request.Context())
		}
		var counts map[string]int
		if usage != nil {
			var err error
			if counts, err = usage.DatasourceUsage(c.Request.Context()); err != nil {
				logger.ErrorContext(c.Request.Context(), "count dashboards per datasource", logattrs.Failure("datasource_usage_failed", err)...)
				httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
				return
			}
		}
		out := make([]datasourceJSON, len(statuses))
		for i, s := range statuses {
			out[i] = datasourceJSON{Status: s, DashboardCount: counts[s.UID]}
		}
		c.JSON(http.StatusOK, datasourcesResponse{Datasources: out})
	})
}
