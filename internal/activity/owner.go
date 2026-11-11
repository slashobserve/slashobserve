package activity

import "github.com/jackc/pgx/v5/pgtype"

type ownerKind string

const (
	ownerNone        ownerKind = ""
	ownerDashboard   ownerKind = "dashboard"
	ownerFinding     ownerKind = "finding"
	ownerSavedSearch ownerKind = "saved_search"
	ownerGeneral     ownerKind = "general"
)

type Owner struct {
	kind ownerKind
	id   pgtype.UUID
}

func OnDashboard(id pgtype.UUID) Owner { return ownerOf(ownerDashboard, id) }

func OnFinding(id pgtype.UUID) Owner { return ownerOf(ownerFinding, id) }

func OnSavedSearch(id pgtype.UUID) Owner { return ownerOf(ownerSavedSearch, id) }

func General() Owner { return Owner{kind: ownerGeneral} }

func ownerOf(kind ownerKind, id pgtype.UUID) Owner {
	if !id.Valid {
		return Owner{}
	}
	return Owner{kind: kind, id: id}
}

func (o Owner) locks() bool {
	return o.kind == ownerDashboard || o.kind == ownerFinding || o.kind == ownerSavedSearch
}
