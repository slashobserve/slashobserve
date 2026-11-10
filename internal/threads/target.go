package threads

import (
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
)

type Kind string

const (
	KindDashboard  Kind = "dashboard"
	KindFinding    Kind = "finding"
	KindAnnotation Kind = "annotation"
)

type Target struct {
	Kind Kind
	ID   pgtype.UUID
}

func ParseTarget(s string) (Target, error) {
	kind, raw, ok := strings.Cut(s, ":")
	if !ok {
		return Target{}, invalidTarget("target must be kind:uuid, with kind dashboard, finding or annotation")
	}
	switch Kind(kind) {
	case KindDashboard, KindFinding, KindAnnotation:
	default:
		return Target{}, invalidTarget("target kind must be dashboard, finding or annotation")
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		return Target{}, invalidTarget("target id must be a UUID")
	}
	return Target{Kind: Kind(kind), ID: id}, nil
}

func (t Target) String() string {
	return string(t.Kind) + ":" + t.ID.String()
}

func invalidTarget(message string) error {
	return &InvalidInputError{Code: CodeInvalidTarget, Message: message}
}
