package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const evolveSpecsVersion int64 = 10

var specUpgradeBatch = 500

func evolveSpecsMigration() *goose.Migration {
	return goose.NewGoMigration(evolveSpecsVersion,
		&goose.GoFunc{RunTx: UpgradeStoredSpecs},
		&goose.GoFunc{RunTx: refuseDown},
	)
}

type storedSpec struct {
	dashboardID string
	version     int
	text        string
}

func UpgradeStoredSpecs(ctx context.Context, tx *sql.Tx) error {
	var last *storedSpec
	for {
		page, err := readSpecPage(ctx, tx, last)
		if err != nil {
			return err
		}
		for _, row := range page {
			if err := upgradeStoredSpec(ctx, tx, row); err != nil {
				return err
			}
		}
		if len(page) < specUpgradeBatch {
			return nil
		}
		last = &page[len(page)-1]
	}
}

func readSpecPage(ctx context.Context, tx *sql.Tx, after *storedSpec) ([]storedSpec, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if after == nil {
		rows, err = tx.QueryContext(ctx,
			`SELECT dashboard_id::text, version, spec::text FROM dashboard_versions ORDER BY dashboard_id, version LIMIT $1`,
			specUpgradeBatch)
	} else {
		rows, err = tx.QueryContext(ctx,
			`SELECT dashboard_id::text, version, spec::text FROM dashboard_versions
WHERE (dashboard_id, version) > ($2::uuid, $3) ORDER BY dashboard_id, version LIMIT $1`,
			specUpgradeBatch, after.dashboardID, after.version)
	}
	if err != nil {
		return nil, fmt.Errorf("read stored specs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var page []storedSpec
	for rows.Next() {
		var row storedSpec
		if err := rows.Scan(&row.dashboardID, &row.version, &row.text); err != nil {
			return nil, fmt.Errorf("read stored specs: %w", err)
		}
		page = append(page, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read stored specs: %w", err)
	}
	return page, nil
}

func upgradeStoredSpec(ctx context.Context, tx *sql.Tx, row storedSpec) error {
	out, changed, err := spec.Upgrade([]byte(row.text))
	if err != nil {
		return fmt.Errorf("upgrade the spec of dashboard %s version %d: %w", row.dashboardID, row.version, err)
	}
	if !changed {
		return nil
	}
	if _, err := spec.Parse(out); err != nil {
		return fmt.Errorf("parse the upgraded spec of dashboard %s version %d: %w", row.dashboardID, row.version, err)
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE dashboard_versions SET spec = $1::jsonb WHERE dashboard_id = $2::uuid AND version = $3`,
		string(out), row.dashboardID, row.version)
	if err != nil {
		return fmt.Errorf("store the upgraded spec of dashboard %s version %d: %w", row.dashboardID, row.version, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store the upgraded spec of dashboard %s version %d: %w", row.dashboardID, row.version, err)
	}
	if affected != 1 {
		return fmt.Errorf("store the upgraded spec of dashboard %s version %d: updated %d rows, want 1", row.dashboardID, row.version, affected)
	}
	return nil
}

func refuseDown(context.Context, *sql.Tx) error {
	return errors.New("migration 10 cannot be reverted: it rewrote every stored Spec to evolved ObserveSpec v0; restore the backup taken before the upgrade")
}
