package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RequireRole(min Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p, ok := FromContext(c.Request.Context()); !ok || !p.Role.AtLeast(min) {
			abortError(c, http.StatusForbidden, "forbidden_role", "your role does not allow this")
			return
		}
		c.Next()
	}
}

type Routes struct {
	group *gin.RouterGroup
}

func NewRoutes(g *gin.RouterGroup) Routes {
	return Routes{group: g}
}

func (r Routes) Use(handlers ...gin.HandlerFunc) {
	r.group.Use(handlers...)
}

func (r Routes) GET(path string, min Role, handlers ...gin.HandlerFunc) {
	r.handle(http.MethodGet, path, min, handlers)
}

func (r Routes) POST(path string, min Role, handlers ...gin.HandlerFunc) {
	r.handle(http.MethodPost, path, min, handlers)
}

func (r Routes) PUT(path string, min Role, handlers ...gin.HandlerFunc) {
	r.handle(http.MethodPut, path, min, handlers)
}

func (r Routes) PATCH(path string, min Role, handlers ...gin.HandlerFunc) {
	r.handle(http.MethodPatch, path, min, handlers)
}

func (r Routes) DELETE(path string, min Role, handlers ...gin.HandlerFunc) {
	r.handle(http.MethodDelete, path, min, handlers)
}

func (r Routes) Group(path string, handlers ...gin.HandlerFunc) Routes {
	return Routes{group: r.group.Group(path, handlers...)}
}

func (r Routes) handle(method, path string, min Role, handlers []gin.HandlerFunc) {
	r.group.Handle(method, path, append([]gin.HandlerFunc{RequireRole(min)}, handlers...)...)
}
