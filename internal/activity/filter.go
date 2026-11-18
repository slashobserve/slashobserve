package activity

import (
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
)

const ScopeAdmin = "admin"

type Filter struct {
	DashboardID   pgtype.UUID
	FolderID      pgtype.UUID
	ObservationID pgtype.UUID
	ActorID       pgtype.UUID
	Types         []string
	Since         time.Time
	Until         time.Time
	AdminOnly     bool
}

func uuidParam(q url.Values, name string) (pgtype.UUID, error) {
	raw := q.Get(name)
	if raw == "" {
		return pgtype.UUID{}, nil
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("%s must be a UUID", name)
	}
	return id, nil
}

func timeParam(q url.Values, name string) (time.Time, error) {
	raw := q.Get(name)
	if raw == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be an RFC 3339 time", name)
	}
	return at, nil
}

func ParseFilter(q url.Values) (Filter, error) {
	var f Filter
	var err error
	for _, param := range []struct {
		name string
		dst  *pgtype.UUID
	}{{"dashboard_id", &f.DashboardID}, {"folder_id", &f.FolderID}, {"observation_id", &f.ObservationID}, {"actor_id", &f.ActorID}} {
		if *param.dst, err = uuidParam(q, param.name); err != nil {
			return Filter{}, err
		}
	}
	switch scope := q.Get("scope"); scope {
	case "":
	case ScopeAdmin:
		f.AdminOnly = true
	default:
		return Filter{}, fmt.Errorf("scope must be %q", ScopeAdmin)
	}
	types := q["type"]
	if len(types) > len(knownTypes) {
		return Filter{}, fmt.Errorf("type may be given at most %d times", len(knownTypes))
	}
	for _, t := range types {
		if !Known(t) {
			return Filter{}, fmt.Errorf("type %q is not an activity type", t)
		}
	}
	f.Types = types
	if f.Since, err = timeParam(q, "since"); err != nil {
		return Filter{}, err
	}
	if f.Until, err = timeParam(q, "until"); err != nil {
		return Filter{}, err
	}
	return f, nil
}
