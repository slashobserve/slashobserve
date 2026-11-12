package threads

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxBodyRunes = revisions.MaxPromptRunes
	maxInt32     = 1<<31 - 1

	resolvedByVersionConstraint = "threads_resolved_by_version_fkey"
	observationThreadIndex      = "threads_observation_idx"
	foreignKeyViolationCode     = "23503"
	uniqueViolationCode         = "23505"
)

type Service struct {
	db           *db.DB
	access       *folders.Access
	dashboards   *dashboards.Service
	findings     *findings.Service
	annotations  *annotations.Service
	observations *observations.Service
	panelSets    *panelCache
}

func NewService(database *db.DB, access *folders.Access, dashboards *dashboards.Service, findings *findings.Service, annotations *annotations.Service, observations *observations.Service) *Service {
	return &Service{db: database, access: access, dashboards: dashboards, findings: findings, annotations: annotations, observations: observations, panelSets: newPanelCache()}
}

type OpenInput struct {
	Target  Target
	Version *int
	PanelID *string
	Body    string
}

type ListQuery struct {
	Target  Target
	Status  string
	PanelID string
	Limit   int
	Cursor  string
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

type anchor struct {
	version      pgtype.Int4
	panelID      pgtype.Text
	latestPanels map[string]struct{}
}

func checkAnchorShape(in OpenInput) error {
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

func (s *Service) anchorOf(ctx context.Context, in OpenInput) (anchor, error) {
	if err := checkAnchorShape(in); err != nil {
		return anchor{}, err
	}
	if in.Target.Kind != KindDashboard {
		return anchor{}, nil
	}
	latest, err := s.dashboards.LatestVersion(ctx, in.Target.ID)
	if errors.Is(err, dashboards.ErrNotFound) {
		return anchor{}, ErrTargetNotFound
	}
	if err != nil {
		return anchor{}, err
	}
	version := latest
	if in.Version != nil {
		if *in.Version > latest {
			return anchor{}, ErrUnknownVersion
		}
		version = *in.Version
	}
	at := anchor{version: pgtype.Int4{Int32: httpapi.Int32(version), Valid: true}}
	if in.PanelID == nil {
		return at, nil
	}
	onVersion, err := s.panels(ctx, in.Target.ID, version)
	if err != nil {
		return anchor{}, err
	}
	if _, ok := onVersion[*in.PanelID]; !ok {
		return anchor{}, ErrUnknownPanel
	}
	at.panelID = pgtype.Text{String: *in.PanelID, Valid: true}
	if at.latestPanels, err = s.panels(ctx, in.Target.ID, latest); err != nil {
		return anchor{}, err
	}
	return at, nil
}

func (s *Service) Open(ctx context.Context, by auth.Principal, in OpenInput, hook func(*dbsqlc.Queries, Thread, Message) error) (Thread, Message, error) {
	sc, err := s.scopeOf(ctx, by, in.Target)
	if err != nil {
		return Thread{}, Message{}, err
	}
	if !sc.abilities.Comment {
		return Thread{}, Message{}, folders.ErrForbidden
	}
	body, mentions, err := checkBody(in.Body)
	if err != nil {
		return Thread{}, Message{}, err
	}
	at, err := s.anchorOf(ctx, in)
	if err != nil {
		return Thread{}, Message{}, err
	}
	author := actorOf(ctx, by)
	var thread Thread
	var message Message
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := lockTarget(ctx, q, sc); err != nil {
			return err
		}
		threadRow, err := q.CreateThread(ctx, createThreadParams(sc.target, at))
		if err != nil {
			return fmt.Errorf("create thread: %w", err)
		}
		messageRow, err := createMessage(ctx, q, author, by.Email, newMessage{id: threadRow.ID, threadID: threadRow.ID, body: body, createdAt: threadRow.CreatedAt})
		if err != nil {
			return err
		}
		recorded, err := s.recordMentions(ctx, q, messageRow.ID, mentions)
		if err != nil {
			return err
		}
		thread = threadOf(threadRow, at.latestPanels, sc.abilities.Comment)
		message = messageOf(messageRow, recorded, by, sc.abilities.Comment)
		if err := (emitter{scope: sc, thread: threadRow, actor: author}).opened(ctx, q, messageRow, recorded); err != nil {
			return err
		}
		if hook == nil {
			return nil
		}
		return hook(q, thread, message)
	})
	if isUniqueViolation(err, observationThreadIndex) {
		return Thread{}, Message{}, s.existingObservationThread(ctx, in.Target.ID)
	}
	if err != nil {
		return Thread{}, Message{}, targetGone(err)
	}
	return thread, message, nil
}

func (s *Service) existingObservationThread(ctx context.Context, observationID pgtype.UUID) error {
	id, err := s.db.Queries().ObservationThread(ctx, observationID)
	if err != nil {
		return threadLookup(err)
	}
	return &ObservationThreadExistsError{ThreadID: id}
}

func (s *Service) Reply(ctx context.Context, by auth.Principal, id pgtype.UUID, body string) (Thread, Message, error) {
	row, sc, err := s.writableThread(ctx, by, id)
	if err != nil {
		return Thread{}, Message{}, err
	}
	cleaned, mentions, err := checkBody(body)
	if err != nil {
		return Thread{}, Message{}, err
	}
	author := actorOf(ctx, by)
	var message Message
	var current dbsqlc.Thread
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := lockTarget(ctx, q, sc); err != nil {
			return err
		}
		var err error
		if current, err = q.GetThreadForShare(ctx, row.ID); err != nil {
			return threadLookup(err)
		}
		messageRow, err := createMessage(ctx, q, author, by.Email, newMessage{threadID: current.ID, body: cleaned})
		if err != nil {
			return err
		}
		recorded, err := s.recordMentions(ctx, q, messageRow.ID, mentions)
		if err != nil {
			return err
		}
		message = messageOf(messageRow, recorded, by, sc.abilities.Comment)
		return emitter{scope: sc, thread: current, actor: author}.replied(ctx, q, messageRow, recorded)
	})
	if err != nil {
		return Thread{}, Message{}, threadGone(err)
	}
	thread, err := s.present(ctx, current, sc.abilities.Comment)
	return thread, message, err
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
	if messageRow.AuthorKind != string(by.Kind) || messageRow.AuthorID.String() != by.ID {
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
		if err := lockTarget(ctx, q, sc); err != nil {
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
		if err := q.DeleteMentions(ctx, messageRow.ID); err != nil {
			return fmt.Errorf("delete mentions: %w", err)
		}
		if recorded, err = s.recordMentions(ctx, q, messageRow.ID, mentions); err != nil {
			return err
		}
		return emitter{scope: sc, thread: row, actor: author}.edited(ctx, q, messageRow)
	})
	if err != nil {
		return Thread{}, Message{}, false, threadGone(err)
	}
	thread, err := s.present(ctx, row, sc.abilities.Comment)
	return thread, messageOf(messageRow, recorded, by, sc.abilities.Comment), changed, err
}

func (s *Service) SetStatus(ctx context.Context, by auth.Principal, id pgtype.UUID, status string, resolvedByVersion *int) (Thread, bool, error) {
	if err := checkStatusChange(status, resolvedByVersion); err != nil {
		return Thread{}, false, err
	}
	row, sc, err := s.writableThread(ctx, by, id)
	if err != nil {
		return Thread{}, false, err
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
		if err := lockTarget(ctx, q, sc); err != nil {
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
	thread, err := s.present(ctx, updated, sc.abilities.Comment)
	return thread, changed, err
}

func listParams(in ListQuery) (dbsqlc.ListThreadMessagesParams, error) {
	params := dbsqlc.ListThreadMessagesParams{RowLimit: httpapi.Int32(in.Limit + 1)}
	switch in.Target.Kind {
	case KindDashboard:
		params.DashboardID = in.Target.ID
	case KindFinding:
		params.FindingID = in.Target.ID
	case KindAnnotation:
		params.AnnotationID = in.Target.ID
	case KindObservation:
		params.ObservationID = in.Target.ID
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
	ids := make([]pgtype.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ThreadMessage.ID
	}
	mentions, err := s.mentionsByMessage(ctx, s.db.Queries(), ids)
	if err != nil {
		return Page{}, err
	}
	seen := map[pgtype.UUID]bool{}
	for _, r := range rows {
		if !seen[r.Thread.ID] {
			seen[r.Thread.ID] = true
			page.Threads = append(page.Threads, threadOf(r.Thread, latestPanels, sc.abilities.Comment))
		}
		page.Messages = append(page.Messages, messageOf(r.ThreadMessage, mentions[r.ThreadMessage.ID], by, sc.abilities.Comment))
	}
	return page, nil
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
	return messageOf(row, mentions, by, sc.abilities.Comment), nil
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
	})
	if err != nil {
		return dbsqlc.ThreadMessage{}, fmt.Errorf("create message: %w", err)
	}
	return row, nil
}

func createThreadParams(t Target, at anchor) dbsqlc.CreateThreadParams {
	params := dbsqlc.CreateThreadParams{DashboardVersion: at.version, PanelID: at.panelID}
	switch t.Kind {
	case KindDashboard:
		params.DashboardID = t.ID
	case KindFinding:
		params.FindingID = t.ID
	case KindAnnotation:
		params.AnnotationID = t.ID
	case KindObservation:
		params.ObservationID = t.ID
	}
	return params
}

func (s *Service) present(ctx context.Context, row dbsqlc.Thread, canComment bool) (Thread, error) {
	if !row.PanelID.Valid {
		return threadOf(row, nil, canComment), nil
	}
	latest, err := s.dashboards.LatestVersion(ctx, row.DashboardID)
	if err != nil {
		return Thread{}, err
	}
	panels, err := s.panels(ctx, row.DashboardID, latest)
	if err != nil {
		return Thread{}, err
	}
	return threadOf(row, panels, canComment), nil
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
	cleaned, ok := revisions.CleanPrompt(body)
	if !ok {
		return "", nil, &InvalidInputError{Code: CodeInvalidBody, Message: fmt.Sprintf("body must be 1 to %d characters without control or bidirectional override characters", maxBodyRunes)}
	}
	mentions, err := ParseMentions(cleaned)
	if err != nil {
		return "", nil, err
	}
	return cleaned, mentions, nil
}

func (s *Service) recordMentions(ctx context.Context, q *dbsqlc.Queries, messageID pgtype.UUID, parsed []Mention) ([]Mention, error) {
	var userIDs []pgtype.UUID
	for _, m := range parsed {
		if m.Agent == "" {
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
			names[u.ID] = capName(cmp.Or(u.Name, u.Email))
		}
	}
	recorded := make([]Mention, 0, len(parsed))
	for _, m := range parsed {
		params := dbsqlc.InsertMentionParams{MessageID: messageID, Name: m.Name}
		if m.Agent != "" {
			params.Agent = pgtype.Text{String: m.Agent, Valid: true}
		} else {
			name, ok := names[m.UserID]
			if !ok {
				continue
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
		byMessage[r.MessageID] = append(byMessage[r.MessageID], Mention{UserID: r.UserID, Agent: r.Agent.String, Name: r.Name})
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
