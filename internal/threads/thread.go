package threads

import (
	"context"
	"time"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/origin"
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
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ResolvedAt        *time.Time
	ResolvedBy        *actor.Actor
	ResolvedByVersion *int
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
}

func threadOf(row dbsqlc.Thread, latestPanels map[string]struct{}) Thread {
	_, present := latestPanels[row.PanelID.String]
	t := Thread{
		ID:           row.ID.String(),
		Target:       targetOf(row),
		PanelID:      row.PanelID.String,
		PanelPresent: !row.PanelID.Valid || present,
		Status:       row.Status,
		CreatedAt:    row.CreatedAt.Time.UTC(),
		UpdatedAt:    row.UpdatedAt.Time.UTC(),
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
		}),
		AuthorEmail: row.AuthorEmail.String,
		CreatedAt:   row.CreatedAt.Time.UTC(),
		UpdatedAt:   row.UpdatedAt.Time.UTC(),
		Mentions:    mentions,
		CanEdit:     canComment && row.AuthorKind == string(reader.Kind) && row.AuthorID.String() == reader.ID,
	}
	if row.EditedAt.Valid {
		at := row.EditedAt.Time.UTC()
		m.EditedAt = &at
	}
	return m
}

func actorOf(ctx context.Context, by auth.Principal) actor.Actor {
	return actor.Of(by, origin.From(ctx), "")
}
