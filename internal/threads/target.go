package threads

import (
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
)

type Kind string

const (
	KindDashboard   Kind = "dashboard"
	KindAnnotation  Kind = "annotation"
	KindObservation Kind = "observation"
	KindProposal    Kind = "proposal"
)

type Target struct {
	Kind Kind
	ID   pgtype.UUID
}

func ParseTarget(s string) (Target, error) {
	kind, raw, ok := strings.Cut(s, ":")
	if !ok {
		return Target{}, invalidTarget("target must be kind:uuid, with kind dashboard, annotation, observation or proposal")
	}
	switch Kind(kind) {
	case KindDashboard, KindAnnotation, KindObservation, KindProposal:
	default:
		return Target{}, invalidTarget("target kind must be dashboard, annotation, observation or proposal")
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
