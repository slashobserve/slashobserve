package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/db"
)

type Redirects struct {
	GrafanaDashboard func(r *http.Request, grafanaUID string) (string, bool)
}

type oldForm struct {
	short        string
	keepsSubpath bool
}

var oldForms = map[string]oldForm{
	"dashboards": {short: "d", keepsSubpath: true},
	"folders":    {short: "f"},
	"observe":    {short: "c"},
	"builds":     {short: "b"},
}

func (r Redirects) serve(c *gin.Context) bool {
	target, status, ok := r.target(c.Request)
	if !ok {
		return false
	}
	if status == http.StatusFound {
		c.Header("Cache-Control", "no-store")
	}
	if query := c.Request.URL.RawQuery; query != "" {
		target += "?" + query
	}
	c.Redirect(status, target)
	return true
}

func (r Redirects) target(req *http.Request) (string, int, bool) {
	segments := strings.Split(strings.TrimPrefix(req.URL.EscapedPath(), "/"), "/")
	if len(segments) < 2 {
		return "", 0, false
	}
	id := segments[1]
	if form, ok := oldForms[segments[0]]; ok && isID(id) && (form.keepsSubpath || len(segments) == 2) {
		return "/" + form.short + "/" + strings.Join(segments[1:], "/"), http.StatusMovedPermanently, true
	}
	if segments[0] == "d" && !isID(id) && id != "" && r.GrafanaDashboard != nil {
		uid, err := url.PathUnescape(id)
		if err != nil {
			return "", 0, false
		}
		if target, ok := r.GrafanaDashboard(req, uid); ok {
			if slug := strings.Join(segments[2:], "/"); slug != "" {
				target += "/" + slug
			}
			return target, http.StatusFound, true
		}
	}
	return "", 0, false
}

func isID(segment string) bool {
	_, err := db.ParseUUID(segment)
	return err == nil
}
