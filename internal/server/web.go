package server

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:webdist
var webFiles embed.FS

const (
	assetsDir = "assets/"
	brotliExt = ".br"
)

const webNotBuilt = "SlashObserve web UI is not built into this binary; run make build.\n"

func registerWebRoutes(engine *gin.Engine, redirects Redirects) {
	dist, err := fs.Sub(webFiles, "webdist")
	if err != nil {
		panic(fmt.Sprintf("embedded web assets: %v", err))
	}
	registerWebFiles(engine, redirects, dist)
}

func registerWebFiles(engine *gin.Engine, redirects Redirects, dist fs.FS) {
	index, indexErr := fs.ReadFile(dist, "index.html")

	engine.NoRoute(func(c *gin.Context) {
		urlPath := c.Request.URL.Path
		if isServerPath(urlPath) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found", "code": "not_found"})
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found", "code": "not_found"})
			return
		}
		if redirects.serve(c) {
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
		if isServableFile(dist, name) {
			cacheControl := "no-cache"
			if strings.HasPrefix(name, assetsDir) {
				cacheControl = "public, max-age=31536000, immutable"
			}
			if err := serveFile(c, dist, name, cacheControl); err != nil {
				c.String(http.StatusInternalServerError, "internal server error\n")
			}
			return
		}
		if strings.HasPrefix(name, assetsDir) || isTopLevelFileName(name) {
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

var serverPathPrefixes = []string{"/api", "/oauth", "/.well-known"}

func isServerPath(urlPath string) bool {
	for _, prefix := range serverPathPrefixes {
		if urlPath == prefix || strings.HasPrefix(urlPath, prefix+"/") {
			return true
		}
	}
	return false
}

func serveFile(c *gin.Context, fsys fs.FS, name, cacheControl string) error {
	file, encoded := openForRequest(c.Request, fsys, name)
	if file == nil {
		var err error
		if file, err = fsys.Open(name); err != nil {
			return err
		}
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	content, ok := file.(io.ReadSeeker)
	if !ok {
		return fmt.Errorf("embedded file %q is not seekable", name)
	}
	c.Header("Cache-Control", cacheControl)
	if strings.HasPrefix(name, assetsDir) {
		c.Header("Vary", "Accept-Encoding")
	}
	if encoded {
		c.Header("Content-Encoding", "br")
		c.Header("Content-Type", mime.TypeByExtension(path.Ext(name)))
	}
	http.ServeContent(c.Writer, c.Request, name, info.ModTime(), content)
	return nil
}

func openForRequest(r *http.Request, fsys fs.FS, name string) (fs.File, bool) {
	if !strings.HasPrefix(name, assetsDir) || !acceptsBrotli(r.Header.Values("Accept-Encoding")) {
		return nil, false
	}
	file, err := fsys.Open(name + brotliExt)
	if err != nil {
		return nil, false
	}
	return file, true
}

func acceptsBrotli(values []string) bool {
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			coding, params, _ := strings.Cut(item, ";")
			if strings.TrimSpace(coding) != "br" {
				continue
			}
			_, quality, _ := strings.Cut(params, "=")
			if q, err := strconv.ParseFloat(strings.TrimSpace(quality), 64); err == nil && q == 0 {
				return false
			}
			return true
		}
	}
	return false
}

func isServableFile(fsys fs.FS, name string) bool {
	if name == "" || name == "." || name == "index.html" || path.Ext(name) == brotliExt {
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

func isTopLevelFileName(name string) bool {
	return name != "index.html" && !strings.Contains(name, "/") && path.Ext(name) != ""
}
