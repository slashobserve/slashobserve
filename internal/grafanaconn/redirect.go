package grafanaconn

import (
	"context"
	"errors"
	"net/http"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

type RequestPrincipal func(*http.Request) (auth.Principal, bool, error)

func (s *Service) DashboardRedirect(principalOf RequestPrincipal) func(*http.Request, string) (string, bool) {
	return func(r *http.Request, uid string) (string, bool) {
		ctx := r.Context()
		by, signedIn, err := principalOf(r)
		if err != nil {
			s.redirectFailed(ctx, err)
			return "", false
		}
		if !signedIn {
			return "", false
		}
		id, err := s.Resolve(ctx, by, uid)
		if err != nil {
			if !errors.Is(err, ErrUnknownUID) {
				s.redirectFailed(ctx, err)
			}
			return "", false
		}
		return dashboardPath(id.String()), true
	}
}

func (s *Service) redirectFailed(ctx context.Context, err error) {
	if clientgone.Is(ctx, err) {
		return
	}
	s.logger.ErrorContext(ctx, "redirect grafana dashboard address", logattrs.Failure("grafana_redirect_failed", err)...)
}
