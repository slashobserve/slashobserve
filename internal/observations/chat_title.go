package observations

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var errTitleKept = errors.New("the observation keeps its title")

func chatObservation(ctx context.Context, q *dbsqlc.Queries, link dbsqlc.ChatObservationsParams) (dbsqlc.ChatObservationsRow, bool, error) {
	rows, err := q.ChatObservations(ctx, link)
	if err != nil {
		return dbsqlc.ChatObservationsRow{}, false, fmt.Errorf("find the observation made from a chat: %w", err)
	}
	if len(rows) != 1 {
		return dbsqlc.ChatObservationsRow{}, false, nil
	}
	return rows[0], true, nil
}

func renameChat(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, id pgtype.UUID, title string) (revisions.SharedTitle, error) {
	link, found, err := chatObservation(ctx, q, dbsqlc.ChatObservationsParams{ObservationID: id})
	if err != nil || !found {
		return "", err
	}
	if link.RequestedByKind != string(by.Kind) || link.RequestedByID.String() != by.ID {
		return revisions.TitleKept, nil
	}
	cleaned, err := revisions.CheckTitle(title)
	if err != nil {
		return revisions.TitleKept, nil
	}
	params := dbsqlc.UpdateAgentRequestListFieldsParams{ID: link.ChatID, SetTitle: true, Title: pgtype.Text{String: cleaned, Valid: true}}
	if _, err := q.UpdateAgentRequestListFields(ctx, params); err != nil {
		return "", fmt.Errorf("rename the chat of an observation: %w", err)
	}
	return revisions.TitleRenamed, nil
}

func (s *Service) RenameFromChat(ctx context.Context, by auth.Principal, chatID pgtype.UUID, title string) (revisions.SharedTitle, error) {
	link, found, err := chatObservation(ctx, s.store.db.Queries(), dbsqlc.ChatObservationsParams{ChatID: chatID})
	if err != nil || !found {
		return "", err
	}
	if title, err = validateTitle(title); err != nil {
		return revisions.TitleKept, nil
	}
	err = s.write(ctx, by, link.ObservationID, pgtype.UUID{}, func(q *dbsqlc.Queries, row dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		if permit(Machine, State(row.State), ActionUpdate, can(abilities, false)) != nil {
			return errTitleKept
		}
		if row.Title == title {
			return nil
		}
		return s.saveUpdate(ctx, q, row, who, dbsqlc.UpdateObservationParams{ID: row.ID, Title: title, TimeFrom: row.TimeFrom, TimeTo: row.TimeTo})
	})
	switch {
	case errors.Is(err, ErrNotFound):
		return "", nil
	case errors.Is(err, errTitleKept):
		return revisions.TitleKept, nil
	case err != nil:
		return "", err
	}
	return revisions.TitleRenamed, nil
}

func (s *Service) Visible(ctx context.Context, by auth.Principal, id pgtype.UUID) (bool, error) {
	_, _, _, err := s.open(ctx, by, id)
	switch {
	case errors.Is(err, ErrNotFound):
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}
