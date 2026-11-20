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

func quotesRun(item InboxItem) bool {
	return item.Type == TypeRunFailed && item.Reason == ReasonRunFailed
}

type inboxTexts struct {
	titles   map[objectRef]string
	bodies   map[string]string
	failures map[string]string
	closes   map[string]string
}

func (t inboxTexts) excerptOf(item InboxItem) string {
	switch {
	case item.Type == TypeThreadMentioned || item.Type == TypeThreadReplied:
		return Excerpt(t.bodies[dataOf(item.Event).MessageID], inboxExcerptRunes)
	case quotesRun(item):
		return Excerpt(t.failures[item.TargetID], inboxExcerptRunes)
	case item.Reason == ReasonProposalClosed:
		return Excerpt(t.closes[item.TargetID], inboxExcerptRunes)
	}
	return ""
}

func (s *Store) describeInbox(ctx context.Context, items []InboxItem) error {
	texts, err := s.loadInboxTexts(ctx, items)
	if err != nil {
		return err
	}
	for i := range items {
		item := &items[i]
		item.TargetTitle = texts.titles[subjectOf(item.Event)]
		item.Excerpt = texts.excerptOf(*item)
	}
	return nil
}

type inboxRefs struct {
	byKind   map[string]map[string]bool
	messages map[string]bool
	runs     map[string]bool
	closed   map[string]bool
}

func inboxRefsOf(items []InboxItem) inboxRefs {
	refs := inboxRefs{
		byKind:   map[string]map[string]bool{"dashboard": {}, "observation": {}, "proposal": {}},
		messages: map[string]bool{},
		runs:     map[string]bool{},
		closed:   map[string]bool{},
	}
	for _, item := range items {
		if ref := subjectOf(item.Event); refs.byKind[ref.kind] != nil {
			refs.byKind[ref.kind][ref.id] = true
		}
		if id := dataOf(item.Event).MessageID; id != "" && (item.Type == TypeThreadMentioned || item.Type == TypeThreadReplied) {
			refs.messages[id] = true
		}
		if quotesRun(item) {
			refs.runs[item.TargetID] = true
		}
		if item.Reason == ReasonProposalClosed {
			refs.closed[item.TargetID] = true
		}
	}
	return refs
}

func (s *Store) loadInboxTexts(ctx context.Context, items []InboxItem) (inboxTexts, error) {
	q := s.db.Queries()
	refs := inboxRefsOf(items)
	byKind, messages, runs, closed := refs.byKind, refs.messages, refs.runs, refs.closed
	texts := inboxTexts{titles: map[objectRef]string{}, bodies: map[string]string{}, failures: map[string]string{}, closes: map[string]string{}}
	titleRows, err := q.InboxObjectTitles(ctx, dbsqlc.InboxObjectTitlesParams{
		DashboardIds:   uuidsOf(byKind["dashboard"]),
		ObservationIds: uuidsOf(byKind["observation"]),
		ProposalIds:    uuidsOf(byKind["proposal"]),
	})
	if err != nil {
		return inboxTexts{}, fmt.Errorf("list inbox titles: %w", err)
	}
	for _, r := range titleRows {
		texts.titles[objectRef{r.Kind, r.ID.String()}] = r.Title
	}
	messageRows, err := q.InboxMessageBodies(ctx, uuidsOf(messages))
	if err != nil {
		return inboxTexts{}, fmt.Errorf("list inbox messages: %w", err)
	}
	for _, r := range messageRows {
		texts.bodies[r.ID.String()] = r.Body
	}
	runRows, err := q.InboxRunTexts(ctx, uuidsOf(runs))
	if err != nil {
		return inboxTexts{}, fmt.Errorf("list inbox runs: %w", err)
	}
	for _, r := range runRows {
		texts.failures[r.ID.String()] = firstNonEmpty(r.Error, r.Prompt)
	}
	closeRows, err := q.InboxProposalCloseReasons(ctx, uuidsOf(closed))
	if err != nil {
		return inboxTexts{}, fmt.Errorf("list inbox proposal close reasons: %w", err)
	}
	for _, r := range closeRows {
		texts.closes[r.ID.String()] = r.CloseReason
	}
	return texts, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
