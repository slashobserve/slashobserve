package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/health"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/proposals"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
	"github.com/slashobserve/slashobserve/internal/search"
	"github.com/slashobserve/slashobserve/internal/threads"
)

func TestDeclaredRequiredFieldsMatchWhatHandlersReject(t *testing.T) {
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	ds := &fakeDS{
		rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
			return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
		},
		instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
		},
	}
	datasources, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(datasources, logger)
	access := folders.NewAccess(database.DB)
	service := dashboards.New(database.DB, runner, access, logger)
	notes := annotations.New(database.DB, access)
	observationService := observations.New(database.DB, access, nil, nil, logger)
	found, err := findings.New(database.DB, access, notes, service, observationService, logger)
	if err != nil {
		t.Fatal(err)
	}
	commentThreads := threads.NewService(database.DB, access, service, found, notes, observationService, proposals.New(database.DB, access, service, logger))
	registry := New(Deps{
		Datasources:   datasources,
		Runner:        runner,
		Dashboards:    service,
		Threads:       commentThreads,
		Imports:       imports.New(database.DB, service, access, datasources, logger),
		Folders:       access,
		Findings:      found,
		Observations:  observationService,
		Proposals:     proposals.New(database.DB, access, service, logger),
		SavedSearches: savedsearches.New(database.DB, access, datasources, nil, logger),
		Health:        health.New(health.Deps{DB: database.DB, Runner: runner, Access: access, Logger: logger, Job: "dashboard_health"}),
		Annotations:   notes,
		Search:        search.New(database.DB, access, nil),
		Provider:      &scriptedDraft{},
		Runs:          &fakeRuns{raw: json.RawMessage(`{}`)},
		Logger:        logger,
	})
	ctx := auth.WithPrincipal(context.Background(), placeholderUser)

	validSpec := specOf(statPanel("p1", 0))
	dash, _, err := service.Create(ctx, placeholderUser, pgtype.UUID{}, []byte(validSpec), "seed")
	if err != nil {
		t.Fatal(err)
	}
	dashboardID, err := db.ParseUUID(dash.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, comment, err := commentThreads.Open(ctx, placeholderUser, threads.OpenInput{Target: threads.Target{Kind: threads.KindDashboard, ID: dashboardID}, Body: "seed"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	samples := map[string]json.RawMessage{
		"id":             json.RawMessage(`"` + runID + `"`),
		"datasource":     json.RawMessage(`"` + fakeUID + `"`),
		"datasource_uid": json.RawMessage(`"` + fakeUID + `"`),
		"kind":           json.RawMessage(`"logs"`),
		"title":          json.RawMessage(`"Checkout errors"`),
		"question":       json.RawMessage(`"error logs from checkout"`),
		"query":          json.RawMessage(`"up"`),
		"q":              json.RawMessage(`"checkout"`),
		"table":          json.RawMessage(`"t"`),
		"label":          json.RawMessage(`"job"`),
		"dashboard_id":   json.RawMessage(`"` + dash.ID + `"`),
		"target":         json.RawMessage(`"dashboard:` + dash.ID + `"`),
		"comment_id":     json.RawMessage(`"` + comment.ID + `"`),
		"finding_id":     json.RawMessage(`"` + dash.ID + `"`),
		"observation_id": json.RawMessage(`"` + dash.ID + `"`),
		"block_id":       json.RawMessage(`"` + dash.ID + `"`),
		"proposal_id":    json.RawMessage(`"` + dash.ID + `"`),
		"revision":       json.RawMessage(`1`),
		"version":        json.RawMessage(`1`),
		"folder_id":      json.RawMessage(`"` + dash.ID + `"`),
		"parent_version": json.RawMessage(`1`),
		"run_id":         json.RawMessage(`"` + runID + `"`),
		"attempt":        json.RawMessage(`1`),
		"outcome":        json.RawMessage(`"failed"`),
		"spec":           json.RawMessage(validSpec),
		"body":           json.RawMessage(`"done"`),
		"ids":            json.RawMessage(`["p1"]`),
		"summary":        json.RawMessage(`"Change a panel"`),
		"markdown":       json.RawMessage(`"checkout errors are up"`),
		"text":           json.RawMessage(`"deployed checkout v2"`),
		"trace_id":       json.RawMessage(`"0123456789abcdef0123456789abcdef"`),
		"selected":       json.RawMessage(`["0123456789abcdef0123456789abcdef"]`),
		"baseline":       json.RawMessage(`["fedcba9876543210fedcba9876543210"]`),
		"anchor":         json.RawMessage(`"1700000000000000000"`),
		"json":           json.RawMessage(`"{\"title\":\"Imported\",\"schemaVersion\":39,\"panels\":[]}"`),
	}

	requiredOnlyForSomeDatasourceTypes := map[string][]string{nameSampleRows: {"table"}, nameListComments: {"target", "dashboard_id"}, nameSaveSearch: {"query"}, nameAddBlock: {"panel"}, nameUpdateBlock: {"markdown", "panel", "width", "position"}}
	for _, tool := range registry.List() {
		t.Run(tool.Name, func(t *testing.T) {
			var schema struct {
				Required   []string                   `json:"required"`
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				t.Fatal(err)
			}
			base := map[string]json.RawMessage{}
			for _, field := range schema.Required {
				sample, ok := samples[field]
				if !ok {
					t.Fatalf("no sample value for required field %q; add one to samples", field)
				}
				base[field] = sample
			}
			ctx := ctx
			if tool.AgentOnly {
				ctx = WithRun(ctx, Run{Mode: ModeInvestigate})
			}
			baseRes := callWith(ctx, t, registry, tool.Name, base)
			baseBody := contentJSON(t, baseRes)
			if baseRes.IsError {
				var body struct{ Code, Message string }
				_ = json.Unmarshal(baseBody, &body)
				for field := range schema.Properties {
					if !slices.Contains(schema.Required, field) && !slices.Contains(requiredOnlyForSomeDatasourceTypes[tool.Name], field) && body.Code == codeInvalidRequest && mentions(body.Message, field) {
						t.Errorf("a call with exactly the declared required fields %v is rejected for %q, which the schema does not require: %s", schema.Required, field, body.Message)
					}
				}
			}
			for _, field := range schema.Required {
				without := map[string]json.RawMessage{}
				for name, value := range base {
					if name != field {
						without[name] = value
					}
				}
				res := callWith(ctx, t, registry, tool.Name, without)
				if !res.IsError || string(contentJSON(t, res)) == string(baseBody) {
					t.Errorf("the schema requires %q but the handler does not reject a call without it: %s", field, contentJSON(t, res))
				}
			}
		})
	}
}

func mentions(message, field string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(field) + `\b`).MatchString(message)
}

func callWith(ctx context.Context, t *testing.T, r *Registry, name string, input map[string]json.RawMessage) Result {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Call(ctx, name, body)
	if err != nil {
		t.Fatalf("%s %s: %v", name, body, err)
	}
	return res
}

func contentJSON(t *testing.T, res Result) []byte {
	t.Helper()
	body, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
