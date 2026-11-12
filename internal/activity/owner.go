package activity

import "github.com/jackc/pgx/v5/pgtype"

type ownerKind string

const (
	ownerNone        ownerKind = ""
	ownerDashboard   ownerKind = "dashboard"
	ownerSavedSearch ownerKind = "saved_search"
	ownerObservation ownerKind = "observation"
	ownerGeneral     ownerKind = "general"
)

type Owner struct {
	kind ownerKind
	id   pgtype.UUID
}

func OnDashboard(id pgtype.UUID) Owner { return ownerOf(ownerDashboard, id) }

func OnSavedSearch(id pgtype.UUID) Owner { return ownerOf(ownerSavedSearch, id) }

func OnObservation(id pgtype.UUID) Owner { return ownerOf(ownerObservation, id) }

func General() Owner { return Owner{kind: ownerGeneral} }

func ownerOf(kind ownerKind, id pgtype.UUID) Owner {
	if !id.Valid {
		return Owner{}
	}
	return Owner{kind: kind, id: id}
}

func (o Owner) locks() bool {
	return o.kind == ownerDashboard || o.kind == ownerSavedSearch || o.kind == ownerObservation
}
