package auth

import "context"

type PrincipalKind string

const (
	KindUser           PrincipalKind = "user"
	KindServiceAccount PrincipalKind = "service_account"
)

type Principal struct {
	Kind   PrincipalKind `json:"kind"`
	ID     string        `json:"id"`
	Email  string        `json:"email,omitempty"`
	Name   string        `json:"name"`
	Role   Role          `json:"role"`
	Groups []string      `json:"-"`
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
