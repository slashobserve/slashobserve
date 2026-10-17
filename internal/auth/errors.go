package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func abortError(c *gin.Context, status int, code, message string) {
	if status == http.StatusUnauthorized {
		c.Header("WWW-Authenticate", `Bearer realm="slashobserve"`)
	}
	c.AbortWithStatusJSON(status, gin.H{"error": message, "code": code})
}
