package server

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:webdist
var webFiles embed.FS

const webNotBuilt = "SlashObserve web UI is not built into this binary; run make build.\n"

func registerWebRoutes(engine *gin.Engine) {
	dist, err := fs.Sub(webFiles, "webdist")
	if err != nil {
		panic(fmt.Sprintf("embedded web assets: %v", err))
	}
	index, indexErr := fs.ReadFile(dist, "index.html")

	engine.NoRoute(func(c *gin.Context) {
		urlPath := c.Request.URL.Path
		if urlPath == "/api" || strings.HasPrefix(urlPath, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
		if isServableFile(dist, name) {
			if strings.HasPrefix(name, "assets/") {
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				c.Header("Cache-Control", "no-cache")
			}
			http.ServeFileFS(c.Writer, c.Request, dist, name)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			c.String(http.StatusNotFound, "not found\n")
			return
		}
		if indexErr != nil {
			c.String(http.StatusServiceUnavailable, webNotBuilt)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
}

func isServableFile(fsys fs.FS, name string) bool {
	if name == "" || name == "." || name == "index.html" {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	info, err := fs.Stat(fsys, name)
	return err == nil && !info.IsDir()
}
