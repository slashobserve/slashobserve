package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
)

type Kind string

const (
	KindDashboard   Kind = "dashboard"
	KindFinding     Kind = "finding"
	KindAnnotation  Kind = "annotation"
	KindObservation Kind = "observation"
	KindComment     Kind = "comment"
	KindSavedSearch Kind = "saved_search"
)

const (
	DefaultLimit = 20
	MaxLimit     = 50
	MaxResults   = 200

	annotationMargin = 15 * time.Minute
)

func Kinds() []Kind {
	return []Kind{KindDashboard, KindFinding, KindAnnotation, KindObservation, KindComment, KindSavedSearch}
}

func (k Kind) Valid() bool {
	return slices.Contains(Kinds(), k)
}

type Request struct {
	Q      string
	Kinds  []Kind
	Limit  int
	Cursor string
}

type ResultFolder struct {
	ID    string
	Title string
}

type Target struct {
	Kind  Kind
	ID    string
	Title string
}

type Result struct {
	Kind      Kind
	ID        string
	Title     string
	Snippet   []Segment
	URL       string
	Folder    *ResultFolder
	Target    *Target
	ThreadID  string
	ChangedAt time.Time
}

type Page struct {
	Results []Result
	Next    string
}

type Service struct {
	db        *db.DB
	access    *folders.Access
	publicURL *url.URL
}

func New(database *db.DB, access *folders.Access, publicURL *url.URL) *Service {
	return &Service{db: database, access: access, publicURL: publicURL}
}

func (s *Service) Search(ctx context.Context, by auth.Principal, r Request) (Page, error) {
	query, err := ParseQuery(r.Q)
	if err != nil {
		return Page{}, err
	}
	asOf, offset := time.Now().UTC(), 0
	if r.Cursor != "" {
		if asOf, offset, err = decodeCursor(r.Cursor); err != nil {
			return Page{}, err
		}
	}
	limit := r.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	limit = min(max(limit, 1), MaxLimit, MaxResults-offset)
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return Page{}, err
	}
	audience, err := folders.AudienceOf(by, levels)
	if err != nil {
		return Page{}, err
	}
	rows, err := s.db.Queries().SearchWorkspace(ctx, dbsqlc.SearchWorkspaceParams{
		Words:      query.Words,
		Prefix:     pgtype.Text{String: query.Prefix, Valid: query.Prefix != ""},
		Folders:    audience.Folders,
		Admin:      audience.Admin,
		ReaderKind: audience.ReaderKind,
		ReaderID:   audience.ReaderID,
		AsOf:       pgtype.Timestamptz{Time: asOf, Valid: true},
		Kinds:      kindNames(r.Kinds),
		RowWindow:  httpapi.Int32(offset + limit + 1),
		RowOffset:  httpapi.Int32(offset),
		RowLimit:   httpapi.Int32(limit + 1),
	})
	if err != nil {
		return Page{}, fmt.Errorf("search workspace: %w", err)
	}
	page := Page{Results: make([]Result, 0, min(len(rows), limit))}
	for _, row := range rows[:min(len(rows), limit)] {
		result, err := s.resultOf(row)
		if err != nil {
			return Page{}, err
		}
		page.Results = append(page.Results, result)
	}
	if len(rows) > limit && offset+limit < MaxResults {
		page.Next = encodeCursor(asOf, offset+limit)
	}
	return page, nil
}

func kindNames(kinds []Kind) []string {
	if len(kinds) == 0 {
		kinds = Kinds()
	}
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	return names
}

func (s *Service) resultOf(row dbsqlc.SearchWorkspaceRow) (Result, error) {
	result := Result{
		Kind:      Kind(row.Kind),
		ID:        row.ID.String(),
		Title:     row.Title,
		Snippet:   Segments(row.Headline),
		ChangedAt: row.ChangedAt.Time.UTC(),
	}
	if row.FolderID.Valid {
		result.Folder = &ResultFolder{ID: row.FolderID.String(), Title: row.FolderTitle.String}
	}
	if result.Kind == KindComment {
		result.ThreadID = row.ThreadID.String()
		result.Target = &Target{Kind: Kind(row.SubjectKind), ID: row.SubjectID.String(), Title: row.Title}
	}
	address, err := s.addressOf(row)
	if err != nil {
		return Result{}, err
	}
	result.URL = address
	return result, nil
}

func (s *Service) addressOf(row dbsqlc.SearchWorkspaceRow) (string, error) {
	switch Kind(row.SubjectKind) {
	case KindDashboard:
		return links.Dashboard(s.publicURL, row.SubjectID.String(), row.Title), nil
	case KindFinding:
		return links.Finding(s.publicURL, row.SubjectID.String()), nil
	case KindAnnotation:
		return s.annotationURL(row), nil
	case KindObservation:
		return links.Observation(s.publicURL, row.SubjectID.String()), nil
	}
	return s.savedSearchURL(row)
}

func (s *Service) annotationURL(row dbsqlc.SearchWorkspaceRow) string {
	if !row.AnnotationDashboardID.Valid {
		return ""
	}
	from := row.AnnotationTimeFrom.Time
	to := from
	if row.AnnotationTimeTo.Valid {
		to = row.AnnotationTimeTo.Time
	}
	window := url.Values{
		"from": {strconv.FormatInt(from.Add(-annotationMargin).UnixMilli(), 10)},
		"to":   {strconv.FormatInt(to.Add(annotationMargin).UnixMilli(), 10)},
	}
	return links.Dashboard(s.publicURL, row.AnnotationDashboardID.String(), row.AnnotationDashboardTitle.String) + "?" + window.Encode()
}

func (s *Service) savedSearchURL(row dbsqlc.SearchWorkspaceRow) (string, error) {
	saved := savedsearches.SavedSearch{
		Kind:          savedsearches.Kind(row.SavedKind.String),
		DatasourceUID: row.SavedDatasourceUid.String,
		From:          row.SavedTimeFrom.String,
		To:            row.SavedTimeTo.String,
	}
	if row.SavedQuery.Valid {
		saved.Query = &row.SavedQuery.String
	}
	if err := json.Unmarshal(row.SavedFilters, &saved.Filters); err != nil {
		return "", fmt.Errorf("decode filters of saved search %s: %w", row.SubjectID.String(), err)
	}
	return savedsearches.URL(s.publicURL, saved), nil
}
