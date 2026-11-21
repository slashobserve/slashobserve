package main

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/auditsource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestTheAuditDatasourceIsBuiltInForAdminsAndOffTheSlotGauges(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: datasource.NewUnavailable("prometheus", datasource.ErrUnavailable)})
	if err != nil {
		t.Fatal(err)
	}
	slots := queryproxy.NewSlots(map[string]int{"prom": 8}, nil)
	if err := addAuditDatasource(registry, slots, trail.NewReader(nil, logger), logger); err != nil {
		t.Fatal(err)
	}
	in, err := registry.For(datasource.WithAccess(t.Context(), datasource.Access{By: auth.Principal{Role: auth.RoleAdmin}}), auditsource.UID)
	if err != nil || in.Source != datasource.SourceBuiltin || in.Audience != datasource.AudienceAdmins || in.Datasource.Type() != "opensearch" {
		t.Fatalf("audit datasource = %+v, %v", in, err)
	}
	if _, err := registry.For(datasource.WithAccess(t.Context(), datasource.Access{By: auth.Principal{Role: auth.RoleEditor}}), auditsource.UID); err == nil {
		t.Fatal("a non-Admin reached the audit datasource")
	}
	for _, u := range slots.Usage() {
		if u.Datasource == auditsource.UID && u.Limit != auditsource.MaxConcurrentQueries {
			t.Errorf("audit slots = %+v", u)
		}
	}
	usage := slotUsage(slots)
	if len(usage) != 1 || usage[0].Datasource != "prom" {
		t.Fatalf("slot gauges = %+v, want only prom", usage)
	}
}

func TestAFileDatasourceOnTheAuditUIDStopsTheServer(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: auditsource.UID, Datasource: datasource.NewUnavailable("prometheus", datasource.ErrUnavailable)})
	if err != nil {
		t.Fatal(err)
	}
	err = addAuditDatasource(registry, queryproxy.NewSlots(nil, nil), trail.NewReader(nil, logger), logger)
	if err == nil || !strings.Contains(err.Error(), "datasources file already uses") {
		t.Fatalf("err = %v", err)
	}
}
