package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestAllRequesterListUsesTheListedIndex(t *testing.T) {
	d := dbtest.New(t)
	tx, err := d.SQLDB(t).BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(`EXPLAIN SELECT r.id FROM agent_requests r WHERE r.source <> 'suggest' AND r.archived_at IS NULL ORDER BY r.created_at DESC, r.id DESC LIMIT 21`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	if !strings.Contains(plan.String(), "agent_requests_listed_recent_idx") {
		t.Fatalf("plan does not name agent_requests_listed_recent_idx:\n%s", plan.String())
	}
}
