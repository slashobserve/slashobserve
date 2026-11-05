package annotations

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	MaxTextRunes = 2000
	MaxTags      = 10
)

const (
	messageInvalidText  = "text must be 1 to 2000 characters without control or bidirectional override characters"
	messageInvalidTags  = "tags must be at most 10 lowercase tags of 1 to 40 letters, digits, _, . or -, starting with a letter or digit"
	messageInvalidTime  = "time_to must not be before time_from, and both must be after 1970-01-01"
	messageInvalidPanel = "panel_id must be 1 to 64 characters without NUL and needs a dashboard_id"
)

var (
	ErrNotFound   = errors.New("annotation not found")
	ErrForbidden  = errors.New("the caller's level does not allow this")
	ErrNotCreator = errors.New("only the creator or an admin can delete this annotation")
)

var tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,39}$`)

type InvalidError struct {
	Code    string
	Message string
}

func (e *InvalidError) Error() string { return e.Message }

type Annotation struct {
	ID          string
	DashboardID string
	PanelID     string
	From        time.Time
	To          *time.Time
	Text        string
	Tags        []string
	CreatedBy   string
	CreatedAt   time.Time
}

type CreateInput struct {
	DashboardID pgtype.UUID
	PanelID     string
	From        time.Time
	To          *time.Time
	Text        string
	Tags        []string
}

type ListQuery struct {
	DashboardID pgtype.UUID
	From        *time.Time
	To          *time.Time
	Tags        []string
	After       *httpapi.TimeCursor
	Limit       int
}

type Page struct {
	Annotations []Annotation
	Next        *httpapi.TimeCursor
}

type Service struct {
	store  *store
	access *folders.Access
}

func New(database *db.DB, access *folders.Access) *Service {
	return &Service{store: &store{db: database}, access: access}
}

func (s *Service) Create(ctx context.Context, by auth.Principal, in CreateInput) (Annotation, error) {
	if in.From.IsZero() {
		in.From = time.Now()
	}
	clean, err := validate(in)
	if err != nil {
		return Annotation{}, err
	}
	if err := s.mayCreate(ctx, by, clean.DashboardID); err != nil {
		return Annotation{}, err
	}
	creator, err := db.ParseUUID(by.ID)
	if err != nil {
		return Annotation{}, ErrForbidden
	}
	return s.store.insert(ctx, clean, creator)
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error {
	a, err := s.store.get(ctx, id)
	if err != nil {
		return err
	}
	if a.DashboardID != "" {
		dashboardID, err := db.ParseUUID(a.DashboardID)
		if err != nil {
			return err
		}
		if _, err := s.dashboard(ctx, by, dashboardID); err != nil {
			return err
		}
	}
	if by.Role != auth.RoleAdmin && (a.CreatedBy == "" || a.CreatedBy != by.ID) {
		return ErrNotCreator
	}
	return s.store.remove(ctx, id)
}

func (s *Service) List(ctx context.Context, by auth.Principal, q ListQuery) (Page, error) {
	if q.DashboardID.Valid {
		if _, err := s.dashboard(ctx, by, q.DashboardID); err != nil {
			return Page{}, err
		}
	}
	rows, err := s.store.list(ctx, q, q.Limit+1)
	if err != nil {
		return Page{}, err
	}
	page := Page{Annotations: rows}
	if len(rows) > q.Limit {
		page.Annotations = rows[:q.Limit]
		last := page.Annotations[q.Limit-1]
		lastID, err := db.ParseUUID(last.ID)
		if err != nil {
			return Page{}, err
		}
		page.Next = &httpapi.TimeCursor{Time: last.From, ID: lastID}
	}
	return page, nil
}

func (s *Service) mayCreate(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	if !dashboardID.Valid {
		if !by.Role.AtLeast(auth.RoleEditor) {
			return ErrForbidden
		}
		return nil
	}
	abilities, err := s.dashboard(ctx, by, dashboardID)
	if err != nil {
		return err
	}
	if !abilities.Edit {
		return ErrForbidden
	}
	return nil
}

func (s *Service) dashboard(ctx context.Context, by auth.Principal, id pgtype.UUID) (folders.Abilities, error) {
	access, err := s.access.Dashboard(ctx, by, id)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden):
		return folders.Abilities{}, ErrNotFound
	case err != nil:
		return folders.Abilities{}, err
	}
	return access.Abilities, nil
}

func validate(in CreateInput) (CreateInput, error) {
	text, ok := revisions.CleanPrompt(in.Text)
	if !ok || utf8.RuneCountInString(text) > MaxTextRunes {
		return in, &InvalidError{Code: "invalid_text", Message: messageInvalidText}
	}
	in.Text = text
	if in.PanelID != "" && (!in.DashboardID.Valid || revisions.ValidPanelID(in.PanelID) != nil) {
		return in, &InvalidError{Code: "invalid_panel_id", Message: messageInvalidPanel}
	}
	if !validTime(in.From) || in.To != nil && (!validTime(*in.To) || in.To.Before(in.From)) {
		return in, &InvalidError{Code: "invalid_time", Message: messageInvalidTime}
	}
	tags, ok := CleanTags(in.Tags)
	if !ok {
		return in, &InvalidError{Code: "invalid_tags", Message: messageInvalidTags}
	}
	in.Tags = tags
	return in, nil
}

func validTime(t time.Time) bool {
	return !t.Before(time.Unix(0, 0))
}

func CleanTags(raw []string) ([]string, bool) {
	if len(raw) > MaxTags {
		return nil, false
	}
	tags := make([]string, 0, len(raw))
	for _, t := range raw {
		if !tagPattern.MatchString(t) {
			return nil, false
		}
		if !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	return tags, true
}
