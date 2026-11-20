package threads

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func ownMessage(row dbsqlc.ThreadMessage, by auth.Principal) bool {
	return !row.RunID.Valid && row.AuthorKind == string(by.Kind) && row.AuthorID.String() == by.ID
}

func mayDelete(row dbsqlc.ThreadMessage, by auth.Principal, sc scope, othersWrote bool) bool {
	switch {
	case !sc.abilities.Comment:
		return false
	case sc.abilities.Manage:
		return true
	}
	return ownMessage(row, by) && !othersWrote
}

func (s *Service) threadsWithOtherAuthors(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, threadIDs []pgtype.UUID) (map[pgtype.UUID]bool, error) {
	if len(threadIDs) == 0 {
		return nil, nil
	}
	authorID, err := db.ParseUUID(by.ID)
	if err != nil {
		return nil, fmt.Errorf("author id: %w", err)
	}
	found, err := q.ThreadsWithOtherAuthors(ctx, dbsqlc.ThreadsWithOtherAuthorsParams{ThreadIds: threadIDs, AuthorKind: string(by.Kind), AuthorID: authorID})
	if err != nil {
		return nil, fmt.Errorf("list threads with other authors: %w", err)
	}
	others := make(map[pgtype.UUID]bool, len(found))
	for _, id := range found {
		others[id] = true
	}
	return others, nil
}

func (s *Service) canDeleteOne(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.ThreadMessage, by auth.Principal, sc scope) (bool, error) {
	if !sc.abilities.Comment {
		return false, nil
	}
	if sc.abilities.Manage || !ownMessage(row, by) || row.ID != row.ThreadID {
		return mayDelete(row, by, sc, false), nil
	}
	others, err := s.threadsWithOtherAuthors(ctx, q, by, []pgtype.UUID{row.ThreadID})
	if err != nil {
		return false, err
	}
	return mayDelete(row, by, sc, others[row.ThreadID]), nil
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, messageID pgtype.UUID) error {
	messageRow, err := s.db.Queries().GetMessage(ctx, messageID)
	if err != nil {
		return threadLookup(err)
	}
	row, sc, err := s.threadAndScope(ctx, by, messageRow.ThreadID)
	if err != nil {
		return err
	}
	if !sc.abilities.Comment {
		return folders.ErrForbidden
	}
	root := messageRow.ID == row.ID
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := lockTarget(ctx, q, sc); err != nil {
			return err
		}
		current, err := q.GetMessage(ctx, messageID)
		if err != nil {
			return threadLookup(err)
		}
		allowed, err := s.canDeleteOne(ctx, q, current, by, sc)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrCannotDelete
		}
		if root {
			err = q.DeleteThread(ctx, row.ID)
		} else {
			err = q.DeleteMessage(ctx, messageID)
		}
		if err != nil {
			return fmt.Errorf("delete message: %w", err)
		}
		return emitter{scope: sc, thread: row, actor: actorOf(ctx, by)}.deleted(ctx, q, messageID)
	})
	return threadGone(err)
}

func (s *Service) otherAuthorsOnPage(ctx context.Context, by auth.Principal, sc scope, rows []dbsqlc.ListThreadMessagesRow) (map[pgtype.UUID]bool, error) {
	if sc.abilities.Manage || !sc.abilities.Comment {
		return nil, nil
	}
	var roots []pgtype.UUID
	for _, r := range rows {
		if r.ThreadMessage.ID == r.Thread.ID && ownMessage(r.ThreadMessage, by) {
			roots = append(roots, r.Thread.ID)
		}
	}
	return s.threadsWithOtherAuthors(ctx, s.db.Queries(), by, roots)
}
