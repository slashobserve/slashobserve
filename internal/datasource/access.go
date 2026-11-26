package datasource

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/slashobserve/slashobserve/internal/auth"
)

type Audience string

const (
	AudienceEveryone Audience = "everyone"
	AudienceAdmins   Audience = "admins"
	AudienceCaller   Audience = ""
)

var ErrNotVisible = errors.New("datasource not visible")

const WithheldNote = "Audit data, other datasources only Admins can query and what is stored in folders only Admins can read are available in a private chat, or in a folder only Admins can read. " +
	"What this run writes is read by people who are not Admins, so they are not available to it."

type withheld struct{}

func (withheld) Error() string { return WithheldNote }

func (withheld) Is(target error) bool { return target == ErrNotVisible }

var ErrWithheld error = withheld{}

var ErrNoAccess = errors.New("no datasource access in the context: the entry point did not call datasource.WithAccess")

func RequireAccess(ctx context.Context) error {
	if _, ok := AccessFrom(ctx); !ok {
		return ErrNoAccess
	}
	return nil
}

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

func (a Access) Reads(audience Audience) bool {
	if a.System || audience != AudienceAdmins {
		return true
	}
	return a.By.Role == auth.RoleAdmin && a.Audience != AudienceEveryone
}

func (a Access) Refusal(audience Audience) error {
	if audience == AudienceAdmins && a.By.Role == auth.RoleAdmin && a.Audience == AudienceEveryone {
		return ErrWithheld
	}
	return ErrNotVisible
}

type restrictedReads struct{ read atomic.Bool }

type restrictedReadsKey struct{}

func NotingRestrictedReads(ctx context.Context) (context.Context, func() bool) {
	n := &restrictedReads{}
	return context.WithValue(ctx, restrictedReadsKey{}, n), n.read.Load
}

func NoteRead(ctx context.Context, audience Audience) {
	if a, ok := AccessFrom(ctx); ok {
		noteRestrictedRead(ctx, a, audience)
	}
}

func noteRestrictedRead(ctx context.Context, a Access, audience Audience) {
	if a.System || audience != AudienceAdmins {
		return
	}
	if n, ok := ctx.Value(restrictedReadsKey{}).(*restrictedReads); ok {
		n.read.Store(true)
	}
}

func ReadsRestricted(ctx context.Context) bool {
	a, ok := AccessFrom(ctx)
	return ok && a.Reads(AudienceAdmins)
}
