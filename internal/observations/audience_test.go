package observations_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type audienceFreezer struct {
	mu         sync.Mutex
	restricted []bool
}

func (f *audienceFreezer) Freeze(ctx context.Context, _ auth.Principal, _ *spec.Dashboard, _ string, _, _ time.Time) (observations.Frozen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restricted = append(f.restricted, datasource.ReadsRestricted(ctx))
	return observations.Frozen{Data: json.RawMessage(frozenData)}, nil
}

func (f *audienceFreezer) reads() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bool(nil), f.restricted...)
}

func newAudienceRig(t *testing.T) (rig, *audienceFreezer) {
	t.Helper()
	r := newRig(t)
	f := &audienceFreezer{}
	r.service = observations.New(r.db.DB, folders.NewAccess(r.db.DB), f, nil, slog.New(slog.DiscardHandler))
	return r, f
}

func asCaller(ctx context.Context, by auth.Principal) context.Context {
	return datasource.WithAccess(ctx, datasource.Access{By: by})
}

func TestAnAdminFreezesASharedObservationAsEveryoneDoes(t *testing.T) {
	r, f := newAudienceRig(t)
	ctx := asCaller(t.Context(), admin)
	for _, c := range []struct {
		name   string
		folder func() observations.Observation
		reads  bool
	}{
		{"shared", func() observations.Observation { return r.create(t, admin, r.shared) }, false},
		{"admins only", func() observations.Observation { return r.create(t, admin, foldertest.AdminsOnly(t, r.db, "Payroll")) }, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			o := c.folder()
			panel := r.addPanel(t, admin, o)
			before := len(f.reads())
			if _, err := r.service.AddBlock(ctx, admin, mustUUID(t, o.ID), observations.BlockInput{Kind: observations.KindSnapshot, FromBlockID: mustUUID(t, panel.ID)}); err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			if _, err := r.service.Publish(ctx, admin, mustUUID(t, o.ID)); err != nil {
				t.Fatalf("publish: %v", err)
			}
			got := f.reads()[before:]
			if len(got) != 2 || got[0] != c.reads || got[1] != c.reads {
				t.Fatalf("the snapshot and the RCA froze with restricted reads %v, want %v", got, c.reads)
			}
		})
	}
}
