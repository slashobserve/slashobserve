package observations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
)

const defaultRunTitle = "Investigation"

type RunOpen struct {
	Run           dbsqlc.AgentRequest
	By            auth.Principal
	ObservationID pgtype.UUID
	Strict        bool
}

type RunResult struct {
	RunID    pgtype.UUID
	Title    string
	Markdown string
	Evidence []json.RawMessage
	From, To time.Time
}

type runBody struct {
	RunID string   `json:"run_id"`
	Cites []string `json:"cites"`
}

type runPlace struct {
	observation pgtype.UUID
	folder      pgtype.UUID
	abilities   folders.Abilities
}

func (s *Service) OpenForRun(ctx context.Context, q *dbsqlc.Queries, in RunOpen) (pgtype.UUID, error) {
	for range maxFolderRetries {
		id, err := s.openForRun(ctx, q, in)
		if !errors.Is(err, errFolderMoved) {
			return id, err
		}
	}
	return pgtype.UUID{}, fmt.Errorf("observation changed folders %d times in a row: %w", maxFolderRetries, errFolderMoved)
}

func (s *Service) openForRun(ctx context.Context, q *dbsqlc.Queries, in RunOpen) (pgtype.UUID, error) {
	place, ok, err := s.placeRun(ctx, in)
	if err != nil || !ok {
		return pgtype.UUID{}, err
	}
	state, ok := addedState(place.abilities)
	if !ok {
		return pgtype.UUID{}, folders.ErrForbidden
	}
	who := actor.Of(in.By, origin.From(ctx), in.Run.ID.String())
	parent, err := s.runParent(ctx, q, in, place, who)
	if err != nil {
		return pgtype.UUID{}, err
	}
	body, err := json.Marshal(runBody{RunID: in.Run.ID.String(), Cites: []string{}})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("encode run block: %w", err)
	}
	_, err = insertBlock(ctx, q, parent, preparedBlock{kind: KindRun, body: body, width: WidthFull}, state, who)
	switch {
	case errors.Is(err, ErrTooManyBlocks):
		s.logger.WarnContext(ctx, "run block not added: the observation is full",
			slog.String("agent_request_id", in.Run.ID.String()), slog.String("observation_id", parent.ID.String()))
		return pgtype.UUID{}, nil
	case err != nil:
		return pgtype.UUID{}, err
	}
	return parent.ID, nil
}

func (s *Service) placeRun(ctx context.Context, in RunOpen) (runPlace, bool, error) {
	if in.ObservationID.Valid {
		row, _, abilities, err := s.open(ctx, in.By, in.ObservationID)
		switch {
		case errors.Is(err, ErrNotFound):
			if in.Strict {
				return runPlace{}, false, err
			}
		case err != nil:
			return runPlace{}, false, err
		case !abilities.Comment:
			if in.Strict {
				return runPlace{}, false, folders.ErrForbidden
			}
		default:
			return runPlace{observation: row.ID, folder: row.FolderID, abilities: abilities}, true, nil
		}
	}
	if in.Run.DashboardID.Valid {
		access, err := s.access.Dashboard(ctx, in.By, in.Run.DashboardID)
		switch {
		case err == nil && access.Abilities.Edit:
			return runPlace{folder: access.Folder.ID, abilities: access.Abilities}, true, nil
		case err == nil, errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess):
		default:
			return runPlace{}, false, err
		}
	}
	folder, err := s.access.CreationFolder(ctx, in.By, pgtype.UUID{})
	switch {
	case errors.Is(err, folders.ErrForbidden):
		return runPlace{}, false, nil
	case err != nil:
		return runPlace{}, false, err
	}
	_, abilities, err := s.access.Folder(ctx, in.By, folder.ID)
	if err != nil {
		return runPlace{}, false, err
	}
	return runPlace{folder: folder.ID, abilities: abilities}, true, nil
}

func (s *Service) runParent(ctx context.Context, q *dbsqlc.Queries, in RunOpen, place runPlace, who actor.Actor) (dbsqlc.Observation, error) {
	if place.observation.Valid {
		locked, err := q.LockObservation(ctx, place.observation)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return dbsqlc.Observation{}, ErrNotFound
		case err != nil:
			return dbsqlc.Observation{}, fmt.Errorf("lock observation: %w", err)
		case locked.FolderID != place.folder:
			return dbsqlc.Observation{}, errFolderMoved
		}
		return locked, nil
	}
	cols, err := who.Columns()
	if err != nil {
		return dbsqlc.Observation{}, err
	}
	created, err := q.CreateObservation(ctx, dbsqlc.CreateObservationParams{
		FolderID:      place.folder,
		Title:         runTitle(in.Run.Prompt),
		TimeFrom:      in.Run.ContextFrom,
		TimeTo:        in.Run.ContextTo,
		CreatedByKind: cols.Kind,
		CreatedByID:   cols.ID,
		CreatedByName: cols.Name,
		Origin:        cols.Origin,
		OriginClient:  cols.Client,
		RunID:         cols.RunID,
	})
	if err != nil {
		return dbsqlc.Observation{}, fmt.Errorf("create observation: %w", err)
	}
	if err := appendEvent(ctx, q, activity.TypeObservationCreated, who, created.ID, eventData{Title: created.Title}); err != nil {
		return dbsqlc.Observation{}, err
	}
	return created, nil
}

func runTitle(prompt string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(prompt), "\n")
	title, err := validateTitle(cutRunes(first, MaxTitleRunes))
	if err != nil {
		return defaultRunTitle
	}
	return title
}

func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func (s *Service) RecordRunResult(ctx context.Context, by auth.Principal, in RunResult) error {
	observationID, err := s.store.db.Queries().RunObservation(ctx, in.RunID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("read run observation: %w", err)
	case !observationID.Valid:
		return nil
	}
	return s.write(ctx, by, observationID, in.RunID, func(q *dbsqlc.Queries, parent dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		written, err := q.RunHasTextBlock(ctx, dbsqlc.RunHasTextBlockParams{ObservationID: parent.ID, RunID: in.RunID})
		if err != nil {
			return fmt.Errorf("check run result: %w", err)
		}
		if written {
			return nil
		}
		state, ok := addedState(abilities)
		if !ok {
			return folders.ErrForbidden
		}
		added, err := s.insertRunBlocks(ctx, q, parent, in, state, who)
		if err != nil {
			return err
		}
		return citeBlocks(ctx, q, parent, in.RunID, added, who)
	})
}

func (s *Service) insertRunBlocks(ctx context.Context, q *dbsqlc.Queries, parent dbsqlc.Observation, in RunResult, state BlockState, who actor.Actor) ([]string, error) {
	text, err := json.Marshal(TextBody{Markdown: cutRunes("## "+in.Title+"\n\n"+in.Markdown, MaxTextRunes)})
	if err != nil {
		return nil, fmt.Errorf("encode text block: %w", err)
	}
	blocks := []preparedBlock{{kind: KindText, body: text, width: WidthFull}}
	blocks = append(blocks, s.runPanels(ctx, in)...)
	var added []string
	for _, b := range blocks {
		row, err := insertBlock(ctx, q, parent, b, state, who)
		switch {
		case errors.Is(err, ErrTooManyPanels), errors.Is(err, ErrTooManyBlocks):
			if b.kind == KindText {
				return nil, err
			}
			s.logger.WarnContext(ctx, "run evidence not added: the observation is full",
				slog.String("agent_request_id", in.RunID.String()), slog.String("observation_id", parent.ID.String()))
			return added, nil
		case err != nil:
			return nil, err
		}
		added = append(added, row.ID.String())
	}
	return added, nil
}

func (s *Service) runPanels(ctx context.Context, in RunResult) []preparedBlock {
	var out []preparedBlock
	for i, panel := range in.Evidence {
		raw, err := json.Marshal(PanelBody{Panel: panel, From: &in.From, To: &in.To})
		if err == nil {
			raw, err = normalBody(KindPanel, raw)
		}
		if err != nil {
			s.logger.WarnContext(ctx, "run evidence panel skipped",
				logattrs.Failure("invalid_panel", err, slog.String("agent_request_id", in.RunID.String()), slog.Int("evidence", i))...)
			continue
		}
		out = append(out, preparedBlock{kind: KindPanel, body: raw, width: WidthFull})
	}
	return out
}

func citeBlocks(ctx context.Context, q *dbsqlc.Queries, parent dbsqlc.Observation, runID pgtype.UUID, cited []string, who actor.Actor) error {
	run, err := q.RunBlockOfRun(ctx, dbsqlc.RunBlockOfRunParams{ObservationID: parent.ID, RunID: runID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("read run block: %w", err)
	}
	body, err := json.Marshal(runBody{RunID: runID.String(), Cites: cited})
	if err != nil {
		return fmt.Errorf("encode run block: %w", err)
	}
	updated, err := q.UpdateBlock(ctx, dbsqlc.UpdateBlockParams{
		ID: run.ID, Version: run.Version, FromStates: BlockMachine.From(BlockUpdate),
		Body: body, Width: run.Width, Position: run.Position,
	})
	if err != nil {
		return fmt.Errorf("update run block: %w", err)
	}
	return appendEvent(ctx, q, activity.TypeObservationBlockUpdated, who, parent.ID, blockEvent(parent, updated))
}
