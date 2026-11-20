package datasource

import (
	"context"
	"errors"

	"github.com/slashobserve/slashobserve/internal/auth"
)

type Audience string

const (
	AudienceEveryone Audience = "everyone"
	AudienceAdmins   Audience = "admins"
	AudienceCaller   Audience = ""
)

var ErrNotVisible = errors.New("datasource not visible")

type Access struct {
	By       auth.Principal
	Audience Audience
	System   bool
}

type accessKey struct{}

func WithAccess(ctx context.Context, a Access) context.Context {
	return context.WithValue(ctx, accessKey{}, a)
}

func AccessFrom(ctx context.Context) (Access, bool) {
	a, ok := ctx.Value(accessKey{}).(Access)
	return a, ok
}

func (a Access) reads(audience Audience) bool {
	if a.System || audience != AudienceAdmins {
		return true
	}
	return a.By.Role == auth.RoleAdmin && a.Audience != AudienceEveryone
}

func ReadsRestricted(ctx context.Context) bool {
	a, ok := AccessFrom(ctx)
	return ok && a.reads(AudienceAdmins)
}
