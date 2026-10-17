package auth

import "context"

func (s *Service) BearerPrincipal(ctx context.Context, header string) (Principal, bool, error) {
	return s.bearerPrincipal(ctx, header)
}
