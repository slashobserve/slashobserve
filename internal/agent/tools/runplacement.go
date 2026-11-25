package tools

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/threads"
)

const (
	codePrivateRead = "private_read"

	privateReadNote = "Chats, runs and the inbox are read by their owner and Admins, so they are available in a private chat, not in a run whose answer other people read."
)

type readRule int

const (
	readsFreely readRule = iota
	readsAndMarks
	readsRefused
)

func privateReadRule(inRun bool, a datasource.Access, ok bool) readRule {
	switch {
	case !inRun || a.System:
		return readsFreely
	case !ok || a.Audience == datasource.AudienceEveryone:
		return readsRefused
	}
	return readsAndMarks
}

func auditReadRule(inRun bool, a datasource.Access, ok bool) readRule {
	switch {
	case !inRun || a.System:
		return readsFreely
	case !ok || !a.Reads(datasource.AudienceAdmins):
		return readsRefused
	}
	return readsAndMarks
}

func guardPrivateRead(ctx context.Context, ownRun string) (Result, bool) {
	run, inRun := RunFrom(ctx)
	if inRun && ownRun != "" && ownRun == run.RequestID {
		return Result{}, false
	}
	a, ok := datasource.AccessFrom(ctx)
	switch privateReadRule(inRun, a, ok) {
	case readsRefused:
		return toolError(codePrivateRead, privateReadNote), true
	case readsAndMarks:
		datasource.NoteRead(ctx, datasource.AudienceAdmins)
	}
	return Result{}, false
}

func guardAuditRead(ctx context.Context, tool string) (Result, bool) {
	_, inRun := RunFrom(ctx)
	a, ok := datasource.AccessFrom(ctx)
	switch auditReadRule(inRun, a, ok) {
	case readsRefused:
		if Withheld(a) {
			return toolError(CodeAdminsOnlyDatasource, datasource.WithheldNote), true
		}
		principal, _ := auth.FromContext(ctx)
		return toolError(codeForbiddenRole, roleRefusal(principal, tool)), true
	case readsAndMarks:
		datasource.NoteRead(ctx, datasource.AudienceAdmins)
	}
	return Result{}, false
}

func (r *Registry) readObservationContent(ctx context.Context, principal auth.Principal, id pgtype.UUID) error {
	if _, inRun := RunFrom(ctx); !inRun {
		return nil
	}
	placement, err := r.deps.Observations.Access(ctx, principal, id)
	if err != nil {
		return err
	}
	readContentIn(ctx, id.String(), folders.Folder{ID: placement.Folder, AdminsOnly: placement.AdminsOnly})
	return nil
}

func (r *Registry) readDashboardContent(ctx context.Context, principal auth.Principal, id pgtype.UUID) error {
	if _, inRun := RunFrom(ctx); !inRun {
		return nil
	}
	if r.deps.Folders == nil {
		return errFoldersNotConfigured
	}
	access, err := r.deps.Folders.Dashboard(ctx, principal, id)
	if err != nil {
		return err
	}
	readContentIn(ctx, id.String(), access.Folder)
	return nil
}

func (r *Registry) readCommentTarget(ctx context.Context, principal auth.Principal, target threads.Target) error {
	if _, inRun := RunFrom(ctx); !inRun {
		return nil
	}
	switch target.Kind {
	case threads.KindObservation:
		return r.readObservationContent(ctx, principal, target.ID)
	case threads.KindDashboard:
		return r.readDashboardContent(ctx, principal, target.ID)
	case threads.KindAnnotation:
		dashboardID, _, err := r.deps.Annotations.Access(ctx, principal, target.ID)
		if err != nil || !dashboardID.Valid {
			return err
		}
		return r.readDashboardContent(ctx, principal, dashboardID)
	case threads.KindProposal:
		dashboardID, _, err := r.deps.Proposals.Access(ctx, principal, target.ID)
		if err != nil {
			return err
		}
		return r.readDashboardContent(ctx, principal, dashboardID)
	}
	return nil
}
