package threads

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/events"
)

type objectRef struct {
	Kind Kind   `json:"kind"`
	ID   string `json:"id"`
}

type mentionJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type eventData struct {
	Object            objectRef    `json:"object"`
	Title             string       `json:"title,omitempty"`
	Version           *int32       `json:"version,omitempty"`
	PanelID           string       `json:"panel_id,omitempty"`
	MessageID         string       `json:"message_id,omitempty"`
	ResolvedByVersion *int32       `json:"resolved_by_version,omitempty"`
	Mentioned         *mentionJSON `json:"mentioned,omitempty"`
}

type emitter struct {
	scope  scope
	thread dbsqlc.Thread
	actor  actor.Actor
}

func (e emitter) data() eventData {
	return eventData{Object: objectRef{Kind: e.scope.target.Kind, ID: e.scope.target.ID.String()}}
}

func (e emitter) append(ctx context.Context, q *dbsqlc.Queries, typ string, data eventData, live *events.Event) error {
	return e.appendWith(ctx, q, typ, data, nil, live)
}

func (e emitter) appendWith(ctx context.Context, q *dbsqlc.Queries, typ string, data eventData, auditOnly map[string]any, live *events.Event) error {
	_, err := activity.Append(ctx, q, activity.Entry{
		Type:      typ,
		TargetID:  e.thread.ID,
		Owner:     e.scope.activity,
		Actor:     e.actor,
		Data:      data,
		AuditOnly: auditOnly,
	}, live)
	return err
}

func (e emitter) sent(message dbsqlc.ThreadMessage) map[string]any {
	return audit.MessageData(message.Body, string(anchor.KindOf(e.thread.Anchor)))
}

func (e emitter) live(typ string, commentID, parentID pgtype.UUID, status string) *events.Event {
	if e.scope.target.Kind != KindDashboard {
		return nil
	}
	ev := events.Event{
		Type:        typ,
		DashboardID: e.thread.DashboardID.String(),
		CommentID:   commentID.String(),
		PanelID:     e.thread.PanelID.String,
		Status:      status,
		Author:      events.NewAuthor(string(e.actor.Kind), e.actor.ID, e.actor.Name),
	}
	if parentID.Valid {
		ev.ParentID = parentID.String()
	}
	return &ev
}

func (e emitter) opened(ctx context.Context, q *dbsqlc.Queries, message dbsqlc.ThreadMessage, mentions []Mention) error {
	data := e.data()
	title, err := q.GetThreadEventTitle(ctx, dbsqlc.GetThreadEventTitleParams{DashboardID: e.scope.dashboard, ObservationID: e.thread.ObservationID})
	if err != nil {
		return err
	}
	data.Title = title
	if e.thread.DashboardID.Valid {
		version := e.thread.DashboardVersion.Int32
		data.Version, data.PanelID = &version, e.thread.PanelID.String
	}
	if err := e.appendWith(ctx, q, activity.TypeThreadOpened, data, e.sent(message), e.live(events.TypeCommentCreated, message.ID, pgtype.UUID{}, e.thread.Status)); err != nil {
		return err
	}
	return e.mentioned(ctx, q, message, mentions)
}

func (e emitter) replied(ctx context.Context, q *dbsqlc.Queries, message dbsqlc.ThreadMessage, mentions []Mention) error {
	data := e.data()
	data.MessageID = message.ID.String()
	if err := e.appendWith(ctx, q, activity.TypeThreadReplied, data, e.sent(message), e.live(events.TypeCommentCreated, message.ID, e.thread.ID, e.thread.Status)); err != nil {
		return err
	}
	return e.mentioned(ctx, q, message, mentions)
}

func (e emitter) edited(ctx context.Context, q *dbsqlc.Queries, message dbsqlc.ThreadMessage) error {
	data := e.data()
	data.MessageID = message.ID.String()
	var parent pgtype.UUID
	if message.ID != e.thread.ID {
		parent = e.thread.ID
	}
	return e.append(ctx, q, activity.TypeThreadEdited, data, e.live(events.TypeCommentUpdated, message.ID, parent, e.thread.Status))
}

func (e emitter) deleted(ctx context.Context, q *dbsqlc.Queries, messageID pgtype.UUID) error {
	data := e.data()
	data.MessageID = messageID.String()
	return e.append(ctx, q, activity.TypeThreadDeleted, data, e.live(events.TypeCommentUpdated, e.thread.ID, pgtype.UUID{}, e.thread.Status))
}

func (e emitter) statusChanged(ctx context.Context, q *dbsqlc.Queries) error {
	data := e.data()
	typ := activity.TypeThreadReopened
	if e.thread.Status == StatusResolved {
		typ = activity.TypeThreadResolved
		if e.thread.ResolvedByVersion.Valid {
			version := e.thread.ResolvedByVersion.Int32
			data.ResolvedByVersion = &version
		}
	}
	return e.append(ctx, q, typ, data, e.live(events.TypeCommentUpdated, e.thread.ID, pgtype.UUID{}, e.thread.Status))
}

func (e emitter) mentioned(ctx context.Context, q *dbsqlc.Queries, message dbsqlc.ThreadMessage, mentions []Mention) error {
	for _, m := range mentions {
		data := e.data()
		data.MessageID = message.ID.String()
		data.Mentioned = &mentionJSON{Kind: m.Kind(), ID: m.RefID(), Name: m.Name}
		if err := e.append(ctx, q, activity.TypeThreadMentioned, data, nil); err != nil {
			return err
		}
	}
	return nil
}
