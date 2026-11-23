package findings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auditfolder"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/placement"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	savedDashboardSummary = "Saved from a finding"

	actionRunView audit.Action = "run.view"
)

var ErrNoObservations = errors.New("findings need an observation service")

type Service struct {
	store        *store
	access       *folders.Access
	annotations  *annotations.Service
	dashboards   *dashboards.Service
	observations Observations
	threads      ThreadReader
	answerReads  AnswerReadRecorder
	logger       *slog.Logger
}

func New(database *db.DB, access *folders.Access, ann *annotations.Service, dash *dashboards.Service, observations Observations, logger *slog.Logger) (*Service, error) {
	if observations == nil {
		return nil, ErrNoObservations
	}
	return &Service{store: &store{db: database}, access: access, annotations: ann, dashboards: dash, observations: observations, logger: logger}, nil
}

func (s *Service) ReadThreadsWith(read ThreadReader) {
	s.threads = read
}

func (s *Service) RecordAnswerReadsWith(record AnswerReadRecorder) {
	s.answerReads = record
}

type SaveInput struct {
	AgentRequestID  pgtype.UUID
	DashboardID     pgtype.UUID
	AskDashboardID  pgtype.UUID
	FolderID        pgtype.UUID
	Title, Markdown string
	Evidence        []spec.Panel
	From, To        time.Time
	AsOf            *time.Time
	SuggestedChange string
	ObserveWorthy   bool
}

func (s *Service) Save(ctx context.Context, by auth.Principal, in SaveInput) (Finding, bool, error) {
	if in.DashboardID.Valid && in.FolderID.Valid {
		return Finding{}, false, ErrPlacement
	}
	if in.DashboardID.Valid && in.AskDashboardID.Valid {
		return Finding{}, false, ErrAskPlacement
	}
	for _, id := range []pgtype.UUID{in.DashboardID, in.AskDashboardID} {
		if !id.Valid {
			continue
		}
		if err := s.viewable(ctx, by, id); err != nil {
			return Finding{}, false, err
		}
	}
	if in.Evidence == nil {
		in.Evidence = []spec.Panel{}
	}
	conclusion := Conclusion{
		RunID: in.AgentRequestID, DashboardID: in.DashboardID, FolderID: in.FolderID,
		Title: in.Title, Markdown: in.Markdown, Evidence: in.Evidence, From: in.From, To: in.To,
	}
	conclude := func(q *dbsqlc.Queries) (pgtype.UUID, bool, error) {
		return s.observations.WriteConclusion(ctx, q, by, conclusion)
	}
	chatOnly, err := s.answersWithoutObservation(ctx, in.AgentRequestID)
	if err != nil {
		return Finding{}, false, err
	}
	if chatOnly {
		conclude = func(*dbsqlc.Queries) (pgtype.UUID, bool, error) { return pgtype.UUID{}, false, nil }
	}
	return s.store.insert(ctx, actor.Of(by, origin.From(ctx), in.AgentRequestID.String()), in, inheritingRestricted(ctx, in.AgentRequestID, conclude))
}

func inheritingRestricted(ctx context.Context, runID pgtype.UUID, next conclude) conclude {
	if !runID.Valid {
		return next
	}
	return func(q *dbsqlc.Queries) (pgtype.UUID, bool, error) {
		row, err := q.GetAgentRequestByID(ctx, runID)
		if err != nil {
			return pgtype.UUID{}, false, fmt.Errorf("read the run of the answer: %w", err)
		}
		if _, err := placement.InheritRestricted(ctx, q, row); err != nil {
			return pgtype.UUID{}, false, err
		}
		return next(q)
	}
}

func (s *Service) answersWithoutObservation(ctx context.Context, requestID pgtype.UUID) (bool, error) {
	if !requestID.Valid {
		return false, nil
	}
	observationID, err := s.store.runObservation(ctx, requestID)
	return !observationID.Valid, err
}

func (s *Service) SaveAsObservation(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (Finding, bool, error) {
	f, found, err := s.store.forRequest(ctx, requestID)
	if err == nil && !found {
		err = ErrNotFound
	}
	if err != nil {
		return Finding{}, false, err
	}
	id, err := db.ParseUUID(f.ID)
	if err != nil {
		return Finding{}, false, err
	}
	if _, _, err := s.place(ctx, by, f); err != nil {
		return Finding{}, false, err
	}
	if f.Saved() {
		f, err = s.Get(ctx, by, id)
		return f, true, err
	}
	if !isRequester(by, f) {
		return Finding{}, false, ErrRequesterOnly
	}
	conclusion := Conclusion{RunID: requestID, Title: f.Title, Markdown: f.Markdown, Evidence: f.Evidence, From: f.From, To: f.To}
	kept, err := placement.AnswerKeptWithAdmins(ctx, s.store.db.Queries(), requestID)
	if err != nil {
		return Finding{}, false, err
	}
	if kept {
		if conclusion.FolderID, err = s.restrictedAnswerFolder(ctx, requestID); err != nil {
			return Finding{}, false, err
		}
	}
	if f.DashboardID != "" {
		if conclusion.DashboardID, err = db.ParseUUID(f.DashboardID); err != nil {
			return Finding{}, false, err
		}
	}
	attached, err := s.store.attachObservation(ctx, id, func(q *dbsqlc.Queries) (pgtype.UUID, bool, error) {
		return s.observations.WriteConclusion(ctx, q, by, conclusion)
	})
	if err != nil {
		return Finding{}, false, err
	}
	f, err = s.Get(ctx, by, id)
	return f, !attached, err
}

func (s *Service) restrictedAnswerFolder(ctx context.Context, runID pgtype.UUID) (pgtype.UUID, error) {
	folder, err := s.store.db.Queries().RestrictedAnswerFolder(ctx, dbsqlc.RestrictedAnswerFolderParams{RunID: runID, AuditFolderKey: auditfolder.FolderKey})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("read the folder only Admins read for an answer that read Admin-only data: %w", err)
	}
	if !folder.Valid {
		return pgtype.UUID{}, folders.ErrRestrictedAnswer
	}
	return folder, nil
}

func (s *Service) ShowIn(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, requestID, observationID pgtype.UUID) error {
	f, found, err := s.store.forRequestIn(ctx, q, requestID)
	if err == nil && !found {
		err = ErrNotFound
	}
	if err != nil {
		return err
	}
	id, err := db.ParseUUID(f.ID)
	if err != nil {
		return err
	}
	current, err := q.LockFinding(ctx, id)
	if err != nil {
		return fmt.Errorf("lock finding: %w", err)
	}
	if current == observationID {
		return nil
	}
	conclusion := Conclusion{RunID: requestID, Into: observationID, Title: f.Title, Markdown: f.Markdown, Evidence: f.Evidence, From: f.From, To: f.To}
	if _, _, err := s.observations.WriteConclusion(ctx, q, by, conclusion); err != nil {
		return err
	}
	if current.Valid {
		return nil
	}
	if _, err := q.SetFindingObservation(ctx, dbsqlc.SetFindingObservationParams{ID: id, ObservationID: observationID}); err != nil {
		return fmt.Errorf("show finding in observation: %w", err)
	}
	return nil
}

func (s *Service) HideFrom(ctx context.Context, q *dbsqlc.Queries, requestID, observationID pgtype.UUID) error {
	if err := q.UnsetFindingObservation(ctx, dbsqlc.UnsetFindingObservationParams{AgentRequestID: requestID, ObservationID: observationID}); err != nil {
		return fmt.Errorf("take the answer back from the observation: %w", err)
	}
	return nil
}

func (s *Service) Get(ctx context.Context, by auth.Principal, id pgtype.UUID) (Finding, error) {
	f, err := s.store.get(ctx, id)
	if err != nil {
		return Finding{}, err
	}
	f, folder, abilities, err := s.readPlace(ctx, by, f)
	if err != nil {
		return Finding{}, err
	}
	if f.Saved() {
		f.Folder = &folder
	}
	f.Abilities = abilities
	return s.askOnIfViewable(ctx, by, f)
}

func (s *Service) Read(ctx context.Context, by auth.Principal, id pgtype.UUID) (Finding, error) {
	f, err := s.Get(ctx, by, id)
	if errors.Is(err, ErrNotFound) {
		s.recordRefusedRead(ctx, id)
	}
	if err != nil || f.Saved() || s.answerReads == nil {
		return f, err
	}
	run, err := db.ParseUUID(f.AgentRequestID)
	if err != nil {
		return Finding{}, fmt.Errorf("answer run id: %w", err)
	}
	s.answerReads(ctx, run)
	return f, nil
}

func (s *Service) recordRefusedRead(ctx context.Context, id pgtype.UUID) {
	err := audit.RecordNow(ctx, audit.Event{
		Action:  actionRunView,
		Outcome: audit.OutcomeDenied,
		Reason:  "not_found",
		Status:  http.StatusNotFound,
		Target:  audit.Target{Kind: "finding", ID: id.String()},
	})
	if err != nil && !errors.Is(err, audit.ErrNotInstalled) {
		s.logger.WarnContext(ctx, "refused read not recorded", logattrs.Failure("audit_record_failed", err, slog.String("finding_id", id.String()))...)
	}
}

func (s *Service) ForRequest(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (Finding, bool, error) {
	f, found, err := s.store.forRequest(ctx, requestID)
	if err != nil || !found {
		return Finding{}, false, err
	}
	f, _, _, err = s.readPlace(ctx, by, f)
	switch {
	case errors.Is(err, ErrNotFound):
		return Finding{}, false, nil
	case err != nil:
		return Finding{}, false, err
	}
	f, err = s.askOnIfViewable(ctx, by, f)
	return f, err == nil, err
}

func (s *Service) askOnIfViewable(ctx context.Context, by auth.Principal, f Finding) (Finding, error) {
	if f.AskOnID == "" {
		return f, nil
	}
	id, err := db.ParseUUID(f.AskOnID)
	if err != nil {
		return Finding{}, err
	}
	switch err := s.viewable(ctx, by, id); {
	case errors.Is(err, ErrNotFound):
		f.AskOnID, f.AskOnTitle = "", ""
	case err != nil:
		return Finding{}, err
	}
	return f, nil
}

func (s *Service) EvidenceDashboard(ctx context.Context, by auth.Principal, id pgtype.UUID) (*spec.Dashboard, error) {
	f, err := s.Get(ctx, by, id)
	if err != nil {
		return nil, err
	}
	return f.Spec(), nil
}

type ListFilter struct {
	DashboardID pgtype.UUID
	FolderID    pgtype.UUID
}

func (s *Service) List(ctx context.Context, by auth.Principal, filter ListFilter, limit int, cursor string) ([]Finding, string, error) {
	if filter.DashboardID.Valid && filter.FolderID.Valid {
		return nil, "", ErrPlacement
	}
	after, ok := httpapi.DecodeTimeCursor(cursor)
	if !ok {
		return nil, "", ErrInvalidCursor
	}
	visible, err := s.visibleFolders(ctx, by)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.listRows(ctx, by, filter, visible, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = httpapi.EncodeTimeCursor(rows[limit-1].CreatedAt, rows[limit-1].ID)
	}
	return rows, next, nil
}

func (s *Service) listRows(ctx context.Context, by auth.Principal, filter ListFilter, visible []pgtype.UUID, after *httpapi.TimeCursor, limit int) ([]Finding, error) {
	switch {
	case filter.DashboardID.Valid:
		if err := s.viewable(ctx, by, filter.DashboardID); err != nil {
			return nil, err
		}
		return s.store.listByDashboard(ctx, filter.DashboardID, visible, after, limit)
	case filter.FolderID.Valid:
		if _, _, err := s.access.Folder(ctx, by, filter.FolderID); err != nil {
			if errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrForbidden) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		return s.store.listInFolder(ctx, filter.FolderID, visible, after, limit)
	}
	return s.store.listBy(ctx, by, visible, after, limit)
}

func (s *Service) visibleFolders(ctx context.Context, by auth.Principal) ([]pgtype.UUID, error) {
	levels, err := s.access.Levels(ctx, by)
	if errors.Is(err, folders.ErrForbidden) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return levels.Visible(), nil
}

func (s *Service) pinnable(ctx context.Context, by auth.Principal, id pgtype.UUID) (Finding, error) {
	f, err := s.Get(ctx, by, id)
	switch {
	case err != nil:
		return Finding{}, err
	case !f.Saved():
		return Finding{}, ErrNotSaved
	case f.DashboardID == "":
		return Finding{}, ErrNoDashboard
	}
	return f, nil
}

func (s *Service) Pin(ctx context.Context, by auth.Principal, id pgtype.UUID) (annotations.Annotation, bool, error) {
	f, err := s.pinnable(ctx, by, id)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	if f.AnnotationID != "" {
		a, err := s.pinned(ctx, f)
		if !errors.Is(err, ErrNotFound) {
			return a, false, err
		}
	}
	dashboardID, err := db.ParseUUID(f.DashboardID)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	pin, err := pinOf(f, dashboardID)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	a, err := s.annotations.Create(ctx, by, pin)
	switch {
	case errors.Is(err, annotations.ErrNotFound):
		return annotations.Annotation{}, false, ErrNotFound
	case err != nil:
		return annotations.Annotation{}, false, err
	}
	annotationID, err := db.ParseUUID(a.ID)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	observationID, err := db.ParseUUID(f.ObservationID)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	won, err := s.store.setAnnotation(ctx, actor.Of(by, origin.From(ctx), ""), id, dashboardID, observationID, annotationID)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	if won {
		return a, true, nil
	}
	if err := s.annotations.Delete(ctx, by, annotationID); err != nil {
		s.logger.WarnContext(ctx, "delete the annotation of a pin that lost the race", logattrs.Failure("finding_pin_cleanup_failed", err, slog.String("finding_id", f.ID), slog.String("annotation_id", a.ID))...)
	}
	winner, err := s.store.get(ctx, id)
	if err != nil {
		return annotations.Annotation{}, false, err
	}
	a, err = s.pinned(ctx, winner)
	return a, false, err
}

func pinOf(f Finding, dashboardID pgtype.UUID) (annotations.CreateInput, error) {
	pin := annotations.CreateInput{DashboardID: dashboardID, From: f.CreatedAt, Text: PinText(f), Tags: []string{PinTag}}
	if f.RequestTo != nil {
		pin.From = *f.RequestTo
	}
	run, err := runOf(f)
	pin.AnswerOf = run
	return pin, err
}

func (s *Service) pinned(ctx context.Context, f Finding) (annotations.Annotation, error) {
	if f.AnnotationID == "" {
		return annotations.Annotation{}, ErrNotFound
	}
	id, err := db.ParseUUID(f.AnnotationID)
	if err != nil {
		return annotations.Annotation{}, err
	}
	return s.store.annotation(ctx, id)
}

func (s *Service) SaveAsDashboard(ctx context.Context, by auth.Principal, id pgtype.UUID) (dashboards.Dashboard, dashboards.Version, error) {
	f, err := s.Get(ctx, by, id)
	if err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, err
	}
	if !f.Saved() && !isRequester(by, f) {
		return dashboards.Dashboard{}, dashboards.Version{}, ErrRequesterOnly
	}
	if len(f.Evidence) == 0 {
		return dashboards.Dashboard{}, dashboards.Version{}, ErrNoEvidence
	}
	run, err := runOf(f)
	if err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, err
	}
	raw, err := json.Marshal(f.Spec())
	if err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, fmt.Errorf("encode evidence dashboard: %w", err)
	}
	made, version, err := s.dashboards.CreateFromAnswer(ctx, by, raw, savedDashboardSummary, run)
	if err != nil {
		return dashboards.Dashboard{}, dashboards.Version{}, err
	}
	if err := s.store.attachChat(ctx, by, f.AgentRequestID, made.ID); err != nil {
		s.logger.WarnContext(ctx, "attach the chat to the dashboard made from its answer", logattrs.Failure("chat_attach_failed", err, slog.String("finding_id", f.ID), slog.String("dashboard_id", made.ID))...)
	}
	return made, version, nil
}

func runOf(f Finding) (pgtype.UUID, error) {
	if f.AgentRequestID == "" {
		return pgtype.UUID{}, nil
	}
	return db.ParseUUID(f.AgentRequestID)
}

type Placement struct {
	Dashboard pgtype.UUID
	Folder    *folders.Folder
	Activity  activity.Owner
	Abilities folders.Abilities
}

func (s *Service) Access(ctx context.Context, by auth.Principal, id pgtype.UUID) (Placement, error) {
	f, err := s.store.get(ctx, id)
	if err != nil {
		return Placement{}, err
	}
	folder, abilities, err := s.place(ctx, by, f)
	if err != nil {
		return Placement{}, err
	}
	if !f.Saved() {
		return Placement{}, ErrNotSaved
	}
	observationID, err := db.ParseUUID(f.ObservationID)
	if err != nil {
		return Placement{}, err
	}
	var dashboardID pgtype.UUID
	if f.DashboardID != "" {
		if dashboardID, err = db.ParseUUID(f.DashboardID); err != nil {
			return Placement{}, err
		}
	}
	return Placement{Dashboard: dashboardID, Folder: &folder, Activity: activity.OnObservation(observationID), Abilities: abilities}, nil
}

func (f Finding) SavableBy(by auth.Principal) bool {
	return !f.Saved() && !f.observationHidden && isRequester(by, f)
}

func isRequester(by auth.Principal, f Finding) bool {
	return string(by.Kind) == f.RequestedBy.Kind && by.ID == f.RequestedBy.ID
}

func (s *Service) place(ctx context.Context, by auth.Principal, f Finding) (folders.Folder, folders.Abilities, error) {
	if !f.Saved() {
		if err := s.readableUnsaved(ctx, by, f); err != nil {
			return folders.Folder{}, folders.Abilities{}, err
		}
		return folders.Folder{}, folders.Abilities{View: true}, nil
	}
	observationID, err := db.ParseUUID(f.ObservationID)
	if err != nil {
		return folders.Folder{}, folders.Abilities{}, err
	}
	return s.observations.Placement(ctx, by, observationID)
}

func (s *Service) readPlace(ctx context.Context, by auth.Principal, f Finding) (Finding, folders.Folder, folders.Abilities, error) {
	folder, abilities, err := s.place(ctx, by, f)
	if errors.Is(err, ErrNotFound) && f.Saved() && s.readableInThread(ctx, by, f) == nil {
		f.ObservationID, f.FolderID, f.observationHidden = "", "", true
		return f, folders.Folder{}, folders.Abilities{View: true}, nil
	}
	return f, folder, abilities, err
}

func (s *Service) readableUnsaved(ctx context.Context, by auth.Principal, f Finding) error {
	if !isRequester(by, f) && !by.Role.AtLeast(auth.RoleAdmin) {
		return s.readableInThread(ctx, by, f)
	}
	if f.DashboardID == "" {
		return nil
	}
	dashboardID, err := db.ParseUUID(f.DashboardID)
	if err != nil {
		return err
	}
	return s.viewable(ctx, by, dashboardID)
}

func (s *Service) readableInThread(ctx context.Context, by auth.Principal, f Finding) error {
	if !f.SharedIn.Valid || s.threads == nil {
		return ErrNotFound
	}
	if err := s.threads(ctx, by, f.SharedIn); err != nil {
		return ErrNotFound
	}
	return nil
}

func (s *Service) viewable(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) error {
	_, err := s.dashboardAccess(ctx, by, dashboardID)
	return err
}

func (s *Service) dashboardAccess(ctx context.Context, by auth.Principal, dashboardID pgtype.UUID) (folders.DashboardAccess, error) {
	access, err := s.access.Dashboard(ctx, by, dashboardID)
	switch {
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, folders.ErrNoAccess), errors.Is(err, folders.ErrForbidden):
		return folders.DashboardAccess{}, ErrNotFound
	case err != nil:
		return folders.DashboardAccess{}, err
	case !access.Abilities.View:
		return folders.DashboardAccess{}, ErrNotFound
	}
	return access, nil
}
