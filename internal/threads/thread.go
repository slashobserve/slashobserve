package threads

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	StatusOpen     = "open"
	StatusResolved = "resolved"
	StatusAll      = "all"
)

type Thread struct {
	ID                string
	Target            Target
	DashboardID       string
	Version           *int
	PanelID           string
	PanelPresent      bool
	Status            string
	AllowedActions    []string
	Unread            bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ResolvedAt        *time.Time
	ResolvedBy        *actor.Actor
	ResolvedByVersion *int
	Anchor            *anchor.Anchor
	BlockID           string
	Agent             *revisions.Assignee
	AgentOff          bool
	AgentState        AgentState
}

func (t Thread) recipientState() threadAgent {
	return threadAgent{Agent: t.Agent, Off: t.AgentOff}
}

type Message struct {
	ID          string
	ThreadID    string
	Body        string
	Author      actor.Actor
	AuthorEmail string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	EditedAt    *time.Time
	Mentions    []Mention
	CanEdit     bool
	CanDelete   bool
	BlockID     string
	ReplyTo     *ReplyTo
	Run         *StartedRun
	Answer      *Answer
}

type ReplyTo struct {
	ID      string
	Author  actor.Actor
	Excerpt string
}

type rights struct {
	Comment, Edit, Started bool
}

func (r rights) can(w lifecycle.Who) bool {
	switch w {
	case lifecycle.Commenter:
		return r.Comment
	case lifecycle.Requester:
		return r.Comment && r.Started
	case lifecycle.Editor:
		return r.Edit
	}
	return false
}

func (sc scope) rights(started bool) rights {
	return rights{Comment: sc.abilities.Comment, Edit: sc.abilities.Edit, Started: started}
}

func threadOf(row dbsqlc.Thread, latestPanels map[string]struct{}, r rights) Thread {
	_, present := latestPanels[row.PanelID.String]
	t := Thread{
		ID:             row.ID.String(),
		Target:         targetOf(row),
		PanelID:        row.PanelID.String,
		PanelPresent:   !row.PanelID.Valid || present,
		Status:         row.Status,
		AllowedActions: allowedActions(row.Status, r),
		CreatedAt:      row.CreatedAt.Time.UTC(),
		UpdatedAt:      row.UpdatedAt.Time.UTC(),
	}
	if row.DashboardID.Valid {
		t.DashboardID = row.DashboardID.String()
		version := int(row.DashboardVersion.Int32)
		t.Version = &version
	}
	if row.ResolvedAt.Valid {
		at := row.ResolvedAt.Time.UTC()
		t.ResolvedAt = &at
	}
	if row.ResolvedByKind.Valid {
		resolver := actor.FromColumns(actor.Columns{
			Kind:   row.ResolvedByKind.String,
			ID:     row.ResolvedByID,
			Name:   row.ResolvedByName.String,
			Origin: row.ResolvedOrigin,
			Client: row.ResolvedOriginClient,
			RunID:  row.ResolvedRunID,
		})
		t.ResolvedBy = &resolver
	}
	if row.ResolvedByVersion.Valid {
		version := int(row.ResolvedByVersion.Int32)
		t.ResolvedByVersion = &version
	}
	if len(row.Anchor) > 0 {
		var at anchor.Anchor
		if json.Unmarshal(row.Anchor, &at) == nil {
			t.Anchor = &at
		}
	}
	if row.BlockID.Valid {
		t.BlockID = row.BlockID.String()
	}
	if row.AgentKind.Valid {
		t.Agent = &revisions.Assignee{Kind: revisions.AssigneeKind(row.AgentKind.String), ID: row.AgentID}
	}
	t.AgentOff = row.AgentOff
	t.AgentState = t.recipientState().state()
	return t
}

func messageOf(row dbsqlc.ThreadMessage, mentions []Mention, reader auth.Principal, canComment bool) Message {
	m := Message{
		ID:       row.ID.String(),
		ThreadID: row.ThreadID.String(),
		Body:     row.Body,
		Author: actor.FromColumns(actor.Columns{
			Kind:   row.AuthorKind,
			ID:     row.AuthorID,
			Name:   row.AuthorName,
			Origin: row.Origin,
			Client: row.OriginClient,
			RunID:  builtinRunID(row.RunAssignedKind, row.RunID),
		}),
		AuthorEmail: row.AuthorEmail.String,
		CreatedAt:   row.CreatedAt.Time.UTC(),
		UpdatedAt:   row.UpdatedAt.Time.UTC(),
		Mentions:    mentions,
		CanEdit:     canComment && !row.RunID.Valid && row.AuthorKind == string(reader.Kind) && row.AuthorID.String() == reader.ID,
	}
	if row.ObservationBlockID.Valid {
		m.BlockID = row.ObservationBlockID.String()
	}
	if row.EditedAt.Valid {
		at := row.EditedAt.Time.UTC()
		m.EditedAt = &at
	}
	return m
}

func replyToOf(row dbsqlc.ListReplyQuotesRow) *ReplyTo {
	return &ReplyTo{
		ID: row.QuotedID.String(),
		Author: actor.FromColumns(actor.Columns{
			Kind:   row.AuthorKind,
			ID:     row.AuthorID,
			Name:   row.AuthorName,
			Origin: row.Origin,
			Client: row.OriginClient,
			RunID:  builtinRunID(row.RunAssignedKind, row.RunID),
		}),
		Excerpt: row.Excerpt,
	}
}

func builtinRunID(assignedKind pgtype.Text, runID pgtype.UUID) pgtype.UUID {
	if assignedKind.String != string(revisions.AssignBuiltin) {
		return pgtype.UUID{}
	}
	return runID
}

func actorOf(ctx context.Context, by auth.Principal) actor.Actor {
	return actor.Of(by, origin.From(ctx), "")
}
