package revisions

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
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
