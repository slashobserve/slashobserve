package revisions

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

var ErrInvalidClaimant = errors.New("claimant has no valid assignee id")

type AssigneeKind string

const (
	AssignBuiltin        AssigneeKind = "builtin"
	AssignGrant          AssigneeKind = "grant"
	AssignServiceAccount AssigneeKind = "service_account"
)

type Claimant struct {
	Kind   AssigneeKind
	ID     string
	Person string
}

func (c Claimant) Key() string {
	if c.Kind == AssignBuiltin && c.ID == "" {
		return string(AssignBuiltin)
	}
	return string(c.Kind) + ":" + c.ID
}

func actsFor(c Claimant, requester auth.Principal) bool {
	if c.Kind == AssignGrant {
		return requester.Kind == auth.KindUser && requester.ID == c.Person
	}
	return true
}

func (c Claimant) assignedID() (pgtype.UUID, error) {
	if c.Kind == AssignBuiltin {
		return pgtype.UUID{}, nil
	}
	id, err := db.ParseUUID(c.ID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("%w: %s", ErrInvalidClaimant, c.Kind)
	}
	return id, nil
}

type Assignee struct {
	Kind AssigneeKind
	ID   pgtype.UUID
}

func (a Assignee) kindText() pgtype.Text {
	return pgtype.Text{String: string(a.Kind), Valid: a.Kind != ""}
}

func (a Assignee) builtin() bool {
	return a.Kind == "" || a.Kind == AssignBuiltin
}

func assigneeOf(row dbsqlc.AgentRequest) Assignee {
	return Assignee{Kind: AssigneeKind(row.AssignedKind), ID: row.AssignedID}
}

func checkAssignee(ctx context.Context, q *dbsqlc.Queries, a Assignee, by auth.Principal) error {
	var err error
	switch a.Kind {
	case AssignGrant:
		personID, parseErr := db.ParseUUID(by.ID)
		if by.Kind != auth.KindUser || parseErr != nil {
			return ErrAssigneeUnavailable
		}
		_, err = q.MentionableGrant(ctx, dbsqlc.MentionableGrantParams{ID: a.ID, UserID: personID})
	case AssignServiceAccount:
		_, err = q.MentionableServiceAccount(ctx, a.ID)
	default:
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAssigneeUnavailable
	}
	if err != nil {
		return fmt.Errorf("read the agent the run was assigned to: %w", err)
	}
	return nil
}
