package grafanaconn

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	keptSyncs     = 20
	fetchWorkers  = 4
	busyFirstWait = 100 * time.Millisecond
	busyMaxWait   = 2 * time.Second

	OutcomeImported = "imported"
	OutcomeUpdated  = "updated"
	OutcomeSkipped  = "skipped"
	OutcomeFailed   = "failed"
)

var (
	ErrSyncRunning  = errors.New("an import from this connection is already running")
	ErrSyncNotFound = errors.New("import run not found")
)

type SyncRequest struct {
	All     bool
	Folders []string
	Mapping map[string]string
}

type SyncSummary struct {
	ID         string
	State      SyncState
	Total      int
	Done       int
	Failed     int
	Skipped    int
	Error      string
	StartedAt  time.Time
	FinishedAt *time.Time
}

type SyncResult struct {
	UID               string
	Title             string
	Outcome           string
	DashboardID       string
	UnverifiedPanels  int
	PlaceholderPanels int
	Error             string
}

type Sync struct {
	SyncSummary
	ConnectionID string
	Results      []SyncResult
}

func (s *Service) checkMapping(mapping map[string]string) error {
	known := map[string]bool{}
	for _, d := range s.imports.Datasources() {
		known[d.UID] = true
	}
	for key, target := range mapping {
		if !known[target] {
			return &InvalidError{Message: "mapping for " + key + " names " + target + ", which is not a configured datasource"}
		}
	}
	return nil
}

type syncPlan struct {
	hits    []apiHit
	tree    folderTree
	mapping map[string]string
	general pgtype.UUID
}

func (s *Service) plan(ctx context.Context, by auth.Principal, client grafanaClient, req SyncRequest) (syncPlan, error) {
	p := syncPlan{mapping: map[string]string{}}
	if len(req.Mapping) > 0 {
		list, err := client.datasources(ctx)
		if err != nil {
			return syncPlan{}, err
		}
		p.mapping = expandMapping(req.Mapping, list)
	}
	tree, err := s.tree(ctx, client)
	if err != nil {
		return syncPlan{}, err
	}
	p.tree = tree
	hits, err := client.searchAll(ctx, "dash-db")
	if err != nil {
		return syncPlan{}, err
	}
	for _, h := range hits {
		if req.All || slices.ContainsFunc(req.Folders, func(chosen string) bool { return p.chose(h.FolderUID, chosen) }) {
			p.hits = append(p.hits, h)
		}
	}
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return syncPlan{}, err
	}
	for _, f := range levels.Folders() {
		if f.General {
			p.general = f.ID
		}
	}
	return p, nil
}

func (p syncPlan) chose(folderUID, chosen string) bool {
	if chosen == rootFolderUID {
		return folderUID == ""
	}
	return folderUID != "" && p.tree.within(folderUID, chosen)
}

func expandMapping(chosen map[string]string, list []apiDatasource) map[string]string {
	out := map[string]string{}
	for key, target := range chosen {
		out[key] = target
	}
	for _, d := range list {
		if target, ok := chosen[d.UID]; ok && d.Name != "" {
			if _, set := out[d.Name]; !set {
				out[d.Name] = target
			}
		}
		if target, ok := chosen[d.Name]; ok && d.UID != "" {
			if _, set := out[d.UID]; !set {
				out[d.UID] = target
			}
		}
	}
	return out
}

func (p syncPlan) target(req *imports.Request, folderUID string) {
	if folderUID == "" {
		req.FolderID = p.general
		return
	}
	title, err := folders.CleanTitle(p.tree.title(folderUID))
	if err != nil {
		req.FolderID = p.general
		return
	}
	req.Folder = title
}

type fetched struct {
	hit     apiHit
	raw     []byte
	version int
	err     error
}

func fetchAll(ctx context.Context, client grafanaClient, hits []apiHit) <-chan fetched {
	jobs := make(chan apiHit)
	out := make(chan fetched)
	var workers sync.WaitGroup
	for range fetchWorkers {
		workers.Go(func() {
			for h := range jobs {
				raw, version, err := client.dashboard(ctx, h.UID)
				select {
				case out <- fetched{hit: h, raw: raw, version: version, err: err}:
				case <-ctx.Done():
					return
				}
			}
		})
	}
	go func() {
		defer close(jobs)
		for _, h := range hits {
			select {
			case jobs <- h:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(out)
	}()
	return out
}

func (s *Service) importOne(ctx context.Context, by auth.Principal, p syncPlan, f fetched) SyncResult {
	res := SyncResult{UID: f.hit.UID, Title: f.hit.Title}
	fail := func(err error) SyncResult {
		res.Outcome, res.Error = OutcomeFailed, describe(err)
		if res.Error == internalMessage {
			s.logger.ErrorContext(ctx, "grafana sync dashboard", logattrs.Failure("internal", err, slog.String("grafana_uid", f.hit.UID))...)
		}
		return res
	}
	if f.err != nil {
		return fail(f.err)
	}
	existing, found, err := s.imports.DashboardByGrafanaUID(ctx, f.hit.UID)
	if err != nil {
		return fail(err)
	}
	if found && existing.GrafanaVersion >= f.version {
		res.Outcome, res.DashboardID = OutcomeSkipped, existing.DashboardID.String()
		return res
	}
	req := imports.Request{JSON: f.raw, GrafanaVersion: f.version, Mapping: s.imports.MappingFor(f.raw, p.mapping)}
	p.target(&req, f.hit.FolderUID)
	res.Outcome = OutcomeImported
	imported, err := whenFree(ctx, func() (imports.Imported, error) {
		if found {
			return s.imports.Reimport(ctx, by, existing.DashboardID, req)
		}
		return s.imports.Import(ctx, by, req)
	})
	if err != nil {
		return fail(err)
	}
	if found {
		res.Outcome = OutcomeUpdated
	}
	res.DashboardID = imported.Dashboard.ID
	res.UnverifiedPanels = imported.Record.Report.Counts.Unverified
	res.PlaceholderPanels = imported.Record.Report.Counts.Placeholder
	return res
}

func whenFree(ctx context.Context, do func() (imports.Imported, error)) (imports.Imported, error) {
	wait := busyFirstWait
	for {
		got, err := do()
		if !errors.Is(err, imports.ErrBusy) {
			return got, err
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return imports.Imported{}, ctx.Err()
		case <-timer.C:
		}
		wait = min(wait*2, busyMaxWait)
	}
}
