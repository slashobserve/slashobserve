package activity

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
)

type Filter struct {
	DashboardID pgtype.UUID
	FolderID    pgtype.UUID
	Types       []string
	Since       time.Time
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
	return f, nil
}
