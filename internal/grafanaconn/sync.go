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
	keptSyncs      = 20
	fetchWorkers   = 4
	busyFirstWait  = 100 * time.Millisecond
	busyMaxWait    = 2 * time.Second
	persistTimeout = 5 * time.Second

	StatusRunning  = "running"
	StatusFinished = "finished"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"

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
	ID         string     `json:"id"`
	Status     string     `json:"status"`
	Total      int        `json:"total"`
	Done       int        `json:"done"`
	Failed     int        `json:"failed"`
	Skipped    int        `json:"skipped"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
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

type run struct {
	mu    sync.Mutex
	state Sync
}

func (r *run) snapshot() Sync {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.state
	out.Results = slices.Clone(r.state.Results)
	return out
}

func (r *run) update(change func(*Sync)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	change(&r.state)
}

func (r *run) record(res SyncResult) {
	r.update(func(s *Sync) {
		s.Done++
		switch res.Outcome {
		case OutcomeFailed:
			s.Failed++
		case OutcomeSkipped:
			s.Skipped++
		}
		s.Results = append(s.Results, res)
	})
}

type syncs struct {
	mu      sync.Mutex
	kept    []*run
	running map[string]*run
}

func newSyncs() *syncs {
	return &syncs{running: map[string]*run{}}
}

func (s *syncs) start(connectionID string) (*run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.running[connectionID]; busy {
		return nil, false
	}
	r := &run{state: Sync{
		SyncSummary:  SyncSummary{ID: newID().String(), Status: StatusRunning, StartedAt: time.Now().UTC()},
		ConnectionID: connectionID,
		Results:      []SyncResult{},
	}}
	s.running[connectionID] = r
	s.kept = append(s.kept, r)
	if len(s.kept) > keptSyncs {
		s.kept = slices.Delete(s.kept, 0, len(s.kept)-keptSyncs)
	}
	return r, true
}

func (s *syncs) done(connectionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, connectionID)
}

func (s *syncs) find(connectionID, syncID string) (*run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.kept {
		r.mu.Lock()
		match := r.state.ID == syncID && r.state.ConnectionID == connectionID
		r.mu.Unlock()
		if match {
			return r, true
		}
	}
	return nil, false
}

func (s *Service) SyncStatus(connectionID pgtype.UUID, syncID string) (Sync, error) {
	r, ok := s.syncs.find(connectionID.String(), syncID)
	if !ok {
		return Sync{}, ErrSyncNotFound
	}
	return r.snapshot(), nil
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

func (s *Service) StartSync(ctx context.Context, by auth.Principal, id pgtype.UUID, req SyncRequest) (Sync, error) {
	if err := s.checkMapping(req.Mapping); err != nil {
		return Sync{}, err
	}
	_, client, err := s.client(ctx, id)
	if err != nil {
		return Sync{}, err
	}
	r, ok := s.syncs.start(id.String())
	if !ok {
		return Sync{}, ErrSyncRunning
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(s.life, cancel)
	s.wg.Go(func() {
		defer cancel()
		defer stop()
		defer s.syncs.done(id.String())
		s.run(runCtx, by, id, client, req, r)
	})
	return r.snapshot(), nil
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

func (s *Service) run(ctx context.Context, by auth.Principal, id pgtype.UUID, client grafanaClient, req SyncRequest, r *run) {
	p, err := s.plan(ctx, by, client, req)
	if err == nil {
		r.update(func(st *Sync) { st.Total = len(p.hits) })
		for f := range fetchAll(ctx, client, p.hits) {
			r.record(s.importOne(ctx, by, p, f))
		}
	}
	status := StatusFinished
	switch {
	case ctx.Err() != nil:
		status = StatusCanceled
	case err != nil:
		status = StatusFailed
	}
	finished := time.Now().UTC()
	r.update(func(st *Sync) {
		st.Status, st.FinishedAt = status, &finished
		if err != nil && status == StatusFailed {
			st.Error = describe(err)
		}
	})
	summary := r.snapshot().SyncSummary
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()
	if err := s.store.setSync(persistCtx, id, summary); err != nil {
		s.logger.ErrorContext(persistCtx, "record grafana sync", logattrs.Failure("internal", err, slog.String("connection_id", id.String()), slog.String("sync_id", summary.ID))...)
	}
	s.logger.InfoContext(persistCtx, "grafana sync finished",
		slog.String("connection_id", id.String()), slog.String("sync_id", summary.ID), slog.String("actor_id", by.ID),
		slog.String("status", status), slog.Int("total", summary.Total), slog.Int("failed", summary.Failed), slog.Int("skipped", summary.Skipped))
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
