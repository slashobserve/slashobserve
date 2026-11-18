package activity

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const inboxExcerptRunes = 200

type objectRef struct {
	kind string
	id   string
}

type inboxData struct {
	Object    *struct{ Kind, ID string } `json:"object"`
	MessageID string                     `json:"message_id"`
}

func dataOf(e Event) inboxData {
	var d inboxData
	_ = json.Unmarshal(e.Data, &d)
	return d
}

func subjectOf(e Event) objectRef {
	d := dataOf(e)
	switch e.TargetKind {
	case "thread":
		if d.Object != nil && d.Object.Kind != "annotation" && d.Object.Kind != "finding" {
			return objectRef{d.Object.Kind, d.Object.ID}
		}
	case "dashboard", "observation", "proposal":
		return objectRef{e.TargetKind, e.TargetID}
	}
	if e.DashboardID != "" {
		return objectRef{"dashboard", e.DashboardID}
	}
	return objectRef{}
}

func uuidsOf(ids map[string]bool) []pgtype.UUID {
	out := make([]pgtype.UUID, 0, len(ids))
	for id := range ids {
		if u, err := db.ParseUUID(id); err == nil {
			out = append(out, u)
		}
	}
	return out
}

func (s *Store) describeInbox(ctx context.Context, items []InboxItem) error {
	q := s.db.Queries()
	byKind := map[string]map[string]bool{"dashboard": {}, "observation": {}, "proposal": {}}
	messages := map[string]bool{}
	runs := map[string]bool{}
	for _, item := range items {
		if ref := subjectOf(item.Event); byKind[ref.kind] != nil {
			byKind[ref.kind][ref.id] = true
		}
		switch item.Type {
		case TypeThreadMentioned, TypeThreadReplied:
			if id := dataOf(item.Event).MessageID; id != "" {
				messages[id] = true
			}
		case TypeRunFailed:
			runs[item.TargetID] = true
		}
	}
	titles := map[objectRef]string{}
	rows, err := q.InboxObjectTitles(ctx, dbsqlc.InboxObjectTitlesParams{
		DashboardIds:   uuidsOf(byKind["dashboard"]),
		ObservationIds: uuidsOf(byKind["observation"]),
		ProposalIds:    uuidsOf(byKind["proposal"]),
	})
	if err != nil {
		return fmt.Errorf("list inbox titles: %w", err)
	}
	for _, r := range rows {
		titles[objectRef{r.Kind, r.ID.String()}] = r.Title
	}
	bodies := map[string]string{}
	messageRows, err := q.InboxMessageBodies(ctx, uuidsOf(messages))
	if err != nil {
		return fmt.Errorf("list inbox messages: %w", err)
	}
	for _, r := range messageRows {
		bodies[r.ID.String()] = r.Body
	}
	failures := map[string]string{}
	runRows, err := q.InboxRunTexts(ctx, uuidsOf(runs))
	if err != nil {
		return fmt.Errorf("list inbox runs: %w", err)
	}
	for _, r := range runRows {
		failures[r.ID.String()] = firstNonEmpty(r.Error, r.Prompt)
	}
	for i := range items {
		item := &items[i]
		item.TargetTitle = titles[subjectOf(item.Event)]
		switch item.Type {
		case TypeThreadMentioned, TypeThreadReplied:
			item.Excerpt = Excerpt(bodies[dataOf(item.Event).MessageID], inboxExcerptRunes)
		case TypeRunFailed:
			item.Excerpt = Excerpt(failures[item.TargetID], inboxExcerptRunes)
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
