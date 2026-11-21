package threads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	guard "github.com/slashobserve/slashobserve/internal/placement"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const actionRunShare audit.Action = "run.share"

const CodeWrongThread = "invalid_thread"

var (
	ErrRunNotFound  = errors.New("agent request not found")
	ErrNotShareable = errors.New("only a private question about a dashboard or an observation, asked in a chat, can be shared")
	ErrNoAnswer     = errors.New("the request has no answer to share")

	errAlreadyShared = errors.New("the request was shared by another call")
	errWrongThread   = &InvalidInputError{Code: CodeWrongThread, Message: "thread_id must be a thread on the dashboard or observation the question was about"}
)

type AnswerShower func(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, requestID, observationID pgtype.UUID) error

type AnswerHider func(ctx context.Context, q *dbsqlc.Queries, requestID, observationID pgtype.UUID) error

func (s *Service) ShowAnswersWith(show AnswerShower, hide AnswerHider) {
	s.showAnswer, s.hideAnswer = show, hide
}

type ShareInput struct {
	RunID    pgtype.UUID
	ThreadID pgtype.UUID
}

type Shared struct {
	Thread   Thread
	Message  Message
	Replayed bool
}

type share struct {
	by        auth.Principal
	run       dbsqlc.AgentRequest
	subject   Target
	evidence  []byte
	onThePage bool
}

func (s *Service) ShareRun(ctx context.Context, by auth.Principal, in ShareInput) (Shared, error) {
	run, err := s.db.Queries().GetAgentRequestByID(ctx, in.RunID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Shared{}, ErrRunNotFound
	case err != nil:
		return Shared{}, fmt.Errorf("read agent request: %w", err)
	case run.RequestedByKind != string(by.Kind) || run.RequestedByID.String() != by.ID:
		return Shared{}, ErrRunNotFound
	case !revisions.PrivateAsk(run):
		return Shared{}, ErrNotShareable
	}
	if run.ThreadMessageID.Valid {
		return s.sharedBefore(ctx, by, run)
	}
	sh, err := s.shareOf(ctx, by, run, !in.ThreadID.Valid)
	if err != nil {
		return Shared{}, err
	}
	w, err := s.postShare(ctx, sh, in.ThreadID)
	if errors.Is(err, errAlreadyShared) {
		if run, err = s.db.Queries().GetAgentRequestByID(ctx, in.RunID); err != nil {
			return Shared{}, fmt.Errorf("read agent request: %w", err)
		}
		return s.sharedBefore(ctx, by, run)
	}
	if err != nil {
		return Shared{}, err
	}
	return Shared{Thread: w.thread, Message: w.message}, nil
}

func (s *Service) shareOf(ctx context.Context, by auth.Principal, run dbsqlc.AgentRequest, onTheSubject bool) (share, error) {
	sh := share{by: by, run: run}
	switch {
	case run.DashboardID.Valid:
		sh.subject = Target{Kind: KindDashboard, ID: run.DashboardID}
	case run.SubjectObservationID.Valid:
		sh.subject = Target{Kind: KindObservation, ID: run.SubjectObservationID}
		sh.onThePage = onTheSubject
	default:
		return share{}, ErrNotShareable
	}
	if revisions.State(run.State) != revisions.StateSucceeded {
		return share{}, ErrNoAnswer
	}
	found, err := s.db.Queries().GetFindingByAgentRequest(ctx, run.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return share{}, ErrNoAnswer
	case err != nil:
		return share{}, fmt.Errorf("read finding of run: %w", err)
	}
	sh.evidence = found.Finding.Evidence
	return sh, nil
}

func (s *Service) postShare(ctx context.Context, sh share, threadID pgtype.UUID) (written, error) {
	opts := writeOptions{hook: s.shareHook(ctx, sh), lockForRun: sh.subject.Kind == KindObservation}
	if threadID.Valid {
		row, sc, err := s.writableThread(ctx, sh.by, threadID)
		if err != nil {
			return written{}, err
		}
		if targetOf(row) != sh.subject {
			return written{}, errWrongThread
		}
		return s.reply(ctx, sh.by, row, sc, ReplyInput{Body: sh.run.Prompt}, opts)
	}
	sc, err := s.scopeOf(ctx, sh.by, sh.subject)
	if err != nil {
		return written{}, err
	}
	open, err := openInputOf(sh)
	if err != nil {
		return written{}, err
	}
	w, err := s.open(ctx, sh.by, sc, open, opts)
	var exists *ObservationThreadExistsError
	if errors.As(err, &exists) && open.Anchor == nil {
		row, sc, err := s.writableThread(ctx, sh.by, exists.ThreadID)
		if err != nil {
			return written{}, err
		}
		return s.reply(ctx, sh.by, row, sc, ReplyInput{Body: sh.run.Prompt, BlockID: open.BlockID}, opts)
	}
	return w, err
}

func openInputOf(sh share) (OpenInput, error) {
	in := OpenInput{Target: sh.subject, Body: sh.run.Prompt}
	if sh.subject.Kind == KindDashboard && sh.run.PanelID.Valid {
		in.PanelID = &sh.run.PanelID.String
	}
	if sh.run.BlockID.Valid {
		block := sh.run.BlockID.String()
		in.BlockID = &block
	}
	if len(sh.run.Anchor) > 0 {
		var at anchor.Anchor
		if err := json.Unmarshal(sh.run.Anchor, &at); err != nil {
			return OpenInput{}, fmt.Errorf("decode the anchor of the run: %w", err)
		}
		in.Anchor = &at
	}
	return in, nil
}

func (s *Service) shareHook(ctx context.Context, sh share) MessageHook {
	return func(q *dbsqlc.Queries, thread Thread, message Message) error {
		messageID, err := uuid.Parse(message.ID)
		if err != nil {
			return fmt.Errorf("message id: %w", err)
		}
		askedIn := pgtype.UUID{Bytes: messageID, Valid: true}
		marked, err := q.ShareRunInThread(ctx, dbsqlc.ShareRunInThreadParams{ID: sh.run.ID, ThreadMessageID: askedIn})
		if err != nil {
			return fmt.Errorf("share agent request: %w", err)
		}
		if marked == 0 {
			return errAlreadyShared
		}
		if err := s.placeAnswer(ctx, q, sh); err != nil {
			return err
		}
		if sh.onThePage {
			if s.showAnswer == nil {
				return errors.New("threads cannot write an answer onto an observation: no answer writer configured")
			}
			if err := s.showAnswer(ctx, q, sh.by, sh.run.ID, sh.subject.ID); err != nil {
				return err
			}
		}
		run := sh.run
		run.ThreadMessageID = askedIn
		if err := replyForRun(ctx, q, run, askedIn); err != nil {
			return err
		}
		return s.recordShare(ctx, q, sh, thread)
	}
}

func (s *Service) placeAnswer(ctx context.Context, q *dbsqlc.Queries, sh share) error {
	var panels []json.RawMessage
	if err := json.Unmarshal(sh.evidence, &panels); err != nil {
		return fmt.Errorf("decode the evidence of the answer: %w", err)
	}
	var refs []string
	for _, panel := range panels {
		found, err := guard.PanelRefs(panel, nil)
		if err != nil {
			return err
		}
		refs = append(refs, found...)
	}
	folder, err := s.subjectFolder(ctx, q, sh.subject)
	if err != nil {
		return err
	}
	return s.dashboards.Placement().Save(ctx, q, folders.ContentAnswer, folder, refs)
}

func (s *Service) subjectFolder(ctx context.Context, q *dbsqlc.Queries, subject Target) (pgtype.UUID, error) {
	var folder pgtype.UUID
	var err error
	if subject.Kind == KindDashboard {
		folder, err = q.ShareDashboardPlacement(ctx, subject.ID)
	} else {
		var row dbsqlc.Observation
		row, err = q.LockObservation(ctx, subject.ID)
		folder = row.FolderID
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, ErrTargetNotFound
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("read the folder of the subject: %w", err)
	}
	return folder, nil
}

func (s *Service) recordShare(ctx context.Context, q *dbsqlc.Queries, sh share, thread Thread) error {
	threadID, err := uuid.Parse(thread.ID)
	if err != nil {
		return fmt.Errorf("thread id: %w", err)
	}
	cols, err := actorOf(ctx, sh.by).Columns()
	if err != nil {
		return err
	}
	target := audit.Target{Kind: "run", ID: sh.run.ID.String()}
	subject := uuid.NullUUID{UUID: sh.subject.ID.Bytes, Valid: true}
	if sh.subject.Kind == KindDashboard {
		target.DashboardID = subject
	} else {
		target.ObservationID = subject
	}
	return audit.Record(ctx, q, audit.Event{
		Action: actionRunShare,
		Target: target,
		Thread: uuid.NullUUID{UUID: threadID, Valid: true},
		Chat:   uuid.NullUUID{UUID: sh.run.ChatID.Bytes, Valid: sh.run.ChatID.Valid},
		Data:   map[string]any{"on_the_page": sh.onThePage},
		By:     cols.AuditBy(),
	})
}

func (s *Service) sharedBefore(ctx context.Context, by auth.Principal, run dbsqlc.AgentRequest) (Shared, error) {
	message, err := s.ReadMessage(ctx, by, run.ThreadMessageID)
	if err != nil {
		return Shared{}, err
	}
	threadID, err := uuid.Parse(message.ThreadID)
	if err != nil {
		return Shared{}, fmt.Errorf("thread id: %w", err)
	}
	row, sc, err := s.threadAndScope(ctx, by, pgtype.UUID{Bytes: threadID, Valid: true})
	if err != nil {
		return Shared{}, err
	}
	thread, err := s.present(ctx, row, by, sc)
	return Shared{Thread: thread, Message: message, Replayed: true}, err
}
