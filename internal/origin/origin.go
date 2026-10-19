package origin

import (
	"context"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
)

type Kind string

const (
	Web Kind = "web"
	CLI Kind = "cli"
	MCP Kind = "mcp"
	API Kind = "api"
)

const MaxClientRunes = 64

type Origin struct {
	Kind   Kind
	Client string
}

type contextKey struct{}

func With(ctx context.Context, o Origin) context.Context {
	return context.WithValue(ctx, contextKey{}, o)
}

func From(ctx context.Context) Origin {
	o, _ := ctx.Value(contextKey{}).(Origin)
	return o
}

func CleanClient(name string) string {
	name = strings.ToValidUTF8(name, "")
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if runes := []rune(name); len(runes) > MaxClientRunes {
		name = string(runes[:MaxClientRunes])
	}
	return strings.TrimSpace(name)
}

func ForPrincipal(p auth.Principal, userAgent string) Origin {
	switch {
	case p.Kind == auth.KindUser:
		return Origin{Kind: Web}
	case strings.HasPrefix(userAgent, "observe/"):
		return Origin{Kind: CLI}
	default:
		return Origin{Kind: API}
	}
}

func Middleware(c *gin.Context) {
	ctx := c.Request.Context()
	if p, ok := auth.FromContext(ctx); ok {
		c.Request = c.Request.WithContext(With(ctx, ForPrincipal(p, c.GetHeader("User-Agent"))))
	}
	c.Next()
}
