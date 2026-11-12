package activity

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

type View string

const (
	ViewForYou     View = "for_you"
	ViewEverything View = "everything"
	ViewProposals  View = "proposals"
	ViewMentions   View = "mentions"
)

var Views = []View{ViewForYou, ViewEverything, ViewProposals, ViewMentions}

type Reason string

const (
	ReasonProposal       Reason = "proposal"
	ReasonRequestedBlock Reason = "requested_block"
	ReasonMention        Reason = "mention"
	ReasonReply          Reason = "reply"
	ReasonRunFailed      Reason = "run_failed"
)

var Reasons = []Reason{ReasonProposal, ReasonRequestedBlock, ReasonMention, ReasonReply, ReasonRunFailed}

const (
	UnreadCap = 100
	InboxDays = 30
)

var (
	ErrUnknownSeq = errors.New("no activity has that seq")
	ErrPersonOnly = errors.New("the inbox belongs to a signed-in person")
)

type InboxItem struct {
	Event
	Reason Reason
	Unread bool
}

type InboxPage struct {
	Items       []InboxItem
	Next        string
	UnreadCount int
}

func ParseView(s string) (View, bool) {
	for _, v := range Views {
		if string(v) == s {
			return v, true
		}
	}
	return "", false
}

func reasonsOf(v View) []string {
	switch v {
	case ViewProposals:
		return []string{string(ReasonProposal)}
	case ViewMentions:
		return []string{string(ReasonMention), string(ReasonReply)}
	}
	out := make([]string, len(Reasons))
	for i, r := range Reasons {
		out[i] = string(r)
	}
	return out
}

type inboxReader struct {
	audience folders.Audience
	editable []pgtype.UUID
}

func (s *Store) inboxReader(ctx context.Context, by auth.Principal) (inboxReader, error) {
	if by.Kind != auth.KindUser {
		return inboxReader{}, ErrPersonOnly
	}
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return inboxReader{}, err
	}
	audience, err := folders.AudienceOf(by, levels)
	if err != nil {
		return inboxReader{}, err
	}
	editable := []pgtype.UUID{}
	for _, f := range levels.Folders() {
		if levels.Of(f.ID).Edit {
			editable = append(editable, f.ID)
		}
	}
	return inboxReader{audience: audience, editable: editable}, nil
}

func (s *Store) Inbox(ctx context.Context, by auth.Principal, view View, cursor string, limit int) (InboxPage, error) {
	reader, err := s.inboxReader(ctx, by)
	if err != nil {
		return InboxPage{}, err
	}
	a := reader.audience
	params := dbsqlc.ListInboxParams{
		ReaderID:   a.ReaderID,
		Editable:   reader.editable,
		Folders:    visibleFolders(a),
		Admin:      a.Admin,
		ReaderKind: a.ReaderKind,
		Everything: view == ViewEverything,
		Reasons:    reasonsOf(view),
		RowLimit:   httpapi.Int32(limit + 1),
	}
	if cursor != "" {
		from, err := decodeCursor(cursor)
		if err != nil {
			return InboxPage{}, err
		}
		params.HasCursor = true
		params.CursorXid = pgtype.Uint64{Uint64: from.xid, Valid: true}
		params.CursorSeq = from.seq
	}
	q := s.db.Queries()
	rows, err := q.ListInbox(ctx, params)
	if err != nil {
		return InboxPage{}, fmt.Errorf("list inbox: %w", err)
	}
	page := InboxPage{}
	if len(rows) > limit {
		rows = rows[:limit]
		page.Next = encodeCursor(positionOf(rows[limit-1].Activity))
	}
	page.Items = make([]InboxItem, 0, len(rows))
	for _, r := range rows {
		page.Items = append(page.Items, InboxItem{
			Event:  eventsOf([]dbsqlc.Activity{r.Activity})[0],
			Reason: Reason(r.Reason),
			Unread: r.Unread,
		})
	}
	count, err := q.InboxUnreadCount(ctx, dbsqlc.InboxUnreadCountParams{
		ReaderID:   a.ReaderID,
		Editable:   reader.editable,
		Folders:    visibleFolders(a),
		Admin:      a.Admin,
		ReaderKind: a.ReaderKind,
	})
	if err != nil {
		return InboxPage{}, fmt.Errorf("count unread inbox items: %w", err)
	}
	page.UnreadCount = min(int(count), UnreadCap)
	return page, nil
}

func (s *Store) MarkRead(ctx context.Context, by auth.Principal, seq int64) error {
	if by.Kind != auth.KindUser {
		return ErrPersonOnly
	}
	userID, err := db.ParseUUID(by.ID)
	if err != nil {
		return folders.ErrForbidden
	}
	q := s.db.Queries()
	xid, err := q.ActivityXid(ctx, seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnknownSeq
	}
	if err != nil {
		return fmt.Errorf("get activity position: %w", err)
	}
	if err := q.MarkInboxRead(ctx, dbsqlc.MarkInboxReadParams{UserID: userID, ReadXid: xid, ReadSeq: seq}); err != nil {
		return fmt.Errorf("mark inbox read: %w", err)
	}
	return nil
}
