package activity

import (
	"errors"
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

func ParseFilter(q url.Values) (Filter, error) {
	var f Filter
	var err error
	if raw := q.Get("dashboard_id"); raw != "" {
		if f.DashboardID, err = db.ParseUUID(raw); err != nil {
			return Filter{}, errors.New("dashboard_id must be a UUID")
		}
	}
	if raw := q.Get("folder_id"); raw != "" {
		if f.FolderID, err = db.ParseUUID(raw); err != nil {
			return Filter{}, errors.New("folder_id must be a UUID")
		}
	}
	if raw := q.Get("observation_id"); raw != "" {
		if f.ObservationID, err = db.ParseUUID(raw); err != nil {
			return Filter{}, errors.New("observation_id must be a UUID")
		}
	}
	if raw := q.Get("actor_id"); raw != "" {
		if f.ActorID, err = db.ParseUUID(raw); err != nil {
			return Filter{}, errors.New("actor_id must be a UUID")
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
	if raw := q.Get("since"); raw != "" {
		if f.Since, err = time.Parse(time.RFC3339, raw); err != nil {
			return Filter{}, errors.New("since must be an RFC 3339 time")
		}
	}
	if raw := q.Get("until"); raw != "" {
		if f.Until, err = time.Parse(time.RFC3339, raw); err != nil {
			return Filter{}, errors.New("until must be an RFC 3339 time")
		}
	}
	return f, nil
}
