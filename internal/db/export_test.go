package db

import "context"

func SetSpecUpgradeBatch(n int) (restore func()) {
	previous := specUpgradeBatch
	specUpgradeBatch = n
	return func() { specUpgradeBatch = previous }
}

func (d *DB) MigrateDownTo(ctx context.Context, version int64) error {
	_, err := d.migrator.DownTo(ctx, version)
	return err
}
