package savedsearches

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/links"
)

type Kind string

const (
	KindLogs   Kind = "logs"
	KindTraces Kind = "traces"
)

const (
	CodeInvalid               = "invalid_saved_search"
	CodeUnknownDatasource     = "unknown_datasource"
	CodeUnsupportedDatasource = "unsupported_datasource"
)

var (
	ErrNotFound        = errors.New("saved search not found")
	ErrFolderNotFound  = errors.New("folder not found")
	ErrInvalidCursor   = errors.New("invalid cursor")
	ErrNothingToUpdate = errors.New("give a title or a folder_id to change")
)

type InvalidInputError struct {
	Code    string
	Message string
}

func (e *InvalidInputError) Error() string { return e.Message }

type Filters struct {
	Columns []string `json:"columns,omitempty"`
	Service string   `json:"service,omitempty"`
	Span    string   `json:"span,omitempty"`
	Min     string   `json:"min,omitempty"`
	Max     string   `json:"max,omitempty"`
	Errors  bool     `json:"errors,omitempty"`
}

type SavedSearch struct {
	ID            string
	Kind          Kind
	Title         string
	DatasourceUID string
	Query         *string
	Filters       Filters
	From, To      string
	FolderID      string
	Actor         actor.Actor
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Folder        folders.Folder
	Abilities     folders.Abilities
	URL           string
}

type CreateInput struct {
	Kind          Kind
	Title         string
	DatasourceUID string
	Query         *string
	Filters       []byte
	From, To      string
	FolderID      pgtype.UUID
}

type UpdateInput struct {
	Title    *string
	FolderID pgtype.UUID
}

type ListFilter struct {
	Kind     Kind
	FolderID pgtype.UUID
}

func URL(base *url.URL, s SavedSearch) string {
	if s.Kind == KindLogs {
		return links.Logs(base, s.address())
	}
	return links.Traces(base, s.address())
}

func (s SavedSearch) address() url.Values {
	query := url.Values{"ds": {s.DatasourceUID}, "from": {s.From}, "to": {s.To}}
	if s.Query != nil {
		query.Set("q", *s.Query)
	}
	if s.Kind == KindLogs {
		if len(s.Filters.Columns) > 0 {
			query.Set("cols", strings.Join(s.Filters.Columns, ","))
		}
		return query
	}
	for name, value := range map[string]string{"service": s.Filters.Service, "span": s.Filters.Span, "min": s.Filters.Min, "max": s.Filters.Max} {
		if value != "" {
			query.Set(name, value)
		}
	}
	if s.Filters.Errors {
		query.Set("errors", "1")
	}
	return query
}
