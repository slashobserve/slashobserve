package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type instanceResponse struct {
	PublicURL string `json:"public_url"`
	MCPURL    string `json:"mcp_url"`
}

func (s *Service) instance(c *gin.Context) {
	c.JSON(http.StatusOK, instanceResponse{
		PublicURL: s.cfg.PublicURL.String(),
		MCPURL:    s.cfg.PublicURL.JoinPath("mcp").String(),
	})
}
