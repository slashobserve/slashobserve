package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func RevalidatedJSON(c *gin.Context, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		_ = c.Error(err)
		AbortError(c, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	sum := sha256.Sum256(raw)
	tag := `W/"` + hex.EncodeToString(sum[:16]) + `"`
	c.Header("Cache-Control", "private, no-cache")
	c.Header("Vary", "Authorization, Cookie")
	c.Header("ETag", tag)
	if matchesTag(c.GetHeader("If-None-Match"), tag) {
		c.Status(http.StatusNotModified)
		c.Writer.WriteHeaderNow()
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}

func matchesTag(header, tag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(tag, "W/") {
			return true
		}
	}
	return false
}
