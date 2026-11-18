package threads

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

const (
	maxMentionablePeople = 20
	mentionableScanRows  = 100
	maxMentionTermRunes  = 100
)

type Person struct {
	ID    string
	Name  string
	Email string
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func likePattern(term string) string {
	return "%" + likeEscaper.Replace(strings.TrimSpace(term)) + "%"
}

func (s *Service) MentionablePeople(ctx context.Context, by auth.Principal, t Target, term string, limit int) ([]Person, error) {
	sc, err := s.scopeOf(ctx, by, t)
	if err != nil {
		return nil, err
	}
	if !sc.abilities.Comment {
		return nil, folders.ErrForbidden
	}
	self, err := db.ParseUUID(by.ID)
	if err != nil {
		self = pgtype.UUID{}
	}
	rows, err := s.db.Queries().MentionCandidates(ctx, dbsqlc.MentionCandidatesParams{Self: self, Pattern: likePattern(term), RowLimit: mentionableScanRows})
	if err != nil {
		return nil, fmt.Errorf("list mention candidates: %w", err)
	}
	people := []Person{}
	for _, row := range rows {
		if len(people) == min(limit, maxMentionablePeople) {
			break
		}
		visible, err := s.canView(ctx, userPrincipal(row.ID, row.Name, row.Email, row.Role, row.Groups), sc.target)
		if err != nil {
			return nil, err
		}
		if visible {
			people = append(people, Person{ID: row.ID.String(), Name: cmp.Or(row.Name, row.Email), Email: row.Email})
		}
	}
	return people, nil
}

func (s *Service) canView(ctx context.Context, p auth.Principal, t Target) (bool, error) {
	found, err := s.scopeOf(ctx, p, t)
	if errors.Is(err, ErrTargetNotFound) {
		return false, nil
	}
	return found.abilities.View, err
}

func userPrincipal(id pgtype.UUID, name, email, role string, groups []string) auth.Principal {
	parsed, _ := auth.ParseRole(role)
	return auth.Principal{Kind: auth.KindUser, ID: id.String(), Name: name, Email: email, Role: parsed, Groups: groups}
}
