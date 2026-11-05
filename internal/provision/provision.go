package provision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	pollInterval    = 30 * time.Second
	maxMessageBytes = 2048
	summaryPrefix   = "Provisioned from "
	specFileExt     = ".json"
)

var (
	ErrOff           = errors.New("provisioning is off: SLASHOBSERVE_PROVISION_DIR is not set")
	ErrDirUnreadable = errors.New("the provisioning directory cannot be read")
)

var principal = auth.Principal{
	Kind: auth.KindServiceAccount,
	ID:   "6f1e0c52-7072-4f76-8973-696f6e696e67",
	Name: "Provisioning",
	Role: auth.RoleAdmin,
}

type Provisioner struct {
	logger     *slog.Logger
	dashboards *dashboards.Service
	folders    *folders.Access
	store      store
	dir        string

	mu      sync.Mutex
	logged  map[string]string
	gone    map[string]bool
	failing bool
}

func New(logger *slog.Logger, dash *dashboards.Service, access *folders.Access, database *db.DB, dir string) *Provisioner {
	return &Provisioner{
		logger:     logger,
		dashboards: dash,
		folders:    access,
		store:      store{db: database},
		dir:        dir,
		logged:     map[string]string{},
		gone:       map[string]bool{},
	}
}

type Report struct {
	Dir        string
	Dashboards []Provisioned
	Errors     []FileError
}

type Provisioned struct {
	Path        string
	DashboardID string
	Folder      string
	UpdatedAt   time.Time
	LastError   string
}

type FileError struct {
	Path    string
	Message string
	SeenAt  time.Time
}

type specFile struct {
	path   string
	folder string
}

func (p *Provisioner) Report(ctx context.Context) (Report, error) {
	rows, err := p.store.dashboards(ctx)
	if err != nil {
		return Report{}, err
	}
	failed, err := p.store.errors(ctx)
	if err != nil {
		return Report{}, err
	}
	out := Report{Dir: p.dir, Dashboards: make([]Provisioned, 0, len(rows)), Errors: make([]FileError, 0, len(failed))}
	for _, r := range rows {
		out.Dashboards = append(out.Dashboards, Provisioned{Path: r.Path, DashboardID: r.DashboardID.String(), Folder: r.FolderTitle, UpdatedAt: r.UpdatedAt.Time, LastError: r.LastError})
	}
	for _, e := range failed {
		out.Errors = append(out.Errors, FileError{Path: e.Path, Message: e.Message, SeenAt: e.SeenAt.Time})
	}
	return out, nil
}

func (p *Provisioner) Run(ctx context.Context) error {
	if p.dir == "" {
		return ErrOff
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	files, err := scan(p.dir)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDirUnreadable, err)
	}
	rows, err := p.store.dashboards(ctx)
	if err != nil {
		return err
	}
	known := make(map[string]dbsqlc.ProvisionedDashboard, len(rows))
	for _, r := range rows {
		known[r.Path] = r
	}
	ctx = origin.With(ctx, origin.Origin{Kind: origin.API})
	present := make(map[string]bool, len(files))
	failing := false
	for _, f := range files {
		present[f.path] = true
		delete(p.gone, f.path)
		row, found := known[f.path]
		failed, err := p.provisionFile(ctx, f, row, found)
		if err != nil {
			return err
		}
		failing = failing || failed
	}
	if err := p.clearRemovedErrors(ctx, present); err != nil {
		return err
	}
	for _, r := range rows {
		if !present[r.Path] && !p.gone[r.Path] {
			p.gone[r.Path] = true
			p.logger.InfoContext(ctx, "a provisioned file is gone; its dashboard stays", slog.String("path", r.Path), slog.String("dashboard_id", r.DashboardID.String()))
		}
	}
	p.failing = failing
	return nil
}

func (p *Provisioner) Watch(ctx context.Context) {
	if p.dir == "" {
		return
	}
	p.logger.InfoContext(ctx, "dashboard provisioning configured", slog.String("dir", p.dir))
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var seen string
	unreadable := false
	for {
		stamp, err := fingerprint(p.dir)
		switch {
		case err != nil:
			if !unreadable {
				p.logger.ErrorContext(ctx, "the provisioning directory cannot be read", logattrs.Failure("provision_dir_unreadable", err, slog.String("dir", p.dir))...)
			}
			unreadable = true
		case stamp != seen || p.retrying():
			unreadable = false
			if err := p.Run(ctx); err != nil {
				if ctx.Err() == nil {
					p.logger.ErrorContext(ctx, "provisioning run failed", logattrs.Failure("provision_failed", err, slog.String("dir", p.dir))...)
				}
			} else {
				seen = stamp
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Provisioner) retrying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.failing
}

func (p *Provisioner) provisionFile(ctx context.Context, f specFile, row dbsqlc.ProvisionedDashboard, found bool) (bool, error) {
	raw, err := os.ReadFile(filepath.Join(p.dir, filepath.FromSlash(f.path)))
	if err != nil {
		return p.fail(ctx, f, found, err)
	}
	digest := sha256.Sum256(raw)
	sum := hex.EncodeToString(digest[:])
	if found && row.ContentSha256 == sum {
		if row.LastError == "" {
			return false, nil
		}
		delete(p.logged, f.path)
		return false, p.store.dashboardFailed(ctx, f.path, "")
	}
	summary := summaryPrefix + f.path
	if found {
		latest, err := p.dashboards.LatestVersion(ctx, row.DashboardID)
		if err != nil {
			return p.fail(ctx, f, found, err)
		}
		_, ver, _, err := p.dashboards.AddVersion(ctx, principal, row.DashboardID, latest, raw, summary)
		if err != nil {
			return p.fail(ctx, f, found, err)
		}
		p.saved(ctx, f, row.DashboardID.String(), ver.Number)
		return false, p.store.saved(ctx, f.path, row.DashboardID, sum, row.FolderTitle)
	}
	target, err := p.folders.ForImport(ctx, principal, f.folder)
	if err != nil {
		return p.fail(ctx, f, found, err)
	}
	dash, ver, err := p.dashboards.Create(ctx, principal, target.Folder.ID, raw, summary)
	if err != nil {
		return p.fail(ctx, f, found, err)
	}
	id, err := db.ParseUUID(dash.ID)
	if err != nil {
		return false, err
	}
	p.saved(ctx, f, dash.ID, ver.Number)
	return false, p.store.saved(ctx, f.path, id, sum, target.Folder.Title)
}

func (p *Provisioner) saved(ctx context.Context, f specFile, dashboardID string, version int) {
	delete(p.logged, f.path)
	p.logger.InfoContext(ctx, "provisioned a dashboard", slog.String("path", f.path), slog.String("dashboard_id", dashboardID), slog.Int("version", version))
}

func (p *Provisioner) fail(ctx context.Context, f specFile, found bool, cause error) (bool, error) {
	message, code := describe(cause)
	if p.logged[f.path] != message {
		p.logged[f.path] = message
		p.logger.WarnContext(ctx, "provisioning a file failed", logattrs.Failure(code, cause, slog.String("path", f.path))...)
	}
	if found {
		return true, p.store.dashboardFailed(ctx, f.path, message)
	}
	return true, p.store.fileFailed(ctx, f.path, message)
}

func (p *Provisioner) clearRemovedErrors(ctx context.Context, present map[string]bool) error {
	failed, err := p.store.errors(ctx)
	if err != nil {
		return err
	}
	for _, e := range failed {
		if present[e.Path] {
			continue
		}
		delete(p.logged, e.Path)
		if err := p.store.clearError(ctx, e.Path); err != nil {
			return err
		}
	}
	return nil
}

func describe(err error) (message, code string) {
	var invalid *dashboards.InvalidSpecError
	var conflict *dashboards.ConflictError
	var pathErr *fs.PathError
	switch {
	case errors.As(err, &invalid):
		return issuesMessage(invalid.Issues), "invalid_spec"
	case errors.As(err, &conflict):
		return "the dashboard got a newer version while the file was saved; the next run saves it again", "version_conflict"
	case errors.As(err, &pathErr):
		return "the file cannot be read", "file_unreadable"
	case errors.Is(err, folders.ErrForbidden), errors.Is(err, folders.ErrNoAccess), errors.Is(err, dashboards.ErrFolderNotFound), errors.Is(err, dashboards.ErrNotFound):
		return "the provisioning account cannot save into this folder or dashboard", "forbidden"
	}
	return "the save failed; the server log has the cause", "internal"
}

func issuesMessage(issues []spec.Issue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Path+": "+issue.Message)
	}
	return truncate("the spec is invalid: "+strings.Join(parts, "; "), maxMessageBytes)
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func scan(dir string) ([]specFile, error) {
	var files []specFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != dir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), specFileExt) {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		folder, _, nested := strings.Cut(rel, "/")
		if !nested {
			folder = ""
		}
		files = append(files, specFile{path: rel, folder: folder})
		return nil
	})
	return files, err
}

func fingerprint(dir string) (string, error) {
	files, err := scan(dir)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, f := range files {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f.path)))
		if err != nil {
			_, _ = fmt.Fprintf(h, "%s\x00missing\n", f.path)
			continue
		}
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\n", f.path, info.Size(), info.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
