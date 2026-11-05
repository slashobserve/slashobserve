package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/db"
)

type Redirects struct {
	Resolve func(ctx context.Context, kind, id string) (string, bool)
}

const resolvableDashboard = "dashboard"

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
	target, status, ok := r.target(c.Request.Context(), c.Request.URL.EscapedPath())
	if !ok {
		return false
	}
	if query := c.Request.URL.RawQuery; query != "" {
		target += "?" + query
	}
	c.Redirect(status, target)
	return true
}

func (r Redirects) target(ctx context.Context, escapedPath string) (string, int, bool) {
	segments := strings.Split(strings.TrimPrefix(escapedPath, "/"), "/")
	if len(segments) < 2 {
		return "", 0, false
	}
	id := segments[1]
	if form, ok := oldForms[segments[0]]; ok && isID(id) && (form.keepsSubpath || len(segments) == 2) {
		return "/" + form.short + "/" + strings.Join(segments[1:], "/"), http.StatusMovedPermanently, true
	}
	if segments[0] == "d" && !isID(id) && id != "" && r.Resolve != nil {
		uid, err := url.PathUnescape(id)
		if err != nil {
			return "", 0, false
		}
		if target, ok := r.Resolve(ctx, resolvableDashboard, uid); ok {
			return target, http.StatusFound, true
		}
	}
	return "", 0, false
}

func isID(segment string) bool {
	_, err := db.ParseUUID(segment)
	return err == nil
}
