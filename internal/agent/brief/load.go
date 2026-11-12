package brief

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxLoggedIssues = 20

var (
	ErrInvalidTask  = errors.New("invalid agent task")
	ErrDraftInvalid = errors.New("the draft panel is not valid")
)

type LoadError struct {
	Message string
	Code    revisions.FailureCode
	Err     error
}

func (e *LoadError) Error() string { return e.Message }
func (e *LoadError) Unwrap() error { return e.Err }

type DashboardReader interface {
	Get(ctx context.Context, id pgtype.UUID) (dashboards.Dashboard, dashboards.Version, error)
	GetVersion(ctx context.Context, id pgtype.UUID, version int) (dashboards.Dashboard, dashboards.Version, error)
}

type ImportReader interface {
	ForDashboard(ctx context.Context, id pgtype.UUID) (imports.Record, error)
}

type Loader struct {
	Dashboards DashboardReader
	Imports    ImportReader
	Logger     *slog.Logger
}

func (l Loader) Latest(ctx context.Context, id pgtype.UUID, requestID string) (*Revision, error) {
	dash, ver, err := l.Dashboards.Get(ctx, id)
	rev, err := l.loaded(ctx, dash, ver, err, requestID, "latest version has no spec")
	if err != nil {
		return nil, err
	}
	rev.Conversion = l.conversionNotes(ctx, id, rev.Number, requestID)
	return rev, nil
}

func (l Loader) Base(ctx context.Context, id pgtype.UUID, version int, panelID string, draft json.RawMessage, requestID string) (*Revision, error) {
	dash, ver, err := l.Dashboards.GetVersion(ctx, id, version)
	rev, err := l.loaded(ctx, dash, ver, err, requestID, "base version has no spec")
	if err != nil {
		return nil, err
	}
	rev.Conversion = l.conversionNotes(ctx, id, rev.Number, requestID)
	if len(draft) == 0 {
		if _, ok := rev.Spec.FindPanel(panelID); !ok {
			return nil, fmt.Errorf("%w: the panel is not in that version of the dashboard", ErrInvalidTask)
		}
	} else if rev.Spec, err = spec.WithPanel(rev.Spec, panelID, draft); err != nil {
		return nil, l.draftInvalid(ctx, requestID, err)
	}
	entry, ok := rev.Spec.FindPanel(panelID)
	switch {
	case !ok:
		return nil, l.draftInvalid(ctx, requestID, ErrDraftInvalid)
	case !entry.Panel.Type.IsEditable():
		return nil, fmt.Errorf("%w: the panel is not a data, text or dashboard list panel", ErrInvalidTask)
	}
	return rev, nil
}

func (l Loader) conversionNotes(ctx context.Context, id pgtype.UUID, number int, requestID string) []tools.Attention {
	if l.Imports == nil {
		return nil
	}
	record, err := l.Imports.ForDashboard(ctx, id)
	switch {
	case errors.Is(err, imports.ErrNotFound), errors.Is(err, dashboards.ErrNotFound):
		return nil
	case err != nil:
		if ctx.Err() == nil {
			l.Logger.WarnContext(ctx, "agent could not read the import record", logattrs.Failure("import_read_failed", err, slog.String("agent_request_id", requestID))...)
		}
		return nil
	case record.Version != number:
		return nil
	}
	return tools.ConversionAttention(record.Report)
}

func (l Loader) draftInvalid(ctx context.Context, requestID string, err error) error {
	attrs := logattrs.Failure("draft_invalid", err, slog.String("agent_request_id", requestID))
	var invalid *spec.ValidationError
	if errors.As(err, &invalid) {
		issues := invalid.Issues[:min(len(invalid.Issues), maxLoggedIssues)]
		codes := make([]string, len(issues))
		paths := make([]string, len(issues))
		for i, issue := range issues {
			codes[i], paths[i] = string(issue.Code), issue.Path
		}
		attrs = append(attrs, slog.Int("issue_count", len(invalid.Issues)), slog.Any("issue_codes", codes), slog.Any("issue_paths", paths))
	}
	l.Logger.WarnContext(ctx, "agent draft panel is not valid", attrs...)
	return ErrDraftInvalid
}

func (l Loader) loaded(ctx context.Context, dash dashboards.Dashboard, ver dashboards.Version, err error, requestID, noSpec string) (*Revision, error) {
	switch {
	case errors.Is(err, dashboards.ErrNotFound):
		return nil, dashboards.ErrNotFound
	case err != nil:
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		l.Logger.ErrorContext(ctx, "agent could not load the dashboard", append([]any{
			slog.String("error_code", string(revisions.FailureLoadFailed)),
			slog.String("agent_request_id", requestID),
		}, logattrs.Safe(err)...)...)
		return nil, &LoadError{Message: "could not load the dashboard", Code: revisions.FailureLoadFailed, Err: err}
	case ver.Spec == nil:
		return nil, &LoadError{Message: "could not load the dashboard", Code: revisions.FailureLoadFailed, Err: errors.New(noSpec)}
	}
	return &Revision{DashboardID: dash.ID, Number: ver.Number, Spec: ver.Spec, Unverified: ver.Unverified}, nil
}
