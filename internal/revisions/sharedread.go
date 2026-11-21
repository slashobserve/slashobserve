package revisions

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type ThreadReader func(ctx context.Context, by auth.Principal, messageID pgtype.UUID) error

func (s *Service) ReadThreadsWith(read ThreadReader) {
	s.threadReads = read
}

func (s *Service) sharedWith(ctx context.Context, id pgtype.UUID, by auth.Principal) (dbsqlc.AgentRequest, bool, error) {
	if s.threadReads == nil {
		return dbsqlc.AgentRequest{}, false, nil
	}
	row, err := s.GetByID(ctx, id)
	if err != nil {
		return dbsqlc.AgentRequest{}, false, err
	}
	if !PrivateAsk(row) || !row.ThreadMessageID.Valid || s.threadReads(ctx, by, row.ThreadMessageID) != nil {
		return dbsqlc.AgentRequest{}, false, nil
	}
	return row, true, nil
}
