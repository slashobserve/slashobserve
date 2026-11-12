package grantmode

import "context"

type Mode string

const (
	Direct  Mode = "direct"
	Propose Mode = "propose"
)

type contextKey struct{}

func Parse(s string) Mode {
	if s == string(Propose) {
		return Propose
	}
	return Direct
}

func With(ctx context.Context, m Mode) context.Context {
	return context.WithValue(ctx, contextKey{}, m)
}

func From(ctx context.Context) Mode {
	if m, ok := ctx.Value(contextKey{}).(Mode); ok && m != "" {
		return m
	}
	return Direct
}
