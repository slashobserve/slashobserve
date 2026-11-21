package revisions

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func (s *Service) ShowsRunAnswer(ctx context.Context, id pgtype.UUID, by auth.Principal) (bool, error) {
	row, _, err := s.resolveObserve(ctx, id, by)
	switch {
	case errors.Is(err, ErrRequestNotFound):
		return false, nil
	case err != nil:
		return false, err
	}
	return findingShown(s.viewOf(ctx, row, by), row), nil
}
