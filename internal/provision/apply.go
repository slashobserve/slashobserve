package provision

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/specfiles"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	OutcomeCreated   = "created"
	OutcomeUpdated   = "updated"
	OutcomeUnchanged = "unchanged"
	OutcomeFailed    = "failed"

	CodeAmbiguous       = "ambiguous"
	CodeProvisioned     = "provisioned"
	CodeInvalidSpec     = "invalid_spec"
	CodeVersionConflict = "version_conflict"
	CodeNotFound        = "not_found"

	applySummaryPrefix = "Applied from "
	maxPathBytes       = 512
	maxPathSegments    = 2
	generalFolderTitle = "General"
)

var ErrInvalidPath = errors.New("path must be a relative .json file path, either a file name or one folder directory and a file name, with no empty, . or .. segments")

type Applier struct {
	db         *db.DB
	dashboards *dashboards.Service
	folders    *folders.Access
}

func NewApplier(database *db.DB, dash *dashboards.Service, access *folders.Access) *Applier {
	return &Applier{db: database, dashboards: dash, folders: access}
}

type Result struct {
	Path        string
	Outcome     string
	DashboardID string
	Version     int
	Code        string
	Message     string
	Issues      []spec.Issue
}

type Binding struct {
	Path        string
	DashboardID string
}

func CheckPath(path string) error {
	if path == "" || len(path) > maxPathBytes || !utf8.ValidString(path) || strings.ContainsAny(path, "\\\x00") || !strings.HasSuffix(path, specfiles.Ext) {
		return ErrInvalidPath
	}
	segments := strings.Split(path, "/")
	if len(segments) > maxPathSegments {
		return ErrInvalidPath
	}
	for _, s := range segments {
		if s == "" || strings.HasPrefix(s, ".") {
			return ErrInvalidPath
		}
	}
	if segments[len(segments)-1] == specfiles.Ext {
		return ErrInvalidPath
	}
	return nil
}

type adoption int

const (
	adoptNone adoption = iota
	adoptOne
	adoptAmbiguous
)

func adopt(candidates []pgtype.UUID) (pgtype.UUID, adoption) {
	switch len(candidates) {
	case 0:
		return pgtype.UUID{}, adoptNone
	case 1:
		return candidates[0], adoptOne
	}
	return pgtype.UUID{}, adoptAmbiguous
}

func folderTitleOf(path string) string {
	if title := specfiles.FolderOf(path); title != "" {
		return title
	}
	return generalFolderTitle
}

func (a *Applier) Apply(ctx context.Context, by auth.Principal, path string, raw json.RawMessage) (Result, error) {
	if err := CheckPath(path); err != nil {
		return Result{}, err
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return Result{Path: path, Outcome: OutcomeFailed, Code: CodeInvalidSpec, Message: "the file is not JSON"}, nil
	}
	digest := sha256.Sum256(compact.Bytes())
	sum := hex.EncodeToString(digest[:])
	res := Result{Path: path}

	bound, err := a.db.Queries().GetAppliedDashboard(ctx, path)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, fmt.Errorf("read applied dashboard: %w", err)
	}
	target := bound.DashboardID
	var folderID pgtype.UUID
	if !found {
		folderID, err = a.existingFolder(ctx, path)
		if err != nil {
			return Result{}, err
		}
		if folderID.Valid {
			candidates, err := a.db.Queries().ApplyCandidates(ctx, dbsqlc.ApplyCandidatesParams{FolderID: folderID, Title: specTitle(raw)})
			if err != nil {
				return Result{}, fmt.Errorf("find dashboards to adopt: %w", err)
			}
			id, choice := adopt(candidates)
			if choice == adoptAmbiguous {
				return refused(res, CodeAmbiguous, "more than one dashboard in the folder has this title; rename one or apply the file under the path it was applied from"), nil
			}
			target = id
		}
	}
	if !target.Valid {
		return a.create(ctx, by, res, folderID, raw, sum)
	}
	return a.update(ctx, by, res, target, found && bound.ContentSha256 == sum, raw, sum)
}

func (a *Applier) existingFolder(ctx context.Context, path string) (pgtype.UUID, error) {
	title, err := folders.CleanTitle(folderTitleOf(path))
	if err != nil {
		title = generalFolderTitle
	}
	id, err := a.db.Queries().FolderByTitle(ctx, title)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, nil
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("find folder: %w", err)
	}
	return id, nil
}

func (a *Applier) create(ctx context.Context, by auth.Principal, res Result, folderID pgtype.UUID, raw []byte, sum string) (Result, error) {
	if !folderID.Valid {
		target, err := a.folders.ForImport(ctx, by, folderTitleOf(res.Path))
		if err != nil {
			return failure(res, err)
		}
		folderID = target.Folder.ID
	}
	dash, ver, err := a.dashboards.Create(ctx, by, folderID, raw, applySummaryPrefix+res.Path)
	if err != nil {
		return failure(res, err)
	}
	id, err := db.ParseUUID(dash.ID)
	if err != nil {
		return Result{}, err
	}
	if err := a.bind(ctx, by, res.Path, id, sum); err != nil {
		return Result{}, err
	}
	res.Outcome, res.DashboardID, res.Version = OutcomeCreated, dash.ID, ver.Number
	return res, nil
}

func (a *Applier) update(ctx context.Context, by auth.Principal, res Result, id pgtype.UUID, sameFile bool, raw []byte, sum string) (Result, error) {
	access, err := a.folders.Dashboard(ctx, by, id)
	if err != nil {
		return failure(res, err)
	}
	if !access.Abilities.Edit {
		return failure(res, folders.ErrForbidden)
	}
	provisioned, err := a.db.Queries().IsProvisionedDashboard(ctx, id)
	if err != nil {
		return Result{}, fmt.Errorf("check provisioned dashboard: %w", err)
	}
	if provisioned {
		return refused(res, CodeProvisioned, "the dashboard comes from a file in the server's provisioning directory; change that file instead"), nil
	}
	latest, err := a.dashboards.LatestVersion(ctx, id)
	if err != nil {
		return failure(res, err)
	}
	res.DashboardID = id.String()
	unchanged := sameFile
	if !unchanged {
		unchanged, err = a.sameAsVersion(ctx, id, latest, raw)
		if err != nil {
			return failure(res, err)
		}
	}
	if unchanged {
		if err := a.bind(ctx, by, res.Path, id, sum); err != nil {
			return Result{}, err
		}
		res.Outcome, res.Version = OutcomeUnchanged, latest
		return res, nil
	}
	_, ver, _, err := a.dashboards.AddVersion(ctx, by, id, latest, raw, applySummaryPrefix+res.Path)
	if err != nil {
		return failure(res, err)
	}
	if err := a.bind(ctx, by, res.Path, id, sum); err != nil {
		return Result{}, err
	}
	res.Outcome, res.Version = OutcomeUpdated, ver.Number
	return res, nil
}

func (a *Applier) sameAsVersion(ctx context.Context, id pgtype.UUID, version int, raw []byte) (bool, error) {
	parsed, err := spec.Parse(raw)
	if err != nil {
		return false, nil
	}
	stored, err := a.dashboards.VersionSpec(ctx, id, version)
	if err != nil {
		return false, err
	}
	next, err := json.Marshal(parsed)
	if err != nil {
		return false, err
	}
	prev, err := json.Marshal(stored)
	if err != nil {
		return false, err
	}
	return bytes.Equal(next, prev), nil
}

func (a *Applier) bind(ctx context.Context, by auth.Principal, path string, id pgtype.UUID, sum string) error {
	appliedBy, err := db.ParseUUID(by.ID)
	if err != nil {
		return fmt.Errorf("applier id: %w", err)
	}
	err = a.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.DeleteAppliedDashboardElsewhere(ctx, dbsqlc.DeleteAppliedDashboardElsewhereParams{DashboardID: id, Path: path}); err != nil {
			return err
		}
		return q.UpsertAppliedDashboard(ctx, dbsqlc.UpsertAppliedDashboardParams{Path: path, DashboardID: id, ContentSha256: sum, AppliedByID: appliedBy})
	})
	if db.IsForeignKeyViolation(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("bind applied dashboard: %w", err)
	}
	return nil
}

func (a *Applier) Applied(ctx context.Context, by auth.Principal, folderID pgtype.UUID) ([]Binding, error) {
	levels, err := a.folders.Levels(ctx, by)
	if err != nil {
		return nil, err
	}
	if !levels.Of(folderID).View {
		return nil, folders.ErrNotFound
	}
	rows, err := a.db.Queries().ListAppliedInFolder(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("list applied dashboards: %w", err)
	}
	out := make([]Binding, 0, len(rows))
	for _, r := range rows {
		out = append(out, Binding{Path: r.Path, DashboardID: r.DashboardID.String()})
	}
	return out, nil
}

func specTitle(raw []byte) string {
	var head struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(raw, &head)
	return head.Title
}

func refused(res Result, code, message string) Result {
	res.Outcome, res.Code, res.Message = OutcomeFailed, code, message
	return res
}

func failure(res Result, err error) (Result, error) {
	var invalid *dashboards.InvalidSpecError
	var conflict *dashboards.ConflictError
	switch {
	case errors.As(err, &invalid):
		res = refused(res, CodeInvalidSpec, issuesMessage(invalid.Issues))
		res.Issues = invalid.Issues
		return res, nil
	case errors.As(err, &conflict):
		return refused(res, CodeVersionConflict, "the dashboard got a newer version while the file was applied; apply it again"), nil
	case errors.Is(err, folders.ErrNotFound), errors.Is(err, dashboards.ErrNotFound), errors.Is(err, dashboards.ErrFolderNotFound):
		return refused(res, CodeNotFound, "the dashboard or its folder no longer exists"), nil
	case errors.Is(err, folders.ErrForbidden), errors.Is(err, folders.ErrNoAccess):
		return refused(res, forbidden.Code, "you cannot save into this folder or dashboard"), nil
	}
	return Result{}, err
}
