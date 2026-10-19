package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestOriginClientConstraintsRequireAnMCPOrigin(t *testing.T) {
	d := dbtest.New(t)
	sqlDB := d.SQLDB(t)
	ctx := context.Background()
	seedDashboard(t, d, dashboardA, 1)
	long := strings.Repeat("c", 65)
	inserts := map[string]string{
		"dashboard_versions": `INSERT INTO dashboard_versions (dashboard_id, version, spec, summary, author_kind, author_id, author_name, origin, origin_client)
VALUES ($1::uuid, $2, '{}'::jsonb, 's', 'user', gen_random_uuid(), 'n', $3, $4)`,
		"agent_requests": `INSERT INTO agent_requests (dashboard_id, source, prompt, base_version, requested_by_kind, requested_by_id, requested_by_name, origin, origin_client)
VALUES ($1::uuid, 'prompt', 'p', $2, 'user', gen_random_uuid(), 'n', $3, $4)`,
	}
	tests := []struct {
		name   string
		origin any
		client any
		ok     bool
	}{
		{"no origin and no client", nil, nil, true},
		{"web without a client", "web", nil, true},
		{"mcp without a client", "mcp", nil, true},
		{"mcp with a client", "mcp", "claude-code", true},
		{"no origin with a client", nil, "x", false},
		{"web with a client", "web", "x", false},
		{"cli with a client", "cli", "x", false},
		{"api with a client", "api", "x", false},
		{"mcp with an empty client", "mcp", "", false},
		{"mcp with a client over 64 characters", "mcp", long, false},
		{"unknown origin", "email", nil, false},
	}
	for table, query := range inserts {
		for i, tt := range tests {
			t.Run(table+" "+tt.name, func(t *testing.T) {
				_, err := sqlDB.ExecContext(ctx, query, dashboardA, i+1, tt.origin, tt.client)
				if tt.ok && err != nil {
					t.Fatalf("insert rejected: %v", err)
				}
				if !tt.ok && (err == nil || !strings.Contains(err.Error(), "violates check constraint")) {
					t.Fatalf("insert (origin %v, client %v) = %v, want a check constraint violation", tt.origin, tt.client, err)
				}
			})
		}
	}
}
