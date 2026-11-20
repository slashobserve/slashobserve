package proxyhttp

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func as(ctx context.Context, p auth.Principal) context.Context {
	return datasource.WithAccess(auth.WithPrincipal(ctx, p), datasource.Access{By: p})
}

func signIn(c *gin.Context, p auth.Principal) {
	c.Request = c.Request.WithContext(as(c.Request.Context(), p))
}
