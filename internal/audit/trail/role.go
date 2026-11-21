package trail

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
)

const RoleCheckTimeout = 5 * time.Second

var ErrNoLongerAdmin = errors.New("the caller is no longer an Admin")

type Principals interface {
	RequestPrincipal(r *http.Request) (auth.Principal, bool, error)
}

func StillAdmin(r *http.Request, principals Principals) (bool, error) {
	ctx, cancel := context.WithTimeout(r.Context(), RoleCheckTimeout)
	defer cancel()
	principal, ok, err := principals.RequestPrincipal(r.WithContext(ctx))
	if err != nil {
		return false, err
	}
	return ok && principal.Role.AtLeast(auth.RoleAdmin), nil
}
