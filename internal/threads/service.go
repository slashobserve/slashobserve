package threads

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/proposals"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/usertext"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxBodyRunes = revisions.MaxPromptRunes
	maxInt32     = 1<<31 - 1

	resolvedByVersionConstraint = "threads_resolved_by_version_fkey"
	observationThreadIndex      = "threads_observation_page_idx"
	foreignKeyViolationCode     = "23503"
	uniqueViolationCode         = "23505"
)

type Service struct {
	db           *db.DB
	access       *folders.Access
	dashboards   *dashboards.Service
	annotations  *annotations.Service
	observations *observations.Service
	proposals    *proposals.Service
	panelSets    *panelCache
	showAnswer   AnswerShower
	hideAnswer   AnswerHider
	answers      *answerReads
}

func NewService(database *db.DB, access *folders.Access, dashboards *dashboards.Service, annotations *annotations.Service, observations *observations.Service, proposals *proposals.Service) *Service {
	return &Service{db: database, access: access, dashboards: dashboards, annotations: annotations, observations: observations, proposals: proposals, panelSets: newPanelCache()}
}

type MessageHook func(*dbsqlc.Queries, Thread, Message) error

type OpenInput struct {
	Target   Target
	Version  *int
	PanelID  *string
	BlockID  *string
	Anchor   *anchor.Anchor
	ClientID pgtype.UUID
	Body     string
}

type ReplyInput struct {
	Body      string
	BlockID   *string
	ReplyToID *string
	ClientID  pgtype.UUID
}

type writeOptions struct {
	hook       MessageHook
	lockForRun bool
}

type written struct {
	thread   Thread
	message  Message
	replayed bool
	refusal  *RunRefusal
}

var errReplayed = errors.New("a message with this client_id was already posted")

type ListQuery struct {
	Target  Target
	Status  string
	PanelID string
	Limit   int
	Cursor  string

	WithAnswers bool
}

type Page struct {
	Threads    []Thread
	Messages   []Message
	Next       string
	CanComment bool
}

func (s *Service) Abilities(ctx context.Context, by auth.Principal, t Target) (folders.Abilities, error) {
	sc, err := s.scopeOf(ctx, by, t)
	return sc.abilities, err
}

type placement struct {
	version      pgtype.Int4
	panelID      pgtype.Text
	latestPanels map[string]struct{}
	point        threadPoint
}

type threadPoint struct {
	anchor []byte
	block  pgtype.UUID
}

func checkPlacementShape(in OpenInput) error {
	if in.Target.Kind != KindDashboard {
		if in.Version != nil || in.PanelID != nil {
			return invalidTarget("version and panel_id apply only to a dashboard")
		}
		return nil
	}
	if in.Version != nil && !validVersion(*in.Version) {
		return &InvalidInputError{Code: CodeInvalidVersion, Message: "version must be a positive 32-bit integer"}
	}
	if in.PanelID != nil {
		return checkPanelID(*in.PanelID)
	}
	return nil
}

func (s *Service) placementOf(ctx context.Context, in OpenInput) (placement, error) {
	if err := checkPlacementShape(in); err != nil {
		return placement{}, err
	}
	if in.Target.Kind != KindDashboard {
		return placement{}, nil
	}
	latest, err := s.dashboards.LatestVersion(ctx, in.Target.ID)
	if errors.Is(err, dashboards.ErrNotFound) {
		return placement{}, ErrTargetNotFound
	}
	if err != nil {
		return placement{}, err
	}
	version := latest
	if in.Version != nil {
		if *in.Version > latest {
			return placement{}, ErrUnknownVersion
		}
		version = *in.Version
	}
	at := placement{version: pgtype.Int4{Int32: httpapi.Int32(version), Valid: true}}
	if in.PanelID == nil {
		return at, nil
	}
	onVersion, err := s.panels(ctx, in.Target.ID, version)
	if err != nil {
		return placement{}, err
	}
	if _, ok := onVersion[*in.PanelID]; !ok {
		return placement{}, ErrUnknownPanel
	}
	at.panelID = pgtype.Text{String: *in.PanelID, Valid: true}
	if at.latestPanels, err = s.panels(ctx, in.Target.ID, latest); err != nil {
		return placement{}, err
	}
	return at, nil
}

func (s *Service) Open(ctx context.Context, by auth.Principal, in OpenInput, hook MessageHook) (Thread, Message, error) {
	sc, err := s.scopeOf(ctx, by, in.Target)
	if err != nil {
		return Thread{}, Message{}, err
	}
	w, err := s.open(ctx, by, sc, in, writeOptions{hook: hook})
	return w.thread, w.message, err
}

func (s *Service) open(ctx context.Context, by auth.Principal, sc scope, in OpenInput, opts writeOptions) (written, error) {
	in.Target = sc.target
	if !sc.abilities.Comment {
		return written{}, folders.ErrForbidden
	}
	first, err := s.firstMessageOf(ctx, in)
	if err != nil {
		return written{}, err
	}
	body, mentions, at, refs := first.body, first.mentions, first.at, first.refs
	author := actorOf(ctx, by)
	var thread Thread
	var message Message
	var threadID pgtype.UUID
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := lockTarget(ctx, q, sc, opts.lockForRun); err != nil {
			return err
		}
		threadRow, err := q.CreateThread(ctx, createThreadParams(sc.target, at))
		if err != nil {
			return fmt.Errorf("create thread: %w", err)
		}
		threadID = threadRow.ID
		if err := refs.lock(ctx, q, sc.target, threadRow.ID); err != nil {
			return err
		}
		messageRow, err := createMessage(ctx, q, author, by.Email, newMessage{id: threadRow.ID, threadID: threadRow.ID, body: body, createdAt: threadRow.CreatedAt, refs: refs, clientID: in.ClientID})
		if err != nil {
			return err
		}
		recorded, err := s.recordMentions(ctx, q, by, sc.target, messageRow.ID, mentions, nil)
		if err != nil {
			return err
		}
		thread = threadOf(threadRow, at.latestPanels, sc.rights(true))
		message = messageOf(messageRow, recorded, by, sc.abilities.Comment)
		message.CanDelete = mayDelete(messageRow, by, sc, false)
		if err := (emitter{scope: sc, thread: threadRow, actor: author}).opened(ctx, q, messageRow, recorded); err != nil {
			return err
		}
		if opts.hook == nil {
			return nil
		}
		return opts.hook(q, thread, message)
	})
	switch {
	case errors.Is(err, errReplayed):
		return s.replayed(ctx, by, sc, author, in.ClientID)
	case isUniqueViolation(err, observationThreadIndex):
		return written{}, s.existingObservationThread(ctx, in.Target.ID)
	case err != nil:
		return written{}, targetGone(err)
	}
	if opts.hook != nil {
		row, err := s.db.Queries().GetThread(ctx, threadID)
		if err != nil {
			return written{}, threadLookup(err)
		}
		thread = threadOf(row, at.latestPanels, sc.rights(true))
	}
	return written{thread: thread, message: message}, nil
}

type firstMessage struct {
	body     string
	mentions []Mention
	at       placement
	refs     messageRefs
}

func (s *Service) firstMessageOf(ctx context.Context, in OpenInput) (firstMessage, error) {
	body, mentions, err := checkBody(in.Body)
	if err != nil {
		return firstMessage{}, err
	}
	at, err := s.placementOf(ctx, in)
	if err != nil {
		return firstMessage{}, err
	}
	refs, err := parseRefs(in.Target.Kind, in.BlockID, nil)
	if err != nil {
		return firstMessage{}, err
	}
	if at.point, err = pointOf(in, refs); err != nil {
		return firstMessage{}, err
	}
	return firstMessage{body: body, mentions: mentions, at: at, refs: refs}, nil
}

func pointOf(in OpenInput, refs messageRefs) (threadPoint, error) {
	if in.Anchor == nil {
		return threadPoint{}, nil
	}
	if in.Target.Kind != KindDashboard && in.Target.Kind != KindObservation {
		return threadPoint{}, invalidTarget("anchor applies only to a dashboard or an observation")
	}
	if err := in.Anchor.Validate(); err != nil {
		return threadPoint{}, &InvalidInputError{Code: CodeInvalidAnchor, Message: "anchor: " + err.Error()}
	}
	if refs.block.Valid {
		if err := in.Anchor.CheckOnBlock(); err != nil {
			return threadPoint{}, &InvalidInputError{Code: CodeInvalidAnchor, Message: err.Error()}
		}
	}
	raw, err := json.Marshal(in.Anchor)
	if err != nil {
		return threadPoint{}, fmt.Errorf("encode anchor: %w", err)
	}
	return threadPoint{anchor: raw, block: refs.block}, nil
}

func (s *Service) replayed(ctx context.Context, by auth.Principal, sc scope, author actor.Actor, clientID pgtype.UUID) (written, error) {
	cols, err := author.Columns()
	if err != nil {
		return written{}, err
	}
	row, err := s.db.Queries().GetMessageByClient(ctx, dbsqlc.GetMessageByClientParams{AuthorKind: cols.Kind, AuthorID: cols.ID, ClientID: clientID})
	if err != nil {
		return written{}, threadLookup(err)
	}
	threadRow, err := s.db.Queries().GetThread(ctx, row.ThreadID)
	if err != nil {
		return written{}, threadLookup(err)
	}
	if targetOf(threadRow) != sc.target {
		return written{}, &InvalidInputError{Code: CodeClientIDReused, Message: "client_id was already used for a message elsewhere; send a new one"}
	}
	mentions, err := s.mentionsOf(ctx, s.db.Queries(), row.ID)
	if err != nil {
		return written{}, err
	}
	message, err := s.presentMessage(ctx, row, mentions, by, sc)
	if err != nil {
		return written{}, err
	}
	message.CanDelete = mayDelete(row, by, sc, false)
	runs, err := runsOf(ctx, s.db.Queries(), []pgtype.UUID{row.ID})
	if err != nil {
		return written{}, err
	}
	message.Run = runs[row.ID]
	thread, err := s.present(ctx, threadRow, by, sc)
	return written{thread: thread, message: message, replayed: true, refusal: storedRefusal(row)}, err
}

func (s *Service) existingObservationThread(ctx context.Context, observationID pgtype.UUID) error {
	id, err := s.db.Queries().ObservationThread(ctx, observationID)
	if err != nil {
		return threadLookup(err)
	}
	return &ObservationThreadExistsError{ThreadID: id}
}

func (s *Service) Reply(ctx context.Context, by auth.Principal, id pgtype.UUID, in ReplyInput, hook MessageHook) (Thread, Message, error) {
	row, sc, err := s.writableThread(ctx, by, id)
	if err != nil {
		return Thread{}, Message{}, err
	}
	w, err := s.reply(ctx, by, row, sc, in, writeOptions{hook: hook})
	return w.thread, w.message, err
}

func (s *Service) reply(ctx context.Context, by auth.Principal, row dbsqlc.Thread, sc scope, in ReplyInput, opts writeOptions) (written, error) {
	cleaned, mentions, err := checkBody(in.Body)
	if err != nil {
		return written{}, err
	}
	refs, err := parseRefs(sc.target.Kind, in.BlockID, in.ReplyToID)
	if err != nil {
		return written{}, err
	}
	author := actorOf(ctx, by)
	var message Message
	var current dbsqlc.Thread
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := lockTarget(ctx, q, sc, opts.lockForRun); err != nil {
			return err
		}
		var err error
		if current, err = q.LockThreadForWrite(ctx, row.ID); err != nil {
			return threadLookup(err)
		}
		if err := refs.lock(ctx, q, sc.target, current.ID); err != nil {
			return err
		}
		messageRow, err := createMessage(ctx, q, author, by.Email, newMessage{threadID: current.ID, body: cleaned, refs: refs, clientID: in.ClientID})
		if err != nil {
			return err
		}
		if err := markRead(ctx, q, by, current.ID); err != nil {
			return err
		}
		recorded, err := s.recordMentions(ctx, q, by, sc.target, messageRow.ID, mentions, nil)
		if err != nil {
			return err
		}
		quotes, err := quotesOf(ctx, q, []dbsqlc.ThreadMessage{messageRow})
		if err != nil {
			return err
		}
		message = messageOf(messageRow, recorded, by, sc.abilities.Comment)
		message.CanDelete = mayDelete(messageRow, by, sc, false)
		message.ReplyTo = quotes[messageRow.ID]
		if err := (emitter{scope: sc, thread: current, actor: author}).replied(ctx, q, messageRow, recorded); err != nil {
			return err
		}
		if opts.hook == nil {
			return nil
		}
		return opts.hook(q, threadOf(current, nil, sc.rights(false)), message)
	})
	if errors.Is(err, errReplayed) {
		return s.replayed(ctx, by, sc, author, in.ClientID)
	}
	if err != nil {
		return written{}, threadGone(err)
	}
	if current, err = s.db.Queries().GetThread(ctx, current.ID); err != nil {
		return written{}, threadLookup(err)
	}
	thread, err := s.present(ctx, current, by, sc)
	return written{thread: thread, message: message}, err
}

func (s *Service) Edit(ctx context.Context, by auth.Principal, messageID pgtype.UUID, body string) (Thread, Message, bool, error) {
	messageRow, err := s.db.Queries().GetMessage(ctx, messageID)
	if err != nil {
		return Thread{}, Message{}, false, threadLookup(err)
	}
	row, sc, err := s.threadAndScope(ctx, by, messageRow.ThreadID)
	if err != nil {
		return Thread{}, Message{}, false, err
	}
	if !writtenBy(messageRow, by) {
		return Thread{}, Message{}, false, ErrNotAuthor
	}
	if !sc.abilities.Comment {
		return Thread{}, Message{}, false, folders.ErrForbidden
	}
	cleaned, mentions, err := checkBody(body)
	if err != nil {
		return Thread{}, Message{}, false, err
	}
	author := actorOf(ctx, by)
	changed := false
	var recorded []Mention
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := lockTarget(ctx, q, sc, false); err != nil {
			return err
		}
		var err error
		if messageRow, err = q.GetMessage(ctx, messageID); err != nil {
			return threadLookup(err)
		}
		if messageRow.Body == cleaned {
			recorded, err = s.mentionsOf(ctx, q, messageRow.ID)
			return err
		}
		changed = true
		if messageRow, err = q.EditMessage(ctx, dbsqlc.EditMessageParams{ID: messageID, Body: cleaned}); err != nil {
			return fmt.Errorf("edit message: %w", err)
		}
		before, err := s.mentionsOf(ctx, q, messageRow.ID)
		if err != nil {
			return err
		}
		if err := q.DeleteMentions(ctx, messageRow.ID); err != nil {
			return fmt.Errorf("delete mentions: %w", err)
		}
		if recorded, err = s.recordMentions(ctx, q, by, sc.target, messageRow.ID, mentions, before); err != nil {
			return err
		}
		return emitter{scope: sc, thread: row, actor: author}.edited(ctx, q, messageRow)
	})
	if err != nil {
		return Thread{}, Message{}, false, threadGone(err)
	}
	message, err := s.presentMessage(ctx, messageRow, recorded, by, sc)
	if err != nil {
		return Thread{}, Message{}, false, err
	}
	thread, err := s.present(ctx, row, by, sc)
	return thread, message, changed, err
}

func writtenBy(message dbsqlc.ThreadMessage, by auth.Principal) bool {
	return !message.RunID.Valid && message.AuthorKind == string(by.Kind) && message.AuthorID.String() == by.ID
}

func (s *Service) SetStatus(ctx context.Context, by auth.Principal, id pgtype.UUID, status string, resolvedByVersion *int) (Thread, bool, error) {
	if err := checkStatusChange(status, resolvedByVersion); err != nil {
		return Thread{}, false, err
	}
	row, sc, err := s.writableThread(ctx, by, id)
	if err != nil {
		return Thread{}, false, err
	}
	if status == StatusResolved {
		started, err := s.startedBy(ctx, s.db.Queries(), by, row.ID)
		if err != nil {
			return Thread{}, false, err
		}
		if !slices.Contains(allowedActions(StatusOpen, sc.rights(started)), string(ActionResolve)) {
			return Thread{}, false, ErrNotStarter
		}
	}
	params := dbsqlc.SetThreadStatusParams{ID: row.ID, Status: status}
	if status == StatusResolved {
		resolver, err := actorOf(ctx, by).Columns()
		if err != nil {
			return Thread{}, false, err
		}
		params.ResolvedByKind = pgtype.Text{String: resolver.Kind, Valid: true}
		params.ResolvedByID = resolver.ID
		params.ResolvedByName = pgtype.Text{String: resolver.Name, Valid: true}
		params.ResolvedOrigin, params.ResolvedOriginClient = resolver.Origin, resolver.Client
	}
	if resolvedByVersion != nil {
		if err := s.checkResolvedByVersion(ctx, row, *resolvedByVersion); err != nil {
			return Thread{}, false, err
		}
		params.ResolvedByVersion = pgtype.Int4{Int32: httpapi.Int32(*resolvedByVersion), Valid: true}
	}
	var updated dbsqlc.Thread
	changed := false
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := lockTarget(ctx, q, sc, false); err != nil {
			return err
		}
		var err error
		updated, err = q.SetThreadStatus(ctx, params)
		switch {
		case err == nil:
			changed = true
			return emitter{scope: sc, thread: updated, actor: actorOf(ctx, by)}.statusChanged(ctx, q)
		case errors.Is(err, pgx.ErrNoRows):
			updated, err = q.GetThread(ctx, row.ID)
			return threadLookup(err)
		}
		if isConstraintViolation(err, resolvedByVersionConstraint) {
			return ErrUnknownVersion
		}
		return fmt.Errorf("set thread status: %w", err)
	})
	if err != nil {
		return Thread{}, false, threadGone(err)
	}
	thread, err := s.present(ctx, updated, by, sc)
	return thread, changed, err
}

func listParams(in ListQuery) (dbsqlc.ListThreadMessagesParams, error) {
	params := dbsqlc.ListThreadMessagesParams{RowLimit: httpapi.Int32(in.Limit + 1)}
	switch in.Target.Kind {
	case KindDashboard:
		params.DashboardID = in.Target.ID
	case KindAnnotation:
		params.AnnotationID = in.Target.ID
	case KindObservation:
		params.ObservationID = in.Target.ID
	case KindProposal:
		params.ProposalID = in.Target.ID
	}
	switch in.Status {
	case StatusOpen, StatusResolved:
		params.Status = pgtype.Text{String: in.Status, Valid: true}
	case StatusAll, "":
	default:
		return params, &InvalidInputError{Code: CodeInvalidStatus, Message: "status must be open, resolved or all"}
	}
	if in.PanelID != "" {
		if err := checkPanelID(in.PanelID); err != nil {
			return params, err
		}
		params.PanelID = pgtype.Text{String: in.PanelID, Valid: true}
	}
	after, ok := httpapi.DecodeTimeCursor(in.Cursor)
	if !ok {
		return params, dashboards.ErrInvalidCursor
	}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	return params, nil
}

func (s *Service) List(ctx context.Context, by auth.Principal, in ListQuery) (Page, error) {
	sc, err := s.scopeOf(ctx, by, in.Target)
	if err != nil {
		return Page{}, err
	}
	in.Target = sc.target
	params, err := listParams(in)
	if err != nil {
		return Page{}, err
	}
	rows, err := s.db.Queries().ListThreadMessages(ctx, params)
	if err != nil {
		return Page{}, fmt.Errorf("list thread messages: %w", err)
	}
	page := Page{CanComment: sc.abilities.Comment, Threads: []Thread{}, Messages: []Message{}}
	if len(rows) > in.Limit {
		rows = rows[:in.Limit]
		last := rows[in.Limit-1].ThreadMessage
		page.Next = httpapi.EncodeTimeCursor(last.CreatedAt.Time, last.ID.String())
	}
	if len(rows) == 0 {
		return page, nil
	}
	latestPanels, err := s.latestPanels(ctx, sc)
	if err != nil {
		return Page{}, err
	}
	pd, err := s.pageDataOf(ctx, by, sc, rows)
	if err != nil {
		return Page{}, err
	}
	if in.WithAnswers {
		if pd.answers, err = s.answersOf(ctx, by, messagesOf(rows)); err != nil {
			return Page{}, fmt.Errorf("read the answers of the page's runs: %w", err)
		}
	}
	seen := map[pgtype.UUID]bool{}
	for _, r := range rows {
		if !seen[r.Thread.ID] {
			seen[r.Thread.ID] = true
			thread := threadOf(r.Thread, latestPanels, sc.rights(pd.started[r.Thread.ID]))
			thread.Unread = pd.unread[r.Thread.ID]
			page.Threads = append(page.Threads, thread)
		}
		message := messageOf(r.ThreadMessage, pd.mentions[r.ThreadMessage.ID], by, sc.abilities.Comment)
		message.CanDelete = mayDelete(r.ThreadMessage, by, sc, r.ThreadMessage.ID == r.Thread.ID && pd.others[r.Thread.ID])
		message.ReplyTo = pd.quotes[r.ThreadMessage.ID]
		message.Run = pd.runs[r.ThreadMessage.ID]
		message.Answer = answerOf(pd.answers, r.ThreadMessage)
		page.Messages = append(page.Messages, message)
	}
	return page, nil
}

type pageData struct {
	mentions map[pgtype.UUID][]Mention
	quotes   map[pgtype.UUID]*ReplyTo
	others   map[pgtype.UUID]bool
	runs     map[pgtype.UUID]*StartedRun
	started  map[pgtype.UUID]bool
	unread   map[pgtype.UUID]bool
	answers  map[pgtype.UUID]Answer
}

func (s *Service) pageDataOf(ctx context.Context, by auth.Principal, sc scope, rows []dbsqlc.ListThreadMessagesRow) (pageData, error) {
	ids := make([]pgtype.UUID, len(rows))
	messageRows := make([]dbsqlc.ThreadMessage, len(rows))
	for i, r := range rows {
		ids[i] = r.ThreadMessage.ID
		messageRows[i] = r.ThreadMessage
	}
	var pd pageData
	var err error
	if pd.mentions, err = s.mentionsByMessage(ctx, s.db.Queries(), ids); err != nil {
		return pageData{}, err
	}
	if pd.quotes, err = quotesOf(ctx, s.db.Queries(), messageRows); err != nil {
		return pageData{}, err
	}
	if pd.others, err = s.otherAuthorsOnPage(ctx, by, sc, rows); err != nil {
		return pageData{}, err
	}
	if pd.runs, err = runsOf(ctx, s.db.Queries(), ids); err != nil {
		return pageData{}, err
	}
	if pd.started, err = s.startedAmong(ctx, s.db.Queries(), by, rows); err != nil {
		return pageData{}, err
	}
	if pd.unread, err = s.unreadAmong(ctx, s.db.Queries(), by, threadIDsOf(rows)); err != nil {
		return pageData{}, err
	}
	return pd, nil
}

type CommentRevision struct {
	ID     string
	Status string
}

func (s *Service) latestRevisions(ctx context.Context, commentIDs []string) (map[string]*CommentRevision, error) {
	if len(commentIDs) == 0 {
		return nil, nil
	}
	ids := make([]pgtype.UUID, len(commentIDs))
	for i, raw := range commentIDs {
		id, err := db.ParseUUID(raw)
		if err != nil {
			return nil, fmt.Errorf("comment id: %w", err)
		}
		ids[i] = id
	}
	rows, err := s.db.Queries().LatestAgentRequestsForComments(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("latest agent requests for comments: %w", err)
	}
	byComment := make(map[string]*CommentRevision, len(rows))
	for _, r := range rows {
		byComment[r.CommentID.String()] = &CommentRevision{ID: r.ID.String(), Status: r.Status}
	}
	return byComment, nil
}

func (s *Service) ReadMessage(ctx context.Context, by auth.Principal, id pgtype.UUID) (Message, error) {
	row, err := s.db.Queries().GetMessage(ctx, id)
	if err != nil {
		return Message{}, threadLookup(err)
	}
	_, sc, err := s.threadAndScope(ctx, by, row.ThreadID)
	if err != nil {
		return Message{}, err
	}
	mentions, err := s.mentionsOf(ctx, s.db.Queries(), row.ID)
	if err != nil {
		return Message{}, err
	}
	message, err := s.presentMessage(ctx, row, mentions, by, sc)
	if err != nil {
		return Message{}, err
	}
	runs, err := runsOf(ctx, s.db.Queries(), []pgtype.UUID{row.ID})
	message.Run = runs[row.ID]
	return message, err
}

func runsOf(ctx context.Context, q *dbsqlc.Queries, messageIDs []pgtype.UUID) (map[pgtype.UUID]*StartedRun, error) {
	rows, err := q.RunsOfMessages(ctx, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("list the runs of messages: %w", err)
	}
	runs := make(map[pgtype.UUID]*StartedRun, len(rows))
	for _, r := range rows {
		runs[r.ThreadMessageID] = &StartedRun{ID: r.ID.String(), Status: r.Status, AssignedKind: r.AssignedKind}
	}
	return runs, nil
}

func (s *Service) writableThread(ctx context.Context, by auth.Principal, id pgtype.UUID) (dbsqlc.Thread, scope, error) {
	row, err := s.db.Queries().GetThreadOfMessage(ctx, id)
	if err != nil {
		return dbsqlc.Thread{}, scope{}, threadLookup(err)
	}
	sc, err := s.scopeOfThread(ctx, by, row)
	if err != nil {
		return dbsqlc.Thread{}, scope{}, err
	}
	if !sc.abilities.Comment {
		return dbsqlc.Thread{}, scope{}, folders.ErrForbidden
	}
	return row, sc, nil
}

func (s *Service) threadAndScope(ctx context.Context, by auth.Principal, id pgtype.UUID) (dbsqlc.Thread, scope, error) {
	row, err := s.db.Queries().GetThread(ctx, id)
	if err != nil {
		return dbsqlc.Thread{}, scope{}, threadLookup(err)
	}
	sc, err := s.scopeOfThread(ctx, by, row)
	return row, sc, err
}

type newMessage struct {
	id        pgtype.UUID
	threadID  pgtype.UUID
	body      string
	createdAt pgtype.Timestamptz
	refs      messageRefs
	clientID  pgtype.UUID
}

func createMessage(ctx context.Context, q *dbsqlc.Queries, author actor.Actor, email string, m newMessage) (dbsqlc.ThreadMessage, error) {
	cols, err := author.Columns()
	if err != nil {
		return dbsqlc.ThreadMessage{}, err
	}
	row, err := q.CreateMessage(ctx, dbsqlc.CreateMessageParams{
		ID:           m.id,
		ThreadID:     m.threadID,
		Body:         m.body,
		AuthorKind:   cols.Kind,
		AuthorID:     cols.ID,
		AuthorName:   cols.Name,
		AuthorEmail:  pgtype.Text{String: email, Valid: email != ""},
		Origin:       cols.Origin,
		OriginClient: cols.Client,
		CreatedAt:    m.createdAt,

		ReplyToID:          m.refs.replyTo,
		ObservationBlockID: m.refs.block,
		ClientID:           m.clientID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.ThreadMessage{}, errReplayed
	}
	if err != nil {
		return dbsqlc.ThreadMessage{}, fmt.Errorf("create message: %w", err)
	}
	return row, nil
}

type messageRefs struct {
	block   pgtype.UUID
	replyTo pgtype.UUID
}

func parseRefs(kind Kind, blockID, replyToID *string) (messageRefs, error) {
	var refs messageRefs
	if blockID != nil {
		if kind != KindObservation {
			return refs, errInvalidBlock
		}
		id, err := db.ParseUUID(*blockID)
		if err != nil {
			return refs, errInvalidBlock
		}
		refs.block = id
	}
	if replyToID != nil {
		id, err := db.ParseUUID(*replyToID)
		if err != nil {
			return refs, errInvalidReplyTo
		}
		refs.replyTo = id
	}
	return refs, nil
}

func (r messageRefs) lock(ctx context.Context, q *dbsqlc.Queries, target Target, threadID pgtype.UUID) error {
	if r.block.Valid {
		_, err := q.LockObservationBlock(ctx, dbsqlc.LockObservationBlockParams{ID: r.block, ObservationID: target.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errInvalidBlock
		}
		if err != nil {
			return fmt.Errorf("lock observation block: %w", err)
		}
	}
	if r.replyTo.Valid {
		_, err := q.LockMessageInThread(ctx, dbsqlc.LockMessageInThreadParams{ID: r.replyTo, ThreadID: threadID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errInvalidReplyTo
		}
		if err != nil {
			return fmt.Errorf("lock quoted message: %w", err)
		}
	}
	return nil
}

func (s *Service) presentMessage(ctx context.Context, row dbsqlc.ThreadMessage, mentions []Mention, reader auth.Principal, sc scope) (Message, error) {
	quotes, err := quotesOf(ctx, s.db.Queries(), []dbsqlc.ThreadMessage{row})
	if err != nil {
		return Message{}, err
	}
	message := messageOf(row, mentions, reader, sc.abilities.Comment)
	message.ReplyTo = quotes[row.ID]
	if message.CanDelete, err = s.canDeleteOne(ctx, s.db.Queries(), row, reader, sc); err != nil {
		return Message{}, err
	}
	return message, nil
}

func quotesOf(ctx context.Context, q *dbsqlc.Queries, rows []dbsqlc.ThreadMessage) (map[pgtype.UUID]*ReplyTo, error) {
	var ids []pgtype.UUID
	for _, r := range rows {
		if r.ReplyToID.Valid {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	quoted, err := q.ListReplyQuotes(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list reply quotes: %w", err)
	}
	quotes := make(map[pgtype.UUID]*ReplyTo, len(quoted))
	for _, r := range quoted {
		quotes[r.MessageID] = replyToOf(r)
	}
	return quotes, nil
}

func createThreadParams(t Target, at placement) dbsqlc.CreateThreadParams {
	params := dbsqlc.CreateThreadParams{DashboardVersion: at.version, PanelID: at.panelID, Anchor: at.point.anchor, BlockID: at.point.block}
	switch t.Kind {
	case KindDashboard:
		params.DashboardID = t.ID
	case KindAnnotation:
		params.AnnotationID = t.ID
	case KindObservation:
		params.ObservationID = t.ID
	case KindProposal:
		params.ProposalID = t.ID
	}
	return params
}

func (s *Service) unreadAmong(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, threadIDs []pgtype.UUID) (map[pgtype.UUID]bool, error) {
	if by.Kind != auth.KindUser || len(threadIDs) == 0 {
		return nil, nil
	}
	readerID, err := db.ParseUUID(by.ID)
	if err != nil {
		return nil, fmt.Errorf("reader id: %w", err)
	}
	found, err := q.UnreadThreads(ctx, dbsqlc.UnreadThreadsParams{ThreadIds: threadIDs, ReaderKind: string(by.Kind), ReaderID: readerID})
	if err != nil {
		return nil, fmt.Errorf("list unread threads: %w", err)
	}
	unread := make(map[pgtype.UUID]bool, len(found))
	for _, id := range found {
		unread[id] = true
	}
	return unread, nil
}

func markRead(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, threadID pgtype.UUID) error {
	if by.Kind != auth.KindUser {
		return nil
	}
	readerID, err := db.ParseUUID(by.ID)
	if err != nil {
		return fmt.Errorf("reader id: %w", err)
	}
	if err := q.MarkThreadRead(ctx, dbsqlc.MarkThreadReadParams{UserID: readerID, ThreadID: threadID}); err != nil {
		return fmt.Errorf("mark thread read: %w", err)
	}
	return nil
}

func (s *Service) MarkRead(ctx context.Context, by auth.Principal, id pgtype.UUID) error {
	row, err := s.db.Queries().GetThreadOfMessage(ctx, id)
	if err != nil {
		return threadLookup(err)
	}
	if _, err := s.scopeOfThread(ctx, by, row); err != nil {
		return err
	}
	return threadGone(markRead(ctx, s.db.Queries(), by, row.ID))
}

func (s *Service) startedBy(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, threadID pgtype.UUID) (bool, error) {
	first, err := q.GetMessage(ctx, threadID)
	if err != nil {
		return false, threadLookup(err)
	}
	return ownMessage(first, by), nil
}

func threadIDsOf(rows []dbsqlc.ListThreadMessagesRow) []pgtype.UUID {
	ids := make([]pgtype.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Thread.ID)
	}
	return ids
}

func (s *Service) startedAmong(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, rows []dbsqlc.ListThreadMessagesRow) (map[pgtype.UUID]bool, error) {
	firsts, err := q.ListMessagesByID(ctx, threadIDsOf(rows))
	if err != nil {
		return nil, fmt.Errorf("list the first messages of threads: %w", err)
	}
	started := make(map[pgtype.UUID]bool, len(firsts))
	for _, first := range firsts {
		started[first.ID] = ownMessage(first, by)
	}
	return started, nil
}

func (s *Service) present(ctx context.Context, row dbsqlc.Thread, by auth.Principal, sc scope) (Thread, error) {
	started, err := s.startedBy(ctx, s.db.Queries(), by, row.ID)
	if err != nil {
		return Thread{}, err
	}
	unread, err := s.unreadAmong(ctx, s.db.Queries(), by, []pgtype.UUID{row.ID})
	if err != nil {
		return Thread{}, err
	}
	thread, err := s.presentWithPanels(ctx, row, sc.rights(started))
	thread.Unread = unread[row.ID]
	return thread, err
}

func (s *Service) presentWithPanels(ctx context.Context, row dbsqlc.Thread, r rights) (Thread, error) {
	if !row.PanelID.Valid {
		return threadOf(row, nil, r), nil
	}
	latest, err := s.dashboards.LatestVersion(ctx, row.DashboardID)
	if err != nil {
		return Thread{}, err
	}
	panels, err := s.panels(ctx, row.DashboardID, latest)
	if err != nil {
		return Thread{}, err
	}
	return threadOf(row, panels, r), nil
}

func (s *Service) latestPanels(ctx context.Context, sc scope) (map[string]struct{}, error) {
	if sc.target.Kind != KindDashboard {
		return nil, nil
	}
	latest, err := s.dashboards.LatestVersion(ctx, sc.target.ID)
	if err != nil {
		return nil, err
	}
	return s.panels(ctx, sc.target.ID, latest)
}

func (s *Service) panels(ctx context.Context, dashboardID pgtype.UUID, version int) (map[string]struct{}, error) {
	key := panelKey{dashboard: dashboardID, version: version}
	if set, ok := s.panelSets.get(key); ok {
		return set, nil
	}
	d, err := s.dashboards.VersionSpec(ctx, dashboardID, version)
	if err != nil {
		return nil, err
	}
	set := panelSet(d)
	s.panelSets.put(key, set)
	return set, nil
}

func panelSet(d *spec.Dashboard) map[string]struct{} {
	entries := d.AllPanels()
	set := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		set[e.Panel.ID] = struct{}{}
	}
	return set
}

func (s *Service) checkResolvedByVersion(ctx context.Context, row dbsqlc.Thread, version int) error {
	if !row.DashboardID.Valid {
		return &InvalidInputError{Code: CodeInvalidVersion, Message: "resolved_by_version applies only to a dashboard thread"}
	}
	latest, err := s.dashboards.LatestVersion(ctx, row.DashboardID)
	if err != nil {
		return err
	}
	if version > latest {
		return ErrUnknownVersion
	}
	if version < int(row.DashboardVersion.Int32) {
		return &InvalidInputError{Code: CodeInvalidVersion, Message: fmt.Sprintf("resolved_by_version must be at least the version the thread is on (%d)", row.DashboardVersion.Int32)}
	}
	return nil
}

func checkStatusChange(status string, resolvedByVersion *int) error {
	if status != StatusOpen && status != StatusResolved {
		return &InvalidInputError{Code: CodeInvalidStatus, Message: "status must be open or resolved"}
	}
	if resolvedByVersion == nil {
		return nil
	}
	if status != StatusResolved {
		return &InvalidInputError{Code: CodeInvalidVersion, Message: "resolved_by_version is only allowed with status resolved"}
	}
	if !validVersion(*resolvedByVersion) {
		return &InvalidInputError{Code: CodeInvalidVersion, Message: "resolved_by_version must be a positive 32-bit integer"}
	}
	return nil
}

func validVersion(n int) bool {
	return n >= 1 && n <= maxInt32
}

func checkPanelID(panelID string) error {
	if err := revisions.ValidPanelID(panelID); err != nil {
		return &InvalidInputError{Code: CodeInvalidPanelID, Message: err.Error()}
	}
	return nil
}

func checkBody(body string) (string, []Mention, error) {
	cleaned, err := usertext.Clean("body", body, usertext.Text(maxBodyRunes))
	if err != nil {
		return "", nil, &InvalidInputError{Code: CodeInvalidBody, Message: err.Error(), Cause: err}
	}
	mentions, err := ParseMentions(cleaned)
	if err != nil {
		return "", nil, err
	}
	return cleaned, mentions, nil
}

func (s *Service) recordMentions(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, on Target, messageID pgtype.UUID, parsed, already []Mention) ([]Mention, error) {
	names, err := s.viewableUserNames(ctx, q, on, parsed)
	if err != nil {
		return nil, err
	}
	recorded := make([]Mention, 0, len(parsed))
	for _, m := range parsed {
		params := dbsqlc.InsertMentionParams{MessageID: messageID, Name: m.Name}
		switch m.Kind() {
		case mentionAgent:
			params.Agent = pgtype.Text{String: m.Agent, Valid: true}
		case mentionGrant:
			name, err := mentionableGrant(ctx, q, by, m)
			if err != nil {
				return nil, err
			}
			m.Name = cmp.Or(capName(name), m.Name)
			params.AgentGrantID, params.Name = m.GrantID, m.Name
		case mentionServiceAccount:
			name, err := q.MentionableServiceAccount(ctx, m.ServiceAccountID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errInvalidMention
			}
			if err != nil {
				return nil, fmt.Errorf("read mentioned service account: %w", err)
			}
			m.Name = capName(name)
			params.AgentServiceAccountID, params.Name = m.ServiceAccountID, m.Name
		default:
			name, ok := names[m.UserID]
			if !ok {
				name, ok = nameAmong(already, m.UserID)
			}
			if !ok {
				return nil, errInvalidPerson
			}
			m.Name = name
			params.UserID, params.Name = m.UserID, name
		}
		if err := q.InsertMention(ctx, params); err != nil {
			return nil, fmt.Errorf("record mention: %w", err)
		}
		recorded = append(recorded, m)
	}
	sortMentions(recorded)
	return recorded, nil
}

func nameAmong(mentions []Mention, user pgtype.UUID) (string, bool) {
	for _, m := range mentions {
		if m.Kind() == mentionUser && m.UserID == user {
			return m.Name, true
		}
	}
	return "", false
}

func (s *Service) viewableUserNames(ctx context.Context, q *dbsqlc.Queries, on Target, parsed []Mention) (map[pgtype.UUID]string, error) {
	var userIDs []pgtype.UUID
	for _, m := range parsed {
		if m.Kind() == mentionUser {
			userIDs = append(userIDs, m.UserID)
		}
	}
	names := map[pgtype.UUID]string{}
	if len(userIDs) > 0 {
		users, err := q.UserMentionNames(ctx, userIDs)
		if err != nil {
			return nil, fmt.Errorf("read mentioned users: %w", err)
		}
		for _, u := range users {
			visible, err := s.canView(ctx, userPrincipal(u.ID, u.Name, u.Email, u.Role, u.Groups), on)
			if err != nil {
				return nil, err
			}
			if visible {
				names[u.ID] = capName(cmp.Or(u.Name, u.Email))
			}
		}
	}
	return names, nil
}

func mentionableGrant(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, m Mention) (string, error) {
	if by.Kind != auth.KindUser {
		return "", errInvalidMention
	}
	personID, err := db.ParseUUID(by.ID)
	if err != nil {
		return "", fmt.Errorf("author id: %w", err)
	}
	name, err := q.MentionableGrant(ctx, dbsqlc.MentionableGrantParams{ID: m.GrantID, UserID: personID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errInvalidMention
	}
	if err != nil {
		return "", fmt.Errorf("read mentioned grant: %w", err)
	}
	return name, nil
}

func (s *Service) mentionsOf(ctx context.Context, q *dbsqlc.Queries, messageID pgtype.UUID) ([]Mention, error) {
	byMessage, err := s.mentionsByMessage(ctx, q, []pgtype.UUID{messageID})
	return byMessage[messageID], err
}

func (s *Service) mentionsByMessage(ctx context.Context, q *dbsqlc.Queries, messageIDs []pgtype.UUID) (map[pgtype.UUID][]Mention, error) {
	rows, err := q.ListMentions(ctx, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("list mentions: %w", err)
	}
	byMessage := make(map[pgtype.UUID][]Mention, len(messageIDs))
	for _, r := range rows {
		byMessage[r.MessageID] = append(byMessage[r.MessageID], Mention{UserID: r.UserID, Agent: r.Agent.String, GrantID: r.AgentGrantID, ServiceAccountID: r.AgentServiceAccountID, Name: r.Name})
	}
	for _, mentions := range byMessage {
		sortMentions(mentions)
	}
	return byMessage, nil
}

func sortMentions(mentions []Mention) {
	slices.SortFunc(mentions, func(a, b Mention) int {
		return cmp.Or(
			cmp.Compare(a.Kind(), b.Kind())*-1,
			cmp.Compare(a.Name, b.Name),
			cmp.Compare(a.RefID(), b.RefID()),
		)
	})
}

func threadLookup(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	}
	return fmt.Errorf("get thread: %w", err)
}

func targetGone(err error) error {
	if db.IsForeignKeyViolation(err) {
		return ErrTargetNotFound
	}
	return err
}

func threadGone(err error) error {
	if errors.Is(err, ErrTargetNotFound) || db.IsForeignKeyViolation(err) {
		return ErrNotFound
	}
	return err
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == constraint
}

func isConstraintViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode && pgErr.ConstraintName == constraint
}
