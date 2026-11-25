package annotations

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/placement"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/usertext"
)

const (
	MaxTextRunes = 2000
	MaxTags      = 10
)

const (
	MessageInvalidTags  = "tags must be at most 10 lowercase tags of 1 to 40 letters, digits, _, . or -, starting with a letter or digit"
	messageInvalidTime  = "time_to must not be before time_from, and both must be RFC 3339 times from 1970-01-01 to 2099-12-31"
	messageInvalidPanel = "panel_id must be 1 to 64 characters without NUL and needs a dashboard_id"
	messageUnknownPanel = "panel_id is not a panel of the dashboard's latest version; list the dashboard's panels and use one of their ids"
	messageEmptyUpdate  = "give text, tags or both to change"
)

var latestTime = time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)

var (
	ErrNotFound   = errors.New("annotation not found")
	ErrForbidden  = errors.New("the caller's level does not allow this")
	ErrNotCreator = errors.New("only the creator or an admin can change or delete this annotation")

	ErrInvalidDashboardID = errors.New("dashboard_id must be a UUID")
)

var tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,39}$`)

type InvalidError struct {
	Code    string
	Message string
	Cause   error
}

func (e *InvalidError) Error() string { return e.Message }

func (e *InvalidError) Unwrap() error { return e.Cause }

type Annotation struct {
	ID            string
	DashboardID   string
	PanelID       string
	From          time.Time
	To            *time.Time
	Text          string
	Tags          []string
	CreatedBy     string
	CreatedByKind string
	CreatedByName string
	Origin        origin.Origin
	CreatedAt     time.Time
}

func (a Annotation) Actor() (actor.Actor, bool) {
	if a.CreatedByKind == "" || a.CreatedByName == "" {
		return actor.Actor{}, false
	}
	return actor.Actor{Kind: auth.PrincipalKind(a.CreatedByKind), ID: a.CreatedBy, Name: a.CreatedByName, Origin: a.Origin}, true
}

type CreateInput struct {
	DashboardID pgtype.UUID
	PanelID     string
	From        time.Time
	To          *time.Time
	Text        string
	Tags        []string
	AnswerOf    pgtype.UUID
	CopyOf      placement.Copied
}

type CreateRequest struct {
	DashboardID *string  `json:"dashboard_id"`
	PanelID     string   `json:"panel_id"`
	TimeFrom    *string  `json:"time_from"`
	TimeTo      *string  `json:"time_to"`
	Text        string   `json:"text"`
	Tags        []string `json:"tags"`
}

func ParseCreate(req CreateRequest) (CreateInput, error) {
	in := CreateInput{PanelID: req.PanelID, Text: req.Text, Tags: req.Tags}
	if req.DashboardID != nil {
		id, err := db.ParseUUID(*req.DashboardID)
		if err != nil {
			return in, ErrInvalidDashboardID
		}
		in.DashboardID = id
	}
	if req.TimeFrom != nil {
		from, err := time.Parse(time.RFC3339, *req.TimeFrom)
		if err != nil || !validTime(from) {
			return in, &InvalidError{Code: "invalid_time", Message: messageInvalidTime}
		}
		in.From = from
	}
	if req.TimeTo != nil {
		to, err := time.Parse(time.RFC3339, *req.TimeTo)
		if err != nil || !validTime(to) {
			return in, &InvalidError{Code: "invalid_time", Message: messageInvalidTime}
		}
		in.To = &to
	}
	return in, nil
}

type UpdateInput struct {
	Text *string   `json:"text"`
	Tags *[]string `json:"tags"`
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

type Panels interface {
	LatestVersion(ctx context.Context, id pgtype.UUID) (int, error)
	HasPanel(ctx context.Context, id pgtype.UUID, version int, panelID string) (bool, error)
}

type Option func(*Service)

func WithPanels(p Panels) Option {
	return func(s *Service) { s.panels = p }
}

type Service struct {
	store  *store
	access *folders.Access
	panels Panels
}

func New(database *db.DB, access *folders.Access, opts ...Option) *Service {
	s := &Service{store: &store{db: database}, access: access}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
	if err := s.checkPanel(ctx, clean); err != nil {
		return Annotation{}, err
	}
	creator := actor.In(ctx, by, "")
	if _, err := db.ParseUUID(creator.ID); err != nil {
		return Annotation{}, ErrForbidden
	}
	a, err := s.store.insert(ctx, clean, creator)
	if err != nil {
		return Annotation{}, err
	}
	a.CreatedByKind = string(creator.Kind)
	a.CreatedByName = creator.Name
	a.Origin = creator.Origin
	return a, nil
}

func (s *Service) Update(ctx context.Context, by auth.Principal, id pgtype.UUID, in UpdateInput) (Annotation, error) {
	if in.Text == nil && in.Tags == nil {
		return Annotation{}, &InvalidError{Code: "invalid_request", Message: messageEmptyUpdate}
	}
	a, err := s.mayChange(ctx, by, id)
	if err != nil {
		return Annotation{}, err
	}
	text, tags := a.Text, a.Tags
	if in.Text != nil {
		if text, err = cleanText(*in.Text); err != nil {
			return Annotation{}, err
		}
	}
	if in.Tags != nil {
		var ok bool
		if tags, ok = CleanTags(*in.Tags); !ok {
			return Annotation{}, &InvalidError{Code: "invalid_tags", Message: MessageInvalidTags}
		}
	}
	if text == a.Text && slices.Equal(tags, a.Tags) {
		return s.store.named(ctx, id)
	}
	return s.store.update(ctx, id, text, tags, actor.In(ctx, by, ""))
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error {
	if _, err := s.mayChange(ctx, by, id); err != nil {
		return err
	}
	return s.store.remove(ctx, id, actor.In(ctx, by, ""))
}

func (s *Service) mayChange(ctx context.Context, by auth.Principal, id pgtype.UUID) (Annotation, error) {
	a, err := s.store.get(ctx, id)
	if err != nil {
		return Annotation{}, err
	}
	if a.DashboardID != "" {
		dashboardID, err := db.ParseUUID(a.DashboardID)
		if err != nil {
			return Annotation{}, err
		}
		if _, err := s.dashboard(ctx, by, dashboardID); err != nil {
			return Annotation{}, err
		}
	}
	if by.Role != auth.RoleAdmin && (a.CreatedBy == "" || a.CreatedBy != by.ID) {
		return Annotation{}, ErrNotCreator
	}
	return a, nil
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

func (s *Service) Access(ctx context.Context, by auth.Principal, id pgtype.UUID) (pgtype.UUID, folders.Abilities, error) {
	a, err := s.store.get(ctx, id)
	if err != nil {
		return pgtype.UUID{}, folders.Abilities{}, err
	}
	if a.DashboardID == "" {
		levels, err := s.access.Levels(ctx, by)
		if errors.Is(err, folders.ErrForbidden) {
			return pgtype.UUID{}, folders.Abilities{}, ErrNotFound
		}
		if err != nil {
			return pgtype.UUID{}, folders.Abilities{}, err
		}
		return pgtype.UUID{}, levels.General(), nil
	}
	dashboardID, err := db.ParseUUID(a.DashboardID)
	if err != nil {
		return pgtype.UUID{}, folders.Abilities{}, err
	}
	abilities, err := s.dashboard(ctx, by, dashboardID)
	if err != nil {
		return pgtype.UUID{}, folders.Abilities{}, err
	}
	return dashboardID, abilities, nil
}

func (s *Service) mayCreate(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	if !dashboardID.Valid {
		if !by.Role.AtLeast(auth.RoleEditor) {
			return forbidden.Need(ErrForbidden, forbidden.NeedsRole(string(auth.RoleEditor)))
		}
		return nil
	}
	access, err := s.dashboardAccess(ctx, by, dashboardID)
	if err != nil {
		return err
	}
	if !access.Abilities.Edit {
		return forbidden.Need(ErrForbidden, forbidden.NeedsAccess("edit", access.Folder.Title))
	}
	return nil
}

func (s *Service) dashboard(ctx context.Context, by auth.Principal, id pgtype.UUID) (folders.Abilities, error) {
	access, err := s.dashboardAccess(ctx, by, id)
	return access.Abilities, err
}

func (s *Service) dashboardAccess(ctx context.Context, by auth.Principal, id pgtype.UUID) (folders.DashboardAccess, error) {
	access, err := s.access.Dashboard(ctx, by, id)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden):
		return folders.DashboardAccess{}, ErrNotFound
	case err != nil:
		return folders.DashboardAccess{}, err
	}
	return access, nil
}

func (s *Service) checkPanel(ctx context.Context, in CreateInput) error {
	if in.PanelID == "" || s.panels == nil {
		return nil
	}
	latest, err := s.panels.LatestVersion(ctx, in.DashboardID)
	if errors.Is(err, dashboards.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	known, err := s.panels.HasPanel(ctx, in.DashboardID, latest, in.PanelID)
	if err != nil {
		return err
	}
	if !known {
		return &InvalidError{Code: "unknown_panel", Message: messageUnknownPanel}
	}
	return nil
}

func cleanText(raw string) (string, error) {
	text, err := usertext.Clean("text", raw, usertext.Text(MaxTextRunes))
	if err != nil {
		return "", &InvalidError{Code: "invalid_text", Message: err.Error(), Cause: err}
	}
	return text, nil
}

func validate(in CreateInput) (CreateInput, error) {
	text, err := cleanText(in.Text)
	if err != nil {
		return in, err
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
		return in, &InvalidError{Code: "invalid_tags", Message: MessageInvalidTags}
	}
	in.Tags = tags
	return in, nil
}

func validTime(t time.Time) bool {
	return !t.Before(time.Unix(0, 0)) && t.Before(latestTime)
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
