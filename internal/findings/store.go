package findings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

type store struct {
	db *db.DB
}

type conclude func(q *dbsqlc.Queries) (id pgtype.UUID, created bool, err error)

func (s *store) insert(ctx context.Context, by actor.Actor, in SaveInput, conclude conclude) (Finding, bool, error) {
	cols, err := by.Columns()
	if err != nil {
		return Finding{}, false, err
	}
	evidence, err := json.Marshal(in.Evidence)
	if err != nil {
		return Finding{}, false, fmt.Errorf("encode evidence: %w", err)
	}
	cause := rootCauseColumns(in.Structured.RootCause)
	unverified, causes, err := encodeChecks(in)
	if err != nil {
		return Finding{}, false, err
	}
	var row dbsqlc.Finding
	insert := func(q *dbsqlc.Queries) error {
		observationID, created, err := conclude(q)
		if err != nil {
			return err
		}
		var id pgtype.UUID
		if created {
			id = observationID
		}
		row, err = q.InsertFinding(ctx, dbsqlc.InsertFindingParams{
			ID:             id,
			AgentRequestID: in.AgentRequestID,
			DashboardID:    in.DashboardID,
			AskDashboardID: in.AskDashboardID,
			ObservationID:  observationID,
			Title:          in.Title,
			Markdown:       in.Markdown,
			Evidence:       evidence,
			TimeFrom:       pgtype.Timestamptz{Time: in.From, Valid: true},
			TimeTo:         pgtype.Timestamptz{Time: in.To, Valid: true},
			CreatedByKind:  cols.Kind,
			CreatedByID:    cols.ID,
			CreatedByName:  cols.Name,
			Origin:         cols.Origin,
			OriginClient:   cols.Client,

			SuggestedChange: pgtype.Text{String: in.SuggestedChange, Valid: in.SuggestedChange != ""},
			ObserveWorthy:   in.ObserveWorthy,
			AsOf:            optionalTimestamp(in.AsOf),

			RootCauseComponent: cause.component,
			RootCauseKind:      cause.kind,
			RootCauseSummary:   cause.summary,
			Onset:              optionalTimestamp(in.Structured.Onset),
			Impact:             optionalText(in.Structured.Impact),
			Confidence:         optionalText(in.Structured.Confidence),
			RuledOut:           in.Structured.RuledOut,
			NextSteps:          in.Structured.NextSteps,
			NoIncident:         pgtype.Bool{Bool: true, Valid: in.Structured.NoIncident},
			Unverified:         unverified,
			Causes:             causes,
		})
		if err != nil {
			return err
		}
		return appendCreated(ctx, q, by, row)
	}
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := insert(q); err != nil {
			return err
		}
		return markRunCommitted(ctx, q, in)
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		existing, found, err := s.forRequest(ctx, in.AgentRequestID)
		if err == nil && !found {
			err = fmt.Errorf("read the finding saved first for agent request %s: %w", in.AgentRequestID.String(), ErrNotFound)
		}
		return existing, true, err
	case db.IsForeignKeyViolation(err):
		return Finding{}, false, ErrNotFound
	case err != nil:
		return Finding{}, false, fmt.Errorf("insert finding: %w", err)
	}
	f, err := s.get(ctx, row.ID)
	return f, false, err
}

func appendCreated(ctx context.Context, q *dbsqlc.Queries, by actor.Actor, row dbsqlc.Finding) error {
	if !row.ObservationID.Valid {
		return nil
	}
	data := map[string]string{"title": row.Title}
	if row.AgentRequestID.Valid {
		data["agent_request_id"] = row.AgentRequestID.String()
	}
	owner, err := createdOwner(ctx, q, row)
	if err != nil {
		return err
	}
	_, err = activity.Append(ctx, q, activity.Entry{
		Type:     activity.TypeFindingCreated,
		TargetID: row.ID,
		Owner:    owner,
		Actor:    by,
		Data:     data,
	}, nil)
	return err
}

func markRunCommitted(ctx context.Context, q *dbsqlc.Queries, in SaveInput) error {
	if !in.AgentRequestID.Valid || in.ClaimEpoch == 0 {
		return nil
	}
	n, err := q.MarkAgentRequestFindingCommitted(ctx, dbsqlc.MarkAgentRequestFindingCommittedParams{ID: in.AgentRequestID, ClaimEpoch: httpapi.Int32(in.ClaimEpoch)})
	if err != nil {
		return fmt.Errorf("mark the run's answer committed: %w", err)
	}
	if n == 0 {
		return ErrRunLost
	}
	return nil
}

func (s *store) runObservation(ctx context.Context, requestID pgtype.UUID) (pgtype.UUID, error) {
	id, err := s.db.Queries().RunObservation(ctx, requestID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, fmt.Errorf("read run observation: %w", err)
	}
	return id, nil
}

func (s *store) attachObservation(ctx context.Context, id pgtype.UUID, conclude conclude) (bool, error) {
	attached := false
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		existing, err := q.LockFinding(ctx, id)
		if err != nil || existing.Valid {
			return err
		}
		observationID, _, err := conclude(q)
		if err != nil {
			return err
		}
		if _, err := q.SetFindingObservation(ctx, dbsqlc.SetFindingObservationParams{ID: id, ObservationID: observationID}); err != nil {
			return err
		}
		attached = true
		return nil
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, ErrNotFound
	case err != nil:
		return false, fmt.Errorf("save finding as observation: %w", err)
	}
	return attached, nil
}

func (s *store) get(ctx context.Context, id pgtype.UUID) (Finding, error) {
	row, err := s.db.Queries().GetFinding(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Finding{}, ErrNotFound
	case err != nil:
		return Finding{}, fmt.Errorf("get finding: %w", err)
	}
	f, err := findingOf(row.Finding, row.ObservationFolderID, row.RequestTo, requester(row.RequestedByKind.String, row.RequestedByID))
	f.SharedIn, f.Restricted = row.RequestThreadMessageID, row.KeptWithAdmins
	return withAskOn(f, row.Finding.AskDashboardID, row.AskDashboardTitle), err
}

func (s *store) forRequest(ctx context.Context, requestID pgtype.UUID) (Finding, bool, error) {
	return s.forRequestIn(ctx, s.db.Queries(), requestID)
}

func (s *store) forRequestIn(ctx context.Context, q *dbsqlc.Queries, requestID pgtype.UUID) (Finding, bool, error) {
	row, err := q.GetFindingByAgentRequest(ctx, requestID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Finding{}, false, nil
	case err != nil:
		return Finding{}, false, fmt.Errorf("get finding of agent request: %w", err)
	}
	f, err := findingOf(row.Finding, row.ObservationFolderID, pgtype.Timestamptz{}, requester(row.RequestedByKind, row.RequestedByID))
	f.SharedIn, f.Restricted = row.RequestThreadMessageID, row.KeptWithAdmins
	return withAskOn(f, row.Finding.AskDashboardID, row.AskDashboardTitle), err == nil, err
}

func withAskOn(f Finding, id pgtype.UUID, title pgtype.Text) Finding {
	if id.Valid && title.Valid {
		f.AskOnID, f.AskOnTitle = id.String(), title.String
	}
	return f
}

func (s *store) listByDashboard(ctx context.Context, dashboardID pgtype.UUID, visible []pgtype.UUID, after *httpapi.TimeCursor, rows int) ([]Finding, error) {
	params := dbsqlc.ListFindingsByDashboardParams{DashboardID: dashboardID, Folders: visible, RowLimit: httpapi.Int32(rows)}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	found, err := s.db.Queries().ListFindingsByDashboard(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list findings of dashboard: %w", err)
	}
	return findingsOf(found, func(r dbsqlc.ListFindingsByDashboardRow) (dbsqlc.Finding, pgtype.UUID, bool) {
		return r.Finding, r.ObservationFolderID, r.KeptWithAdmins
	})
}

func (s *store) listInFolder(ctx context.Context, folderID pgtype.UUID, visible []pgtype.UUID, after *httpapi.TimeCursor, rows int) ([]Finding, error) {
	params := dbsqlc.ListFindingsInFolderParams{FolderID: folderID, Folders: visible, RowLimit: httpapi.Int32(rows)}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	found, err := s.db.Queries().ListFindingsInFolder(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list findings of folder: %w", err)
	}
	return findingsOf(found, func(r dbsqlc.ListFindingsInFolderRow) (dbsqlc.Finding, pgtype.UUID, bool) {
		return r.Finding, r.ObservationFolderID, r.KeptWithAdmins
	})
}

func (s *store) listBy(ctx context.Context, by auth.Principal, visible []pgtype.UUID, after *httpapi.TimeCursor, rows int) ([]Finding, error) {
	creator, err := db.ParseUUID(by.ID)
	if err != nil {
		return nil, fmt.Errorf("creator id: %w", err)
	}
	params := dbsqlc.ListFindingsByParams{CreatedByKind: string(by.Kind), CreatedByID: creator, Folders: visible, RowLimit: httpapi.Int32(rows)}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	found, err := s.db.Queries().ListFindingsBy(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list findings of creator: %w", err)
	}
	return findingsOf(found, func(r dbsqlc.ListFindingsByRow) (dbsqlc.Finding, pgtype.UUID, bool) {
		return r.Finding, r.ObservationFolderID, r.KeptWithAdmins
	})
}

func createdOwner(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.Finding) (activity.Owner, error) {
	kept, err := folders.RunAnswerKeptWithAdmins(ctx, q, row.AgentRequestID)
	if err != nil || !kept {
		return activityOwner(row.DashboardID, row.ObservationID), err
	}
	return activity.OnObservation(row.ObservationID), nil
}

func activityOwner(dashboardID, observationID pgtype.UUID) activity.Owner {
	if dashboardID.Valid {
		return activity.OnDashboard(dashboardID)
	}
	return activity.OnObservation(observationID)
}

func findingsOf[R any](rows []R, split func(R) (dbsqlc.Finding, pgtype.UUID, bool)) ([]Finding, error) {
	out := make([]Finding, len(rows))
	for i, r := range rows {
		row, folder, restricted := split(r)
		f, err := findingOf(row, folder, pgtype.Timestamptz{}, Author{})
		if err != nil {
			return nil, err
		}
		f.Restricted = restricted
		out[i] = f
	}
	return out, nil
}

func requester(kind string, id pgtype.UUID) Author {
	if !id.Valid {
		return Author{}
	}
	return Author{Kind: kind, ID: id.String()}
}

func findingOf(r dbsqlc.Finding, observationFolder pgtype.UUID, requestTo pgtype.Timestamptz, requestedBy Author) (Finding, error) {
	f := Finding{
		ID:          r.ID.String(),
		Title:       r.Title,
		Markdown:    r.Markdown,
		From:        r.TimeFrom.Time.UTC(),
		To:          r.TimeTo.Time.UTC(),
		AsOf:        timeOf(r.AsOf),
		CreatedAt:   r.CreatedAt.Time.UTC(),
		CreatedBy:   Author{Kind: r.CreatedByKind, ID: r.CreatedByID.String(), Name: r.CreatedByName},
		RequestedBy: requestedBy,

		SuggestedChange: r.SuggestedChange.String,
		ObserveWorthy:   r.ObserveWorthy,
		Structured:      StructuredOf(r),
		Unverified:      UnverifiedOf(r.Unverified),
		Actor: actor.FromColumns(actor.Columns{
			Kind: r.CreatedByKind, ID: r.CreatedByID, Name: r.CreatedByName,
			Origin: r.Origin, Client: r.OriginClient, RunID: r.AgentRequestID,
		}),
	}
	if err := json.Unmarshal(r.Evidence, &f.Evidence); err != nil {
		return Finding{}, fmt.Errorf("decode evidence of finding %s: %w", f.ID, err)
	}
	if r.ObservationID.Valid {
		f.ObservationID = r.ObservationID.String()
	}
	if r.AgentRequestID.Valid {
		f.AgentRequestID = r.AgentRequestID.String()
	}
	if r.DashboardID.Valid {
		f.DashboardID = r.DashboardID.String()
	} else if observationFolder.Valid {
		f.FolderID = observationFolder.String()
	}
	if r.AnnotationID.Valid {
		f.AnnotationID = r.AnnotationID.String()
	}
	if requestTo.Valid {
		t := requestTo.Time.UTC()
		f.RequestTo = &t
	}
	return f, nil
}

func (s *store) attachChat(ctx context.Context, by auth.Principal, runID, dashboardID string) error {
	run, err := db.ParseUUID(runID)
	if err != nil {
		return nil
	}
	dashboard, err := db.ParseUUID(dashboardID)
	if err != nil {
		return fmt.Errorf("read the made dashboard id: %w", err)
	}
	owner, err := db.ParseUUID(by.ID)
	if err != nil {
		return fmt.Errorf("read the caller id: %w", err)
	}
	err = s.db.Queries().AttachRunChatToDashboard(ctx, dbsqlc.AttachRunChatToDashboardParams{
		DashboardID: dashboard, RunID: run, OwnerKind: string(by.Kind), OwnerID: owner,
	})
	if err != nil {
		return fmt.Errorf("attach the chat to the dashboard made from its answer: %w", err)
	}
	return nil
}

func optionalTimestamp(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func timeOf(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	at := t.Time.UTC()
	return &at
}

func optionalText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func encodeChecks(in SaveInput) (unverified, causes []byte, err error) {
	if unverified, err = encodeUnverified(in.Unverified); err != nil {
		return nil, nil, err
	}
	causes, err = encodeCauses(in.Structured.Causes)
	return unverified, causes, err
}

type causeColumns struct{ component, kind, summary pgtype.Text }

func rootCauseColumns(c *RootCause) causeColumns {
	if c == nil {
		return causeColumns{}
	}
	return causeColumns{component: optionalText(c.Component), kind: optionalText(c.Kind), summary: optionalText(c.Summary)}
}

func StructuredOf(r dbsqlc.Finding) Structured {
	s := Structured{
		Onset:      timeOf(r.Onset),
		Impact:     r.Impact.String,
		Confidence: r.Confidence.String,
		RuledOut:   r.RuledOut,
		NextSteps:  r.NextSteps,
		NoIncident: r.NoIncident.Bool,
	}
	if r.RootCauseKind.Valid {
		s.RootCause = &RootCause{Component: r.RootCauseComponent.String, Kind: r.RootCauseKind.String, Summary: r.RootCauseSummary.String}
	}
	s.Causes = causesOf(r.Causes, s.RootCause, s.Onset, s.Confidence)
	return s
}
