package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type datasourcesResponse struct {
	Datasources []datasource.Status `json:"datasources"`
}

func registerDatasourceRoutes(api *gin.RouterGroup, registry *datasource.Registry) {
	api.GET("/datasources", func(c *gin.Context) {
		statuses := []datasource.Status{}
		if registry != nil {
			statuses = registry.Health(c.Request.Context())
		}
		c.JSON(http.StatusOK, datasourcesResponse{Datasources: statuses})
	})
}
